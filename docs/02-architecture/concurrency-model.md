# 泳道并发模型

## 问题背景

在处理高并发聊天时，传统的线程池或单线程模型存在以下问题：

| 模型 | 问题 |
|------|------|
| **单线程** | 处理复杂查询时阻塞其他用户 |
| **全局线程池** | 上下文混乱，状态竞争 |
| **每消息一线程** | 资源消耗大，无法保证顺序 |

## 泳道设计理念

OpenClaw 引入了**泳道（Lanes）**的概念，这是一种轻量级的应用层并发控制机制。

### 核心思想

```
每个会话 (Session) = 独立的泳道 (Lane)
泳道内串行处理 + 泳道间并行执行 = 高并发 + 上下文隔离
```

### 类比

类似游泳比赛的泳道：

```
用户 A ━━━━━━━━━━━━━━━━━━━━━━→  泳道 1
用户 B ━━━━━━━━━━━━━━━━━━━━━━→  泳道 2
用户 C ━━━━━━━━━━━━━━━━━━━━━━→  泳道 3
```

- 各泳道互不干扰（并行）
- 同一泳道内按顺序处理（串行）

## 三大核心特性

### 1. 隔离性（Isolation）

每个会话被分配到一个独立的逻辑泳道中。

**好处**：

```
用户 A 的复杂查询（生成长报告）
    ↓
不会阻塞
    ↓
用户 B 的简单问候
```

**实现机制**：

```go
// 每个 SessionID 对应一个独立的 Goroutine
type Gateway struct {
    sessions map[string]chan Event  // SessionID → 泳道 Channel
    mu       sync.RWMutex
}

func (g *Gateway) dispatch(evt Event) {
    g.mu.Lock()
    defer g.mu.Unlock()

    lane, exists := g.sessions[evt.SessionID]
    if !exists {
        // 创建新泳道
        lane = make(chan Event, 100)
        g.sessions[evt.SessionID] = lane
        go g.processLane(evt.SessionID, lane)
    }

    lane <- evt  // 推入泳道队列
}
```

### 2. 优先级队列（Priority Queue）

泳道内部维护着优先级队列，确保关键消息优先处理。

#### 优先级分类

| 优先级 | 泳道类型 | 说明 | 示例 |
|--------|----------|------|------|
| **高** | Chat Lane | 用户直接指令 | "@bot 查询订单" |
| **中** | System Lane | 系统通知 | "新消息提醒" |
| **低** | Cron/Heartbeat Lane | 后台定时任务 | "每小时检查" |

#### 实现示例

```go
type PriorityEvent struct {
    Event    Event
    Priority int
}

type LaneProcessor struct {
    queue PriorityQueue  // 最小堆实现
}

func (lp *LaneProcessor) Run() {
    for {
        evt := lp.queue.Pop()  // 取出最高优先级事件
        lp.handle(evt)
    }
}
```

### 3. 状态锁定（State Locking）

在同一个泳道内，消息串行处理，防止上下文竞争。

#### 问题场景

```
用户快速发送两条消息：
1. "我叫 Alice"
2. "我叫什么？"

如果并行处理，可能导致：
- 消息 2 先处理，AI 回答"我不知道"
- 消息 1 后处理，更新记忆
```

#### 解决方案

泳道内串行处理，确保：

```
1. 收到"我叫 Alice" → 处理 → 更新记忆
2. 收到"我叫什么？"  → 处理 → 读取记忆 → 回答"Alice"
```

**实现**：

```go
func (g *Gateway) processLane(sessionID string, lane <-chan Event) {
    brain := NewAgentBrain(sessionID)

    // 串行处理该 Session 的所有事件
    for evt := range lane {
        // 确保上一条消息处理完成后再处理下一条
        brain.Handle(evt)
    }
}
```

## 泳道生命周期

### 创建时机

当收到来自新 SessionID 的第一个事件时：

```go
func (g *Gateway) getOrCreateLane(sessionID string) chan Event {
    g.mu.Lock()
    defer g.mu.Unlock()

    if lane, exists := g.sessions[sessionID]; exists {
        return lane
    }

    // 创建新泳道
    lane := make(chan Event, 100)  // 缓冲 100 个事件
    g.sessions[sessionID] = lane

    // 启动处理 Goroutine
    go g.processLane(sessionID, lane)

    return lane
}
```

### 销毁时机

为了避免内存泄漏，需要定期清理不活跃的泳道：

```go
func (g *Gateway) cleanupInactiveLanes() {
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()

    for range ticker.C {
        g.mu.Lock()
        now := time.Now()

        for sessionID, lastActive := range g.lastActivity {
            // 清理超过 24 小时无活动的泳道
            if now.Sub(lastActive) > 24*time.Hour {
                close(g.sessions[sessionID])
                delete(g.sessions, sessionID)
                delete(g.lastActivity, sessionID)
            }
        }

        g.mu.Unlock()
    }
}
```

## 性能优化

### 1. 缓冲通道

使用带缓冲的 Channel 避免阻塞：

```go
// 缓冲 100 个事件，减少发送方等待
lane := make(chan Event, 100)
```

### 2. Goroutine 池

对于 CPU 密集型任务，使用有限的 Worker 池：

```go
type WorkerPool struct {
    tasks   chan func()
    workers int
}

func (wp *WorkerPool) Start() {
    for i := 0; i < wp.workers; i++ {
        go func() {
            for task := range wp.tasks {
                task()
            }
        }()
    }
}
```

### 3. 背压控制

当泳道处理速度跟不上消息到达速度时：

```go
func (g *Gateway) dispatch(evt Event) {
    lane := g.getOrCreateLane(evt.SessionID)

    select {
    case lane <- evt:
        // 成功推入
    case <-time.After(5 * time.Second):
        // 超时，泳道可能阻塞
        logger.Warn("Lane full", "sessionId", evt.SessionID)
        // 可以选择丢弃或持久化到数据库
    }
}
```

## 监控与可观测性

### 关键指标

| 指标 | 说明 | 告警阈值 |
|------|------|----------|
| **活跃泳道数** | 当前正在处理的会话数 | > 10000 |
| **泳道队列深度** | 等待处理的消息数 | > 500 |
| **平均处理延迟** | 从收到消息到开始处理的时间 | > 10s |
| **Goroutine 数量** | 系统 Goroutine 总数 | > 50000 |

### 日志示例

```go
func (g *Gateway) processLane(sessionID string, lane <-chan Event) {
    logger.Info("Lane started", "sessionId", sessionID)
    defer logger.Info("Lane stopped", "sessionId", sessionID)

    brain := NewAgentBrain(sessionID)

    for evt := range lane {
        start := time.Now()
        brain.Handle(evt)

        logger.Debug("Event processed",
            "sessionId", sessionID,
            "eventType", evt.Type,
            "duration", time.Since(start),
        )
    }
}
```

## 与其他模型对比

| 模型 | 优势 | 劣势 | 适用场景 |
|------|------|------|----------|
| **泳道模型** | 隔离性强，顺序保证 | 需要管理泳道生命周期 | AI 对话，有状态服务 |
| **Actor 模型** | 天然隔离，易于分布式 | 学习曲线陡峭 | Erlang/Akka 应用 |
| **线程池** | 简单直观 | 无状态隔离 | 无状态 HTTP 服务 |
| **事件循环** | 低资源消耗 | 单线程阻塞 | Node.js 应用 |

---

**相关文档**:
- [网关架构](gateway.md)
- [网关实现](../04-go-implementation/gateway-impl.md)
