# 聊天软件适配器

## 设计原则

OpenClaw 的强大之处在于其对主流聊天软件的深度集成，这主要依赖于一系列开源协议适配器。

### 适配器模式

```
聊天平台协议 → 适配器 → 统一 Event 格式 → 网关
```

## 支持的平台

| 平台 | 库/协议 | 连接方式 | 特点 |
|------|---------|----------|------|
| **WhatsApp** | Baileys | WebSocket | 端到端加密，无需 API 申请 |
| **Telegram** | grammY | Long Polling / Webhook | 官方 Bot API，功能丰富 |
| **Discord** | discord.js | Gateway WebSocket | 支持分片，适合大服务器 |

## WhatsApp 集成

### 技术选型：Baileys 协议

Baileys 是一个 TypeScript 编写的库，逆向工程了 WhatsApp Web 的 WebSocket 协议。

#### 工作原理

```
1. 模拟浏览器客户端
   ↓
2. 扫描二维码配对
   ↓
3. 建立 WebSocket 长连接
   ↓
4. 解密 Protobuf 消息
   ↓
5. 转换为标准事件
```

#### 核心优势

| 特性 | 说明 |
|------|------|
| **无需申请 API** | 适合个人用户使用 |
| **端到端加密** | 支持 E2E 加密消息 |
| **完整功能** | 支持文本、图片、视频、语音 |
| **群组支持** | 支持群聊和广播列表 |

#### 实现示例

```typescript
import makeWASocket from '@whiskeysockets/baileys';

const sock = makeWASocket({
    auth: state.auth,
    printQRInTerminal: true,
});

// 监听消息
sock.ev.on('messages.upsert', async (m) => {
    const msg = m.messages[0];

    // 转换为标准事件
    const event = {
        type: 'message.received',
        source: 'whatsapp',
        sessionId: msg.key.remoteJid,
        payload: {
            content: msg.message?.conversation || '',
            senderId: msg.key.participant || msg.key.remoteJid,
            senderName: msg.pushName || 'Unknown',
            messageId: msg.key.id,
        },
        timestamp: new Date(msg.messageTimestamp * 1000),
    };

    // 推送到网关
    gateway.pushEvent(event);
});
```

#### 消息类型处理

```typescript
function extractMessage(msg: WAMessage): string {
    // 文本消息
    if (msg.message?.conversation) {
        return msg.message.conversation;
    }

    // 扩展文本（带格式）
    if (msg.message?.extendedTextMessage) {
        return msg.message.extendedTextMessage.text;
    }

    // 图片带描述
    if (msg.message?.imageMessage?.caption) {
        return msg.message.imageMessage.caption;
    }

    // 其他类型...
    return '[不支持的消息类型]';
}
```

#### 会话持久化

```typescript
import { useMultiFileAuthState } from '@whiskeysockets/baileys';

// 存储认证信息，避免每次都要扫码
const { state, saveCreds } = await useMultiFileAuthState('./auth_info');

const sock = makeWASocket({ auth: state });

// 保存凭证
sock.ev.on('creds.update', saveCreds);
```

## Telegram 集成

### 技术选型：grammY 框架

grammY 是对接 Telegram Bot API 的现代 TypeScript 框架。

#### Long Polling 模式

```typescript
import { Bot } from 'grammy';

const bot = new Bot(process.env.TELEGRAM_BOT_TOKEN);

// 消息处理
bot.on('message:text', async (ctx) => {
    const event = {
        type: 'message.received',
        source: 'telegram',
        sessionId: `telegram_${ctx.chat.id}`,
        payload: {
            content: ctx.message.text,
            senderId: ctx.from.id.toString(),
            senderName: ctx.from.first_name,
            messageId: ctx.message.message_id.toString(),
        },
        timestamp: new Date(ctx.message.date * 1000),
    };

    gateway.pushEvent(event);
});

// 启动 Long Polling
bot.start();
```

#### 核心优势

| 特性 | 说明 |
|------|------|
| **内网穿透免费** | Long Polling 无需公网 IP |
| **功能丰富** | 支持内联键盘、投票、支付 |
| **文件处理** | 支持大文件（最高 2GB） |
| **群组管理** | 强大的群组权限控制 |

#### 命令处理

```typescript
// 命令：/start
bot.command('start', async (ctx) => {
    await ctx.reply('欢迎使用 OpenClaw！');
});

// 命令：/status
bot.command('status', async (ctx) => {
    const status = await getSystemStatus();
    await ctx.reply(`系统状态：\n${status}`);
});
```

#### 文件处理

```typescript
bot.on('message:photo', async (ctx) => {
    const photo = ctx.message.photo[ctx.message.photo.length - 1];
    const file = await ctx.api.getFile(photo.file_id);
    const url = `https://api.telegram.org/file/bot${token}/${file.file_path}`;

    // 下载文件
    const response = await fetch(url);
    const buffer = await response.arrayBuffer();

    // 保存或处理文件...
});
```

## Discord 集成

### 技术选型：discord.js

discord.js 是 Discord 官方推荐的 Node.js 库。

#### Gateway 连接

```typescript
import { Client, GatewayIntentBits } from 'discord.js';

const client = new Client({
    intents: [
        GatewayIntentBits.Guilds,
        GatewayIntentBits.GuildMessages,
        GatewayIntentBits.MessageContent,
        GatewayIntentBits.DirectMessages,
    ],
});

client.on('messageCreate', async (message) => {
    // 忽略机器人自己的消息
    if (message.author.bot) return;

    const event = {
        type: 'message.received',
        source: 'discord',
        sessionId: `discord_${message.channelId}`,
        payload: {
            content: message.content,
            senderId: message.author.id,
            senderName: message.author.username,
            messageId: message.id,
            isDirect: message.channel.type === 'DM',
        },
        timestamp: message.createdAt,
    };

    gateway.pushEvent(event);
});

client.login(process.env.DISCORD_BOT_TOKEN);
```

#### 意图（Intents）管理

Discord 要求明确声明需要的权限：

| Intent | 说明 | 场景 |
|--------|------|------|
| **Guilds** | 基础服务器信息 | 必需 |
| **GuildMessages** | 服务器消息 | 群聊机器人 |
| **MessageContent** | 消息内容（需申请） | 读取消息文本 |
| **DirectMessages** | 私信 | DM 支持 |

#### 提及处理

```typescript
client.on('messageCreate', async (message) => {
    // 只响应@机器人的消息
    if (!message.mentions.has(client.user.id)) return;

    // 去除@前缀
    const content = message.content.replace(`<@${client.user.id}>`, '').trim();

    // 处理消息...
});
```

#### Slash 命令

```typescript
// 注册命令
await client.application.commands.create({
    name: 'ask',
    description: '向 AI 提问',
    options: [
        {
            name: 'question',
            description: '你的问题',
            type: 3, // STRING
            required: true,
        },
    ],
});

// 处理命令
client.on('interactionCreate', async (interaction) => {
    if (!interaction.isCommand()) return;

    if (interaction.commandName === 'ask') {
        const question = interaction.options.getString('question');
        await interaction.reply(`思考中...`);

        // 处理问题...
        const answer = await processQuestion(question);
        await interaction.editReply(answer);
    }
});
```

## 统一适配器接口

为了便于扩展新平台，定义统一的适配器接口：

```typescript
interface ChatAdapter {
    name: string;
    connect(): Promise<void>;
    disconnect(): Promise<void>;
    sendMessage(sessionId: string, content: string): Promise<void>;
    onMessage(handler: (event: Event) => void): void;
}
```

### 实现示例

```typescript
class WhatsAppAdapter implements ChatAdapter {
    name = 'whatsapp';
    private sock: WASocket;
    private eventHandler: (event: Event) => void;

    async connect() {
        this.sock = makeWASocket({...});
        this.sock.ev.on('messages.upsert', this.handleMessage.bind(this));
    }

    async disconnect() {
        this.sock?.end();
    }

    async sendMessage(sessionId: string, content: string) {
        await this.sock.sendMessage(sessionId, { text: content });
    }

    onMessage(handler: (event: Event) => void) {
        this.eventHandler = handler;
    }

    private handleMessage(m: any) {
        const event = this.convertToEvent(m);
        this.eventHandler?.(event);
    }
}
```

## 消息发送

### 回复处理

```typescript
// 网关监听需要发送的消息
gateway.on('message.send', async (event) => {
    const adapter = getAdapter(event.source);

    try {
        await adapter.sendMessage(
            event.sessionId,
            event.payload.content
        );

        logger.info('Message sent', {
            source: event.source,
            sessionId: event.sessionId,
        });
    } catch (error) {
        logger.error('Failed to send message', error);
    }
});
```

### 富文本支持

```typescript
interface RichMessage {
    text?: string;
    image?: {
        url: string;
        caption?: string;
    };
    buttons?: Array<{
        text: string;
        callback: string;
    }>;
}
```

## 错误处理

### 重连机制

```typescript
class ResilientAdapter {
    private reconnectAttempts = 0;
    private maxReconnectAttempts = 5;

    async connect() {
        try {
            await this.adapter.connect();
            this.reconnectAttempts = 0;
        } catch (error) {
            if (this.reconnectAttempts < this.maxReconnectAttempts) {
                this.reconnectAttempts++;
                const delay = Math.pow(2, this.reconnectAttempts) * 1000;

                logger.warn(`Reconnecting in ${delay}ms...`);
                await sleep(delay);
                await this.connect();
            } else {
                throw new Error('Max reconnect attempts reached');
            }
        }
    }
}
```

---

**相关文档**:
- [网关架构](gateway.md)
- [聊天适配器实现](../04-go-implementation/chat-adapters-impl.md)
