# GoClaw 系统架构

## 架构概览

GoClaw 采用**清晰架构（Clean Architecture）**，将业务逻辑与外部接口解耦，确保系统的可测试性和可维护性。

### 分层设计

```
┌─────────────────────────────────────────┐
│     接入层 (Interfaces Layer)          │
│  WebSocket Server, HTTP API, CLI       │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│     适配层 (Adapters Layer)            │
│  WhatsApp, Telegram, Discord           │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│     核心层 (Core Layer)                 │
│  Gateway, Brain, Scheduler              │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│  基础设施层 (Infrastructure Layer)      │
│  SQLite, LLM Clients, File System       │
└─────────────────────────────────────────┘
```

### 层级职责

| 层级 | 组件 | 职责 |
|------|------|------|
| **接入层** | WebSocket Server, HTTP API | 处理客户端连接，协议升级，鉴权 |
| **适配层** | WhatsApp(whatsmeow), Telegram(tg-bot-api) | 将各平台私有协议转换为内部 Event 结构 |
| **核心层** | Gateway, Brain (Agent Loop), Scheduler | 核心业务逻辑，状态管理，泳道调度 |
| **基础设施层** | SQLite (CGO), LLM Clients | 数据持久化，外部 API 调用 |

## 项目结构

```
kele/
├── cmd/
│   └── kele/
│       └── main.go              # 入口文件
├── internal/
│   ├── gateway/                 # 网关模块
│   │   ├── gateway.go           # 网关核心
│   │   ├── dispatcher.go        # 事件分发器
│   │   └── lane.go              # 泳道处理
│   ├── adapters/                # 聊天适配器
│   │   ├── whatsapp/
│   │   │   └── connector.go
│   │   ├── telegram/
│   │   │   └── connector.go
│   │   └── discord/
│   │       └── connector.go
│   ├── brain/                   # AI 大脑
│   │   ├── agent.go             # 智能体核心
│   │   ├── tools.go             # 工具执行
│   │   └── memory.go            # 记忆管理
│   ├── llm/                     # LLM 客户端
│   │   ├── provider.go          # 提供商接口
│   │   ├── openai.go
│   │   ├── anthropic.go
│   │   └── router.go            # 智能路由
│   ├── memory/                  # 记忆系统
│   │   ├── store.go             # 存储引擎
│   │   ├── fts.go               # 全文搜索
│   │   ├── vector.go            # 向量搜索
│   │   └── hybrid.go            # 混合检索
│   ├── security/                # 安全模块
│   │   ├── pairing.go           # 配对机制
│   │   ├── whitelist.go         # 白名单管理
│   │   └── ratelimit.go         # 速率限制
│   ├── scheduler/               # 调度器
│   │   └── heartbeat.go         # 心跳机制
│   └── types/                   # 类型定义
│       ├── event.go
│       ├── message.go
│       └── config.go
├── pkg/                         # 可导出的包
│   └── utils/
│       ├── logger.go
│       └── crypto.go
├── configs/                     # 配置文件
│   └── config.yaml
├── data/                        # 数据目录
│   ├── MEMORY.md
│   ├── HEARTBEAT.md
│   └── sessions/
├── docs/                        # 文档
├── scripts/                     # 脚本
│   ├── build.sh
│   └── deploy.sh
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## 核心类型定义

### 事件类型

```go
// internal/types/event.go
package types

import "time"

type EventType string

const (
    EventMessageReceived EventType = "message.received"
    EventMessageSent     EventType = "message.sent"
    EventHeartbeat       EventType = "system.heartbeat"
    EventSystemSignal    EventType = "system.signal"
    EventUserCommand     EventType = "user.command"
)

// Event 定义了系统内部流转的标准数据包
type Event struct {
    ID        string                 `json:"id"`
    Type      EventType              `json:"type"`
    Payload   interface{}            `json:"payload"`
    SessionID string                 `json:"session_id"`
    Timestamp time.Time              `json:"timestamp"`
    Source    string                 `json:"source"` // e.g., "whatsapp", "system"
}

// Message 定义了聊天内容的标准格式
type Message struct {
    SenderID   string `json:"sender_id"`
    SenderName string `json:"sender_name"`
    Content    string `json:"content"`
    IsDirect   bool   `json:"is_direct"`
    MessageID  string `json:"message_id"`
    Raw        any    `json:"-"` // 保留原始协议数据
}
```

### 配置结构

```go
// internal/types/config.go
package types

type Config struct {
    Gateway   GatewayConfig   `yaml:"gateway"`
    Adapters  AdaptersConfig  `yaml:"adapters"`
    LLM       LLMConfig       `yaml:"llm"`
    Memory    MemoryConfig    `yaml:"memory"`
    Security  SecurityConfig  `yaml:"security"`
}

type GatewayConfig struct {
    Host string `yaml:"host"`
    Port int    `yaml:"port"`
}

type AdaptersConfig struct {
    WhatsApp *WhatsAppConfig `yaml:"whatsapp"`
    Telegram *TelegramConfig `yaml:"telegram"`
    Discord  *DiscordConfig  `yaml:"discord"`
}

type LLMConfig struct {
    DefaultProvider string                    `yaml:"default_provider"`
    Providers       map[string]ProviderConfig `yaml:"providers"`
}

type MemoryConfig struct {
    DBPath     string `yaml:"db_path"`
    DataDir    string `yaml:"data_dir"`
    IndexChunk int    `yaml:"index_chunk"`
}

type SecurityConfig struct {
    EnablePairing bool     `yaml:"enable_pairing"`
    AllowedTools  []string `yaml:"allowed_tools"`
    RateLimit     RateLimitConfig `yaml:"rate_limit"`
}
```

## 依赖注入

使用依赖注入容器管理组件生命周期：

```go
// internal/app/app.go
package app

type App struct {
    config    *types.Config
    gateway   *gateway.Gateway
    scheduler *scheduler.Scheduler
    memory    *memory.Store
    security  *security.Manager
}

func New(configPath string) (*App, error) {
    // 1. 加载配置
    cfg, err := loadConfig(configPath)
    if err != nil {
        return nil, err
    }

    // 2. 初始化基础设施
    memStore, err := memory.NewStore(cfg.Memory)
    if err != nil {
        return nil, err
    }

    // 3. 初始化核心组件
    secMgr := security.NewManager(cfg.Security)

    gw := gateway.NewGateway(cfg.Gateway, secMgr)

    sched := scheduler.NewScheduler(cfg, gw)

    // 4. 注册适配器
    if cfg.Adapters.WhatsApp != nil {
        wa := adapters.NewWhatsAppConnector(cfg.Adapters.WhatsApp)
        gw.RegisterConnector(wa)
    }

    if cfg.Adapters.Telegram != nil {
        tg := adapters.NewTelegramConnector(cfg.Adapters.Telegram)
        gw.RegisterConnector(tg)
    }

    return &App{
        config:    cfg,
        gateway:   gw,
        scheduler: sched,
        memory:    memStore,
        security:  secMgr,
    }, nil
}

func (a *App) Start() error {
    // 启动组件
    if err := a.gateway.Start(); err != nil {
        return err
    }

    if err := a.scheduler.Start(); err != nil {
        return err
    }

    return nil
}

func (a *App) Stop() error {
    // 优雅关闭
    a.scheduler.Stop()
    a.gateway.Stop()
    a.memory.Close()

    return nil
}
```

## 主程序入口

```go
// cmd/kele/main.go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    "github.com/yourusername/kele/internal/app"
    "github.com/yourusername/kele/pkg/utils"
)

func main() {
    // 1. 初始化日志
    logger := utils.NewLogger()

    // 2. 创建应用
    application, err := app.New("./configs/config.yaml")
    if err != nil {
        logger.Fatal("Failed to create app", "error", err)
    }

    // 3. 启动应用
    if err := application.Start(); err != nil {
        logger.Fatal("Failed to start app", "error", err)
    }

    logger.Info("GoClaw started successfully")

    // 4. 等待信号
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    select {
    case sig := <-sigChan:
        logger.Info("Received signal", "signal", sig)
    case <-ctx.Done():
        logger.Info("Context cancelled")
    }

    // 5. 优雅关闭
    logger.Info("Shutting down...")
    if err := application.Stop(); err != nil {
        logger.Error("Failed to stop app", "error", err)
    }

    logger.Info("GoClaw stopped")
}
```

## 配置文件示例

```yaml
# configs/config.yaml
gateway:
  host: 127.0.0.1
  port: 18789

adapters:
  whatsapp:
    enabled: true
    session_dir: ./data/whatsapp
  telegram:
    enabled: true
    bot_token: ${TELEGRAM_BOT_TOKEN}
    polling_timeout: 60
  discord:
    enabled: false
    bot_token: ${DISCORD_BOT_TOKEN}

llm:
  default_provider: anthropic
  providers:
    anthropic:
      api_key: ${ANTHROPIC_API_KEY}
      model: claude-3-5-sonnet-20241022
      max_tokens: 4096
    openai:
      api_key: ${OPENAI_API_KEY}
      model: gpt-4o
      max_tokens: 2000

memory:
  db_path: ./data/memory.db
  data_dir: ./data
  index_chunk: 500

security:
  enable_pairing: true
  allowed_tools:
    - send_message
    - read_file
    - search_web
  rate_limit:
    requests_per_minute: 10
    burst: 20
```

## 构建与部署

### Makefile

```makefile
# Makefile
.PHONY: build test run clean

# 构建
build:
	go build -o bin/kele ./cmd/kele

# 开发模式运行
run:
	go run ./cmd/kele

# 测试
test:
	go test -v ./...

# 清理
clean:
	rm -rf bin/

# 交叉编译
build-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/kele-linux-amd64 ./cmd/kele

build-arm:
	GOOS=linux GOARCH=arm64 go build -o bin/kele-linux-arm64 ./cmd/kele

build-all: build-linux build-arm

# Docker
docker-build:
	docker build -t kele:latest .

# 代码检查
lint:
	golangci-lint run ./...

# 代码格式化
fmt:
	go fmt ./...
```

### Dockerfile

```dockerfile
# Dockerfile
FROM golang:1.23-alpine AS builder

WORKDIR /app

# 安装依赖
RUN apk add --no-cache gcc musl-dev sqlite-dev

# 复制依赖文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源码
COPY . .

# 构建
RUN CGO_ENABLED=1 go build -o kele ./cmd/kele

# 运行时镜像
FROM alpine:latest

RUN apk add --no-cache ca-certificates sqlite-libs

WORKDIR /app

# 复制二进制文件
COPY --from=builder /app/kele .

# 复制配置和数据目录
COPY configs/ ./configs/
RUN mkdir -p ./data

# 暴露端口
EXPOSE 18789

CMD ["./kele"]
```

---

**相关文档**:
- [网关实现](gateway-impl.md)
- [聊天适配器实现](chat-adapters-impl.md)
- [记忆系统实现](memory-impl.md)
