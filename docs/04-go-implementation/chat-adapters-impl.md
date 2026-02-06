# 聊天适配器实现

本文档说明如何用 Go 实现各聊天平台的适配器。

## WhatsApp 适配器（whatsmeow）

### 依赖安装

```bash
go get go.mau.fi/whatsmeow@latest
go get go.mau.fi/whatsmeow/store/sqlstore@latest
```

### 核心实现

```go
// internal/adapters/whatsapp/connector.go
package whatsapp

import (
    "context"
    "database/sql"
    "go.mau.fi/whatsmeow"
    "go.mau.fi/whatsmeow/store/sqlstore"
    "go.mau.fi/whatsmeow/types/events"
)

type Connector struct {
    client   *whatsmeow.Client
    eventBus chan<- types.Event
    config   *types.WhatsAppConfig
}

func NewConnector(cfg *types.WhatsAppConfig) *Connector {
    return &Connector{config: cfg}
}

func (c *Connector) Listen(eventBus chan<- types.Event) error {
    c.eventBus = eventBus

    // 1. 创建存储
    db, err := sql.Open("sqlite3", c.config.SessionDir+"/whatsapp.db")
    if err != nil {
        return err
    }

    store := sqlstore.NewWithDB(db, "sqlite3", nil)
    deviceStore, err := store.GetFirstDevice()
    if err != nil {
        return err
    }

    // 2. 创建客户端
    c.client = whatsmeow.NewClient(deviceStore, nil)

    // 3. 注册事件处理器
    c.client.AddEventHandler(c.handleEvent)

    // 4. 连接
    if c.client.Store.ID == nil {
        // 首次连接，显示二维码
        qrChan, _ := c.client.GetQRChannel(context.Background())
        err = c.client.Connect()
        if err != nil {
            return err
        }

        for evt := range qrChan {
            if evt.Event == "code" {
                logger.Info("Scan QR code:", evt.Code)
            }
        }
    } else {
        // 已配对，直接连接
        err = c.client.Connect()
        if err != nil {
            return err
        }
    }

    logger.Info("WhatsApp connected")
    return nil
}

func (c *Connector) handleEvent(evt interface{}) {
    switch v := evt.(type) {
    case *events.Message:
        c.handleMessage(v)
    }
}

func (c *Connector) handleMessage(msg *events.Message) {
    // 转换为标准事件
    event := types.Event{
        ID:        msg.Info.ID,
        Type:      types.EventMessageReceived,
        Source:    "whatsapp",
        SessionID: msg.Info.Chat.String(),
        Timestamp: msg.Info.Timestamp,
        Payload: types.Message{
            SenderID:   msg.Info.Sender.String(),
            SenderName: msg.Info.PushName,
            Content:    msg.Message.GetConversation(),
            MessageID:  msg.Info.ID,
        },
    }

    c.eventBus <- event
}

func (c *Connector) Send(event types.Event) error {
    msg := event.Payload.(types.Message)

    _, err := c.client.SendMessage(
        context.Background(),
        types.ParseJID(event.SessionID),
        &waProto.Message{
            Conversation: proto.String(msg.Content),
        },
    )

    return err
}
```

## Telegram 适配器

### 依赖安装

```bash
go get github.com/go-telegram-bot-api/telegram-bot-api/v5@latest
```

### 核心实现

```go
// internal/adapters/telegram/connector.go
package telegram

import (
    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Connector struct {
    bot      *tgbotapi.BotAPI
    eventBus chan<- types.Event
    config   *types.TelegramConfig
}

func NewConnector(cfg *types.TelegramConfig) *Connector {
    return &Connector{config: cfg}
}

func (c *Connector) Listen(eventBus chan<- types.Event) error {
    c.eventBus = eventBus

    // 1. 创建 Bot
    bot, err := tgbotapi.NewBotAPI(c.config.BotToken)
    if err != nil {
        return err
    }

    c.bot = bot
    logger.Info("Telegram bot authorized", "username", bot.Self.UserName)

    // 2. 配置 Long Polling
    u := tgbotapi.NewUpdate(0)
    u.Timeout = c.config.PollingTimeout

    updates := bot.GetUpdatesChan(u)

    // 3. 处理更新
    for update := range updates {
        if update.Message != nil {
            c.handleMessage(update.Message)
        }
    }

    return nil
}

func (c *Connector) handleMessage(msg *tgbotapi.Message) {
    event := types.Event{
        ID:        strconv.Itoa(msg.MessageID),
        Type:      types.EventMessageReceived,
        Source:    "telegram",
        SessionID: fmt.Sprintf("telegram_%d", msg.Chat.ID),
        Timestamp: time.Unix(int64(msg.Date), 0),
        Payload: types.Message{
            SenderID:   strconv.FormatInt(msg.From.ID, 10),
            SenderName: msg.From.FirstName,
            Content:    msg.Text,
            MessageID:  strconv.Itoa(msg.MessageID),
        },
    }

    c.eventBus <- event
}

func (c *Connector) Send(event types.Event) error {
    msg := event.Payload.(types.Message)

    chatID, _ := strconv.ParseInt(
        strings.TrimPrefix(event.SessionID, "telegram_"),
        10, 64,
    )

    message := tgbotapi.NewMessage(chatID, msg.Content)
    _, err := c.bot.Send(message)

    return err
}
```

## 统一接口

所有适配器都实现相同的接口：

```go
// internal/gateway/connector.go
package gateway

type Connector interface {
    Name() string
    Listen(eventBus chan<- types.Event) error
    Send(event types.Event) error
    Close() error
}
```

---

**相关文档**:
- [聊天软件适配器架构](../02-architecture/chat-adapters.md)
- [网关实现](gateway-impl.md)
