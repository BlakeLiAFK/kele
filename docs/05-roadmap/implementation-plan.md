# 实施计划

本文档提供分阶段的 GoClaw 开发指南。

## 总体时间线

```
阶段一：核心网关 (2-3 周)
    ↓
阶段二：大脑接入 (1-2 周)
    ↓
阶段三：记忆增强 (2-3 周)
    ↓
阶段四：自主性 (1 周)
    ↓
阶段五：生产部署 (1 周)
```

## 阶段一：核心网关

### 目标

搭建基础的事件驱动架构，实现最基本的消息收发功能。

### 任务清单

- [ ] **项目初始化**
  - [ ] 创建 Go 模块：`go mod init github.com/yourusername/kele`
  - [ ] 设置项目结构（参考架构文档）
  - [ ] 配置 Makefile 和 CI

- [ ] **类型定义**
  - [ ] 定义 `Event` 和 `Message` 结构
  - [ ] 定义 `Config` 结构
  - [ ] 实现配置文件加载

- [ ] **网关核心**
  - [ ] 实现 `Gateway` 结构
  - [ ] 实现事件总线（Channel）
  - [ ] 实现泳道调度器
  - [ ] 实现 WebSocket 服务器

- [ ] **WhatsApp 集成**
  - [ ] 集成 whatsmeow 库
  - [ ] 实现消息接收
  - [ ] 实现消息发送
  - [ ] 测试基本收发

### 验收标准

- ✅ 能够通过 WhatsApp 发送消息到 Bot
- ✅ Bot 能够回复固定文本
- ✅ 日志正确记录所有事件

### 代码示例

```go
// 简单的回声 Bot 测试
func (g *Gateway) processLane(sessionID string, lane <-chan types.Event) {
    for evt := range lane {
        if evt.Type == types.EventMessageReceived {
            msg := evt.Payload.(types.Message)

            // 回显消息
            reply := types.Event{
                Type:      types.EventMessageSent,
                Source:    evt.Source,
                SessionID: evt.SessionID,
                Payload: types.Message{
                    Content: "Echo: " + msg.Content,
                },
            }

            connector := g.GetConnector(evt.Source)
            connector.Send(reply)
        }
    }
}
```

## 阶段二：大脑接入

### 目标

集成 LLM API，使 Bot 能够智能回复。

### 任务清单

- [ ] **LLM 客户端**
  - [ ] 实现 OpenAI 客户端
  - [ ] 实现 Anthropic 客户端
  - [ ] 实现统一的 Provider 接口
  - [ ] 实现认证配置轮换

- [ ] **Agent Brain**
  - [ ] 实现基本的对话处理
  - [ ] 实现上下文管理（会话历史）
  - [ ] 实现工具调用框架

- [ ] **测试工具**
  - [ ] 实现 `send_message` 工具
  - [ ] 实现 `search_web` 工具（可选）

### 验收标准

- ✅ Bot 能够理解并回答问题
- ✅ 支持多轮对话
- ✅ 支持切换不同的 LLM 提供商

### 代码示例

```go
// internal/brain/agent.go
func (a *Agent) Handle(evt types.Event) error {
    msg := evt.Payload.(types.Message)

    // 构建对话历史
    history := a.getHistory(evt.SessionID)
    history = append(history, types.ChatMessage{
        Role:    "user",
        Content: msg.Content,
    })

    // 调用 LLM
    response, err := a.llm.Chat(types.ChatParams{
        Messages: history,
    })

    if err != nil {
        return err
    }

    // 发送回复
    a.sendReply(evt, response.Content)

    // 保存历史
    a.saveHistory(evt.SessionID, history, response.Content)

    return nil
}
```

## 阶段三：记忆增强

### 目标

实现持久化记忆和混合检索。

### 任务清单

- [ ] **SQLite 集成**
  - [ ] 创建数据库 Schema
  - [ ] 启用 FTS5 扩展
  - [ ] 实现基本的 CRUD

- [ ] **文件系统**
  - [ ] 实现 MEMORY.md 读写
  - [ ] 实现会话日志（JSONL）
  - [ ] 实现文件变更监听

- [ ] **混合检索**
  - [ ] 实现全文检索
  - [ ] 实现文档索引
  - [ ] 实现 RRF 融合（可选）

### 验收标准

- ✅ Bot 能够记住用户信息
- ✅ 可以通过自然语言查询历史记忆
- ✅ MEMORY.md 文件正确更新

## 阶段四：自主性

### 目标

实现心跳机制，赋予 Bot 主动性。

### 任务清单

- [ ] **心跳调度**
  - [ ] 实现 Ticker 定时器
  - [ ] 生成系统快照
  - [ ] 触发心跳事件

- [ ] **HEARTBEAT.md**
  - [ ] 实现配置文件读取
  - [ ] 集成到 Agent 决策

- [ ] **主动通知**
  - [ ] 实现主动发送消息
  - [ ] 测试定时任务

### 验收标准

- ✅ Bot 能够按照 HEARTBEAT.md 执行定时任务
- ✅ 系统异常时主动告警

## 阶段五：生产部署

### 目标

优化性能，部署到生产环境。

### 任务清单

- [ ] **安全加固**
  - [ ] 实现 DM 配对机制
  - [ ] 实现速率限制
  - [ ] 加密敏感配置

- [ ] **监控与日志**
  - [ ] 集成 Prometheus
  - [ ] 配置结构化日志
  - [ ] 设置告警规则

- [ ] **部署**
  - [ ] 构建 Docker 镜像
  - [ ] 编写 systemd 服务文件
  - [ ] 配置自动重启

- [ ] **文档**
  - [ ] 编写用户手册
  - [ ] 编写运维文档

### 验收标准

- ✅ 系统稳定运行 7 天无故障
- ✅ 完整的监控和告警
- ✅ 文档齐全

## 开发环境设置

### 依赖安装

```bash
# Go 1.23+
brew install go

# SQLite
brew install sqlite3

# 开发工具
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### 项目初始化

```bash
# 创建项目
mkdir kele && cd kele
go mod init github.com/yourusername/kele

# 安装依赖
go get go.mau.fi/whatsmeow
go get github.com/mattn/go-sqlite3
go get github.com/gorilla/websocket
go get gopkg.in/yaml.v3
```

### 运行开发服务器

```bash
# 复制配置文件
cp configs/config.example.yaml configs/config.yaml

# 设置环境变量
export ANTHROPIC_API_KEY=sk-ant-xxx
export TELEGRAM_BOT_TOKEN=123456:ABC

# 运行
make run
```

## 性能优化建议

### Goroutine 池

对于 CPU 密集型任务，使用有限的 Worker 池：

```go
type WorkerPool struct {
    tasks   chan func()
    workers int
}

func NewWorkerPool(workers int) *WorkerPool {
    pool := &WorkerPool{
        tasks:   make(chan func(), 100),
        workers: workers,
    }

    for i := 0; i < workers; i++ {
        go pool.worker()
    }

    return pool
}

func (p *WorkerPool) worker() {
    for task := range p.tasks {
        task()
    }
}
```

### 数据库连接池

```go
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)
```

### 内存优化

```go
// 使用 sync.Pool 复用对象
var eventPool = sync.Pool{
    New: func() interface{} {
        return &types.Event{}
    },
}

func getEvent() *types.Event {
    return eventPool.Get().(*types.Event)
}

func putEvent(e *types.Event) {
    *e = types.Event{} // 重置
    eventPool.Put(e)
}
```

## 故障排查

### 常见问题

| 问题 | 原因 | 解决方案 |
|------|------|----------|
| WhatsApp 连接失败 | 二维码过期 | 删除 `data/whatsapp.db` 重新扫码 |
| SQLite 锁死 | 并发写入冲突 | 启用 WAL 模式：`PRAGMA journal_mode=WAL` |
| 内存泄漏 | Goroutine 未关闭 | 使用 `pprof` 分析 |

### 调试工具

```bash
# CPU 性能分析
go test -cpuprofile=cpu.prof -bench=.
go tool pprof cpu.prof

# 内存分析
go test -memprofile=mem.prof -bench=.
go tool pprof mem.prof

# 竞态检测
go test -race ./...
```

## 下一步

完成基础实施后，可以考虑：

- 📱 实现移动端控制 App
- 🌐 实现 Web 控制台
- 🔌 支持更多聊天平台（Slack, WeChat）
- 🤖 集成更多 LLM 提供商
- 🎯 实现任务队列和工作流
- 📊 实现数据可视化仪表盘

---

**相关文档**:
- [系统架构](../04-go-implementation/architecture.md)
- [网关实现](../04-go-implementation/gateway-impl.md)
