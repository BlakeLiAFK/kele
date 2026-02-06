# 网关架构：单一事实来源

## 设计哲学

在分布式系统中，状态一致性是一个经典难题。OpenClaw 通过**网关模式（Gateway Pattern）**解决了这一问题。

### 核心理念

```
所有外部通信 → 网关 → 内部逻辑
单一控制平面 → 统一状态管理 → 避免竞态条件
```

## 网关职责

网关是一个长连接的 Node.js/Go 进程，充当所有外部通信和内部逻辑的**枢纽（Hub）**。

### 功能清单

| 职责 | 说明 |
|------|------|
| **连接管理** | 维护与聊天平台的持久连接 |
| **协议适配** | 统一不同平台的消息格式 |
| **事件路由** | 将消息分发到对应的处理器 |
| **状态同步** | 保持所有客户端的状态一致 |
| **权限控制** | 实施安全策略和配对验证 |

## WebSocket 控制平面

### 架构设计

网关并不直接处理业务逻辑，而是维护一个本地 WebSocket 服务器。

```
默认地址：ws://127.0.0.1:18789
```

### 连接模型

```
┌──────────────┐
│   CLI 工具   │
└──────┬───────┘
       │
┌──────▼───────┐     ┌─────────────┐
│  Web 控制台  ├─────►│   Gateway   │
└──────────────┘     │  WebSocket  │
                     │   Server    │
┌──────────────┐     │             │
│ 移动端节点   ├─────►│  :18789     │
└──────────────┘     └──────┬──────┘
                            │
┌──────────────┐            │
│ Agent Runtime├────────────┘
└──────────────┘
```

### 客户端类型

所有组件都作为 WebSocket 客户端连接到网关：

1. **CLI 工具**：命令行操作接口
2. **Web 控制台（Control UI）**：可视化管理界面
3. **移动端节点（iOS/Android Node）**：移动端控制器
4. **核心代理运行时（Agent Runtime）**：AI 逻辑处理单元

### 解耦优势

这种设计将"连接管理"与"智能处理"解耦：

```javascript
// 发送命令示例
const message = {
  type: 'user.command',
  payload: {
    command: '/status',
    sessionId: 'user-123'
  }
};

// CLI → WebSocket → Gateway → Agent Runtime
websocket.send(JSON.stringify(message));
```

**好处**：

- ✅ 组件可独立开发和部署
- ✅ 支持多客户端同时连接
- ✅ 易于扩展新的控制接口
- ✅ 统一的事件流转机制

## 统一事件总线

### 事件标准化

无论来源如何，所有输入都会被转换为统一的事件对象（Event Object）：

```typescript
interface Event {
  id: string;              // 唯一事件 ID
  type: EventType;         // 事件类型
  source: string;          // 来源平台（whatsapp/telegram/system）
  sessionId: string;       // 会话 ID
  timestamp: Date;         // 时间戳
  payload: any;            // 具体数据
}
```

### 事件类型

```typescript
enum EventType {
  // 消息事件
  MESSAGE_RECEIVED = 'message.received',
  MESSAGE_SENT = 'message.sent',

  // 系统事件
  SYSTEM_HEARTBEAT = 'system.heartbeat',
  SYSTEM_STARTUP = 'system.startup',

  // 用户事件
  USER_COMMAND = 'user.command',
  USER_PAIRING = 'user.pairing',
}
```

### 协议屏蔽

标准化处理屏蔽了底层协议的差异：

| 平台 | 协议特点 | 网关处理 |
|------|----------|----------|
| **Telegram** | Long Polling | 转换为 MESSAGE_RECEIVED 事件 |
| **Discord** | Gateway Intent | 过滤仅相关的 Intent |
| **WhatsApp** | WebSocket + Protobuf | 解密后标准化 |
| **System** | 定时器触发 | 生成 SYSTEM_HEARTBEAT 事件 |

### 事件流转示例

```
1. WhatsApp 收到消息
   ↓
2. Baileys 适配器解密 Protobuf
   ↓
3. 转换为标准 Event 对象
   {
     type: 'message.received',
     source: 'whatsapp',
     sessionId: '1234567890@s.whatsapp.net',
     payload: {
       content: 'Hello AI',
       senderId: '...',
       senderName: 'Alice'
     }
   }
   ↓
4. 推送到 WebSocket 总线
   ↓
5. 网关路由到对应的泳道
   ↓
6. Agent Runtime 处理
```

## 网关实现要点

### 1. 单例模式

确保整个系统只有一个网关实例：

```go
var (
    gatewayInstance *Gateway
    once            sync.Once
)

func GetGateway() *Gateway {
    once.Do(func() {
        gatewayInstance = &Gateway{
            eventBus: make(chan Event, 1000),
            sessions: make(map[string]chan Event),
        }
    })
    return gatewayInstance
}
```

### 2. 优雅关闭

处理信号，确保资源正确释放：

```go
func (g *Gateway) Shutdown() {
    // 停止接收新事件
    close(g.eventBus)

    // 等待所有泳道处理完成
    g.wg.Wait()

    // 关闭所有连接器
    for _, connector := range g.connectors {
        connector.Close()
    }
}
```

### 3. 监控与日志

记录关键事件便于排查：

```go
func (g *Gateway) dispatch(evt Event) {
    logger.Info("Event received",
        "type", evt.Type,
        "source", evt.Source,
        "sessionId", evt.SessionID,
    )

    // 路由逻辑...
}
```

## 扩展性设计

### 插件式连接器

网关支持动态注册新的聊天平台：

```go
type Connector interface {
    Name() string
    Listen(eventBus chan<- Event)
    Send(event Event) error
    Close() error
}

// 注册新连接器
gateway.RegisterConnector(NewWhatsAppConnector())
gateway.RegisterConnector(NewTelegramConnector())
gateway.RegisterConnector(NewDiscordConnector())
```

### 水平扩展

对于超高并发场景，可以通过消息队列实现多网关：

```
Gateway 1 → Redis Pub/Sub ← Gateway 2
    ↓                           ↓
Agent Pool              Agent Pool
```

---

**相关文档**:
- [泳道并发模型](concurrency-model.md)
- [网关实现](../04-go-implementation/gateway-impl.md)
