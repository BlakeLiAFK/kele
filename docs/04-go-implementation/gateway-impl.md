# 网关实现

本文档详细说明如何使用 Go 实现 OpenClaw 的核心网关组件。

## 核心结构

```go
// internal/gateway/gateway.go
package gateway

import (
    "sync"
    "github.com/yourusername/kele/internal/types"
)

type Gateway struct {
    config     *types.GatewayConfig
    eventBus   chan types.Event
    sessions   map[string]chan types.Event
    connectors []Connector
    mu         sync.RWMutex
    wg         sync.WaitGroup
    security   Security
}

// Connector 定义聊天平台连接器接口
type Connector interface {
    Name() string
    Listen(eventBus chan<- types.Event) error
    Send(event types.Event) error
    Close() error
}

func NewGateway(cfg *types.GatewayConfig, sec Security) *Gateway {
    return &Gateway{
        config:     cfg,
        eventBus:   make(chan types.Event, 1000),
        sessions:   make(map[string]chan types.Event),
        connectors: make([]Connector, 0),
        security:   sec,
    }
}
```

## 启动与主循环

```go
func (g *Gateway) Start() error {
    // 1. 启动所有连接器
    for _, connector := range g.connectors {
        g.wg.Add(1)
        go func(c Connector) {
            defer g.wg.Done()
            if err := c.Listen(g.eventBus); err != nil {
                logger.Error("Connector error", "name", c.Name(), "error", err)
            }
        }(connector)
    }

    // 2. 启动主事件分发循环
    g.wg.Add(1)
    go g.eventLoop()

    logger.Info("Gateway started", "connectors", len(g.connectors))
    return nil
}

func (g *Gateway) eventLoop() {
    defer g.wg.Done()

    for event := range g.eventBus {
        // 安全检查
        if !g.security.CheckPermission(event) {
            logger.Warn("Event rejected by security", "sessionId", event.SessionID)
            continue
        }

        // 分发到泳道
        g.dispatch(event)
    }
}
```

## 泳道调度

```go
// internal/gateway/dispatcher.go
package gateway

func (g *Gateway) dispatch(evt types.Event) {
    g.mu.Lock()
    defer g.mu.Unlock()

    lane, exists := g.sessions[evt.SessionID]

    if !exists {
        // 创建新泳道
        lane = make(chan types.Event, 100)
        g.sessions[evt.SessionID] = lane

        // 启动处理 Goroutine
        g.wg.Add(1)
        go g.processLane(evt.SessionID, lane)

        logger.Info("Lane created", "sessionId", evt.SessionID)
    }

    // 推入泳道（非阻塞）
    select {
    case lane <- evt:
        // 成功推入
    default:
        logger.Warn("Lane full, dropping event", "sessionId", evt.SessionID)
    }
}

func (g *Gateway) processLane(sessionID string, lane <-chan types.Event) {
    defer g.wg.Done()
    defer g.cleanupLane(sessionID)

    // 为每个会话创建独立的 Agent Brain
    brain := brain.NewAgent(sessionID, g.config)

    for evt := range lane {
        // 串行处理该会话的所有事件
        if err := brain.Handle(evt); err != nil {
            logger.Error("Brain handle error",
                "sessionId", sessionID,
                "error", err)
        }
    }
}

func (g *Gateway) cleanupLane(sessionID string) {
    g.mu.Lock()
    defer g.mu.Unlock()

    delete(g.sessions, sessionID)
    logger.Info("Lane cleaned up", "sessionId", sessionID)
}
```

## WebSocket 服务器

```go
// internal/gateway/websocket.go
package gateway

import (
    "github.com/gorilla/websocket"
    "net/http"
)

type WSServer struct {
    gateway  *Gateway
    upgrader websocket.Upgrader
}

func (ws *WSServer) Start(addr string) error {
    http.HandleFunc("/ws", ws.handleWebSocket)

    logger.Info("WebSocket server starting", "addr", addr)
    return http.ListenAndServe(addr, nil)
}

func (ws *WSServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := ws.upgrader.Upgrade(w, r, nil)
    if err != nil {
        logger.Error("WebSocket upgrade failed", "error", err)
        return
    }
    defer conn.Close()

    // 处理客户端消息
    for {
        var evt types.Event
        if err := conn.ReadJSON(&evt); err != nil {
            logger.Error("WebSocket read error", "error", err)
            break
        }

        // 推送到事件总线
        ws.gateway.eventBus <- evt
    }
}
```

## 连接器注册

```go
func (g *Gateway) RegisterConnector(connector Connector) {
    g.connectors = append(g.connectors, connector)
    logger.Info("Connector registered", "name", connector.Name())
}

func (g *Gateway) GetConnector(name string) Connector {
    for _, c := range g.connectors {
        if c.Name() == name {
            return c
        }
    }
    return nil
}
```

## 优雅关闭

```go
func (g *Gateway) Stop() error {
    logger.Info("Gateway stopping...")

    // 1. 停止接收新事件
    close(g.eventBus)

    // 2. 关闭所有泳道
    g.mu.Lock()
    for _, lane := range g.sessions {
        close(lane)
    }
    g.mu.Unlock()

    // 3. 等待所有 Goroutine 完成
    g.wg.Wait()

    // 4. 关闭所有连接器
    for _, connector := range g.connectors {
        if err := connector.Close(); err != nil {
            logger.Error("Connector close error",
                "name", connector.Name(),
                "error", err)
        }
    }

    logger.Info("Gateway stopped")
    return nil
}
```

## 监控指标

```go
// internal/gateway/metrics.go
package gateway

import "github.com/prometheus/client_golang/prometheus"

var (
    eventsProcessed = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_events_total",
            Help: "Total number of events processed",
        },
        []string{"type", "source"},
    )

    activeLanes = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "gateway_active_lanes",
            Help: "Number of active processing lanes",
        },
    )
)

func init() {
    prometheus.MustRegister(eventsProcessed)
    prometheus.MustRegister(activeLanes)
}

func (g *Gateway) updateMetrics() {
    g.mu.RLock()
    defer g.mu.RUnlock()

    activeLanes.Set(float64(len(g.sessions)))
}
```

---

**相关文档**:
- [网关架构](../02-architecture/gateway.md)
- [泳道并发模型](../02-architecture/concurrency-model.md)
