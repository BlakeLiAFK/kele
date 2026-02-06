# 安全模块实现

实现配对机制、白名单和速率限制。

## 配对管理器

```go
// internal/security/pairing.go
package security

import (
    "crypto/rand"
    "fmt"
    "sync"
    "time"
)

type PairingRequest struct {
    Code       string
    SenderID   string
    SenderName string
    Platform   string
    Timestamp  time.Time
}

type PairingManager struct {
    pending  map[string]PairingRequest
    mu       sync.RWMutex
    db       *sql.DB
}

func NewPairingManager(db *sql.DB) *PairingManager {
    return &PairingManager{
        pending: make(map[string]PairingRequest),
        db:      db,
    }
}

func (pm *PairingManager) RequestPairing(senderID, senderName, platform string) string {
    pm.mu.Lock()
    defer pm.mu.Unlock()

    // 生成 6 位码
    code := pm.generateCode()

    req := PairingRequest{
        Code:       code,
        SenderID:   senderID,
        SenderName: senderName,
        Platform:   platform,
        Timestamp:  time.Now(),
    }

    pm.pending[senderID] = req

    logger.Warn("New pairing request",
        "senderId", senderID,
        "name", senderName,
        "platform", platform,
        "code", code,
    )

    return code
}

func (pm *PairingManager) ApprovePairing(code string) error {
    pm.mu.Lock()
    defer pm.mu.Unlock()

    for senderID, req := range pm.pending {
        if req.Code == code {
            // 添加到白名单
            _, err := pm.db.Exec(
                "INSERT INTO whitelist (sender_id, platform) VALUES (?, ?)",
                senderID, req.Platform,
            )

            if err != nil {
                return err
            }

            delete(pm.pending, senderID)

            logger.Info("Pairing approved", "senderId", senderID)
            return nil
        }
    }

    return fmt.Errorf("invalid or expired code")
}

func (pm *PairingManager) generateCode() string {
    b := make([]byte, 3)
    rand.Read(b)
    return fmt.Sprintf("%06d", int(b[0])<<16|int(b[1])<<8|int(b[2]))
}
```

## 白名单检查

```go
// internal/security/whitelist.go
package security

type Whitelist struct {
    db *sql.DB
}

func NewWhitelist(db *sql.DB) *Whitelist {
    return &Whitelist{db: db}
}

func (w *Whitelist) IsWhitelisted(senderID string) bool {
    var count int
    err := w.db.QueryRow(
        "SELECT COUNT(*) FROM whitelist WHERE sender_id = ?",
        senderID,
    ).Scan(&count)

    return err == nil && count > 0
}

func (w *Whitelist) Add(senderID, platform string) error {
    _, err := w.db.Exec(
        "INSERT OR IGNORE INTO whitelist (sender_id, platform) VALUES (?, ?)",
        senderID, platform,
    )
    return err
}

func (w *Whitelist) Remove(senderID string) error {
    _, err := w.db.Exec(
        "DELETE FROM whitelist WHERE sender_id = ?",
        senderID,
    )
    return err
}
```

## 速率限制

```go
// internal/security/ratelimit.go
package security

import (
    "golang.org/x/time/rate"
    "sync"
)

type RateLimiter struct {
    limiters map[string]*rate.Limiter
    mu       sync.RWMutex
    rate     rate.Limit
    burst    int
}

func NewRateLimiter(ratePerMinute int, burst int) *RateLimiter {
    return &RateLimiter{
        limiters: make(map[string]*rate.Limiter),
        rate:     rate.Limit(float64(ratePerMinute) / 60.0),
        burst:    burst,
    }
}

func (rl *RateLimiter) Allow(key string) bool {
    rl.mu.Lock()
    limiter, exists := rl.limiters[key]
    if !exists {
        limiter = rate.NewLimiter(rl.rate, rl.burst)
        rl.limiters[key] = limiter
    }
    rl.mu.Unlock()

    return limiter.Allow()
}
```

## 安全管理器

```go
// internal/security/manager.go
package security

type Manager struct {
    pairing     *PairingManager
    whitelist   *Whitelist
    rateLimiter *RateLimiter
    config      *types.SecurityConfig
}

func NewManager(cfg *types.SecurityConfig) *Manager {
    db, _ := sql.Open("sqlite3", "./data/security.db")

    return &Manager{
        pairing:     NewPairingManager(db),
        whitelist:   NewWhitelist(db),
        rateLimiter: NewRateLimiter(cfg.RateLimit.RequestsPerMinute, cfg.RateLimit.Burst),
        config:      cfg,
    }
}

func (m *Manager) CheckPermission(event types.Event) bool {
    msg := event.Payload.(types.Message)

    // 1. 检查速率限制
    if !m.rateLimiter.Allow(msg.SenderID) {
        logger.Warn("Rate limit exceeded", "senderId", msg.SenderID)
        return false
    }

    // 2. 检查白名单
    if m.config.EnablePairing && !m.whitelist.IsWhitelisted(msg.SenderID) {
        m.pairing.RequestPairing(msg.SenderID, msg.SenderName, event.Source)
        return false
    }

    return true
}
```

---

**相关文档**:
- [安全架构](../03-core-features/security.md)
