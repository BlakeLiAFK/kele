# 心跳实现

使用 Go 的 time.Ticker 实现智能心跳系统。

## 核心结构

```go
// internal/scheduler/heartbeat.go
package scheduler

import (
    "time"
)

type Scheduler struct {
    gateway  Gateway
    config   *types.Config
    ticker   *time.Ticker
    stopChan chan struct{}
}

func NewScheduler(cfg *types.Config, gw Gateway) *Scheduler {
    return &Scheduler{
        gateway:  gw,
        config:   cfg,
        stopChan: make(chan struct{}),
    }
}
```

## 启动心跳

```go
func (s *Scheduler) Start() error {
    interval := s.config.Heartbeat.IntervalMinutes
    s.ticker = time.NewTicker(time.Duration(interval) * time.Minute)

    go s.heartbeatLoop()

    logger.Info("Heartbeat started", "interval", interval)
    return nil
}

func (s *Scheduler) heartbeatLoop() {
    for {
        select {
        case <-s.ticker.C:
            s.triggerHeartbeat()

        case <-s.stopChan:
            logger.Info("Heartbeat stopped")
            return
        }
    }
}
```

## 触发心跳

```go
func (s *Scheduler) triggerHeartbeat() {
    snapshot := s.generateSnapshot()

    event := types.Event{
        ID:        generateID(),
        Type:      types.EventHeartbeat,
        Source:    "scheduler",
        SessionID: "system_main",
        Timestamp: time.Now(),
        Payload:   snapshot,
    }

    s.gateway.PushEvent(event)

    logger.Info("Heartbeat triggered")
}
```

## 系统快照

```go
type SystemSnapshot struct {
    Time struct {
        Current    time.Time `json:"current"`
        Timezone   string    `json:"timezone"`
        DayOfWeek  string    `json:"day_of_week"`
    } `json:"time"`
    System struct {
        Uptime  float64 `json:"uptime"`
        CPU     float64 `json:"cpu"`
        Memory  float64 `json:"memory"`
        Disk    float64 `json:"disk"`
    } `json:"system"`
    Context struct {
        ActiveSessions int      `json:"active_sessions"`
        RecentErrors   []string `json:"recent_errors"`
    } `json:"context"`
}

func (s *Scheduler) generateSnapshot() SystemSnapshot {
    var snapshot SystemSnapshot

    // 时间信息
    snapshot.Time.Current = time.Now()
    snapshot.Time.Timezone = time.Now().Location().String()
    snapshot.Time.DayOfWeek = time.Now().Weekday().String()

    // 系统信息
    snapshot.System.Uptime = time.Since(startTime).Seconds()
    snapshot.System.CPU = getCPUUsage()
    snapshot.System.Memory = getMemoryUsage()
    snapshot.System.Disk = getDiskUsage()

    // 上下文
    snapshot.Context.ActiveSessions = s.gateway.GetActiveSessionCount()
    snapshot.Context.RecentErrors = getRecentErrors()

    return snapshot
}
```

## 停止心跳

```go
func (s *Scheduler) Stop() error {
    close(s.stopChan)

    if s.ticker != nil {
        s.ticker.Stop()
    }

    return nil
}
```

---

**相关文档**:
- [心跳系统](../03-core-features/heartbeat.md)
