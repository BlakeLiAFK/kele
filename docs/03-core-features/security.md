# 安全架构

## 威胁模型

将 AI 接入个人数字生活带来了巨大的安全风险。OpenClaw 设计了多层防御机制。

### 主要威胁

| 威胁 | 描述 | 影响 |
|------|------|------|
| **未授权访问** | 陌生人向 Bot 发送恶意指令 | 信息泄露、资源滥用 |
| **命令注入** | 用户输入包含危险命令 | 系统破坏、数据丢失 |
| **权限提升** | AI 执行超出预期的操作 | 敏感数据访问、财务损失 |
| **数据泄露** | 记忆被未授权读取 | 隐私侵犯 |
| **DoS 攻击** | 大量请求耗尽资源 | 服务不可用 |

## DM 配对机制

### 设计理念

默认情况下，OpenClaw 对所有未知的私信（DM）采取**"拒绝并配对"**策略。

### 工作流程

```
1. 陌生用户发送消息
   ↓
2. 网关拦截
   ↓
3. 生成 6 位配对码
   ↓
4. 记录到日志
   ↓
5. 管理员批准
   ↓
6. 添加到白名单
```

### 实现示例

```typescript
class PairingMiddleware {
    private pendingPairings = new Map<string, PairingRequest>();

    async handleNewDM(event: MessageEvent): Promise<boolean> {
        const senderId = event.payload.senderId;

        // 1. 检查白名单
        if (await this.isWhitelisted(senderId)) {
            return true; // 放行
        }

        // 2. 检查是否已有待处理的配对请求
        if (this.pendingPairings.has(senderId)) {
            const request = this.pendingPairings.get(senderId);

            // 检查是否过期（15 分钟）
            if (Date.now() - request.timestamp < 15 * 60 * 1000) {
                logger.info('Pairing request already exists', { senderId, code: request.code });
                return false; // 阻止
            }
        }

        // 3. 生成新的配对码
        const code = this.generatePairingCode();

        this.pendingPairings.set(senderId, {
            code,
            senderId,
            senderName: event.payload.senderName,
            platform: event.source,
            timestamp: Date.now(),
        });

        // 4. 记录到日志
        logger.warn('New pairing request', {
            senderId,
            senderName: event.payload.senderName,
            platform: event.source,
            code,
        });

        // 5. 发送提示消息（可选）
        await this.sendMessage(event.source, senderId,
            '你还未被授权。请联系管理员获取配对码。'
        );

        return false; // 阻止消息处理
    }

    private generatePairingCode(): string {
        return Math.floor(100000 + Math.random() * 900000).toString();
    }

    async approvePairing(code: string): Promise<boolean> {
        for (const [senderId, request] of this.pendingPairings.entries()) {
            if (request.code === code) {
                // 添加到白名单
                await db.run(
                    'INSERT INTO whitelist (sender_id, platform, added_at) VALUES (?, ?, ?)',
                    [senderId, request.platform, new Date().toISOString()]
                );

                this.pendingPairings.delete(senderId);

                logger.info('Pairing approved', { senderId });

                // 发送欢迎消息
                await this.sendMessage(request.platform, senderId,
                    '✅ 授权成功！欢迎使用 OpenClaw。'
                );

                return true;
            }
        }

        return false;
    }
}
```

### CLI 命令

```bash
# 批准配对
openclaw pairing approve 123456

# 列出待配对请求
openclaw pairing list

# 查看白名单
openclaw whitelist

# 移除白名单
openclaw whitelist remove whatsapp:1234567890
```

### CLI 实现

```typescript
// CLI 命令处理
if (command === 'pairing' && subcommand === 'approve') {
    const code = args[0];

    if (!code || code.length !== 6) {
        console.error('Invalid code. Usage: openclaw pairing approve <6-digit-code>');
        process.exit(1);
    }

    const success = await gateway.pairing.approvePairing(code);

    if (success) {
        console.log('✅ Pairing approved');
    } else {
        console.error('❌ Invalid or expired code');
        process.exit(1);
    }
}
```

## 沙箱与权限控制

### 工具白名单

用户可以配置允许 AI 使用的工具列表。

```json
// tools-config.json
{
  "allowed_tools": [
    "send_message",
    "read_file",
    "search_web"
  ],
  "forbidden_tools": [
    "execute_shell",
    "delete_file",
    "modify_database"
  ],
  "high_risk_tools": [
    "send_money",
    "delete_resource"
  ]
}
```

### 工具拦截

```typescript
class ToolExecutor {
    private config: ToolsConfig;

    async execute(toolCall: ToolCall): Promise<any> {
        // 1. 检查是否在白名单
        if (!this.config.allowed_tools.includes(toolCall.name)) {
            throw new Error(`Tool ${toolCall.name} is not allowed`);
        }

        // 2. 检查是否在黑名单
        if (this.config.forbidden_tools.includes(toolCall.name)) {
            throw new Error(`Tool ${toolCall.name} is forbidden`);
        }

        // 3. 高风险工具需要审批
        if (this.config.high_risk_tools.includes(toolCall.name)) {
            const approved = await this.requestApproval(toolCall);

            if (!approved) {
                throw new Error('User rejected the action');
            }
        }

        // 4. 执行工具
        return await this.callTool(toolCall);
    }

    private async requestApproval(toolCall: ToolCall): Promise<boolean> {
        // 发送审批请求到用户
        const message = `
⚠️ 高风险操作需要批准：

工具: ${toolCall.name}
参数: ${JSON.stringify(toolCall.args, null, 2)}

回复 "approve" 批准，或 "reject" 拒绝。
        `.trim();

        await this.sendMessage(message);

        // 等待用户响应（最多 5 分钟）
        const response = await this.waitForResponse(5 * 60 * 1000);

        return response.content.toLowerCase().includes('approve');
    }
}
```

### Docker 沙箱

对于极度危险的操作，使用 Docker 容器隔离：

```typescript
class DockerSandbox {
    async executeShellCommand(command: string): Promise<string> {
        // 在临时容器中执行
        const result = await exec(`
            docker run --rm \
                --network none \
                --read-only \
                --tmpfs /tmp \
                alpine:latest \
                sh -c ${JSON.stringify(command)}
        `);

        return result.stdout;
    }
}
```

## 输入验证

### SQL 注入防护

```typescript
// ❌ 错误：直接拼接 SQL
const query = `SELECT * FROM users WHERE name = '${userInput}'`;

// ✅ 正确：使用参数化查询
const query = 'SELECT * FROM users WHERE name = ?';
const result = await db.all(query, [userInput]);
```

### 命令注入防护

```typescript
// ❌ 错误：直接执行用户输入
exec(`curl ${userInput}`);

// ✅ 正确：验证和转义
function safeCurl(url: string): Promise<string> {
    // 验证 URL 格式
    const urlObj = new URL(url);

    // 只允许 HTTP(S)
    if (!['http:', 'https:'].includes(urlObj.protocol)) {
        throw new Error('Invalid protocol');
    }

    // 使用库而非 shell
    return fetch(url).then(r => r.text());
}
```

### 路径遍历防护

```typescript
function safeReadFile(requestedPath: string): Promise<string> {
    // 解析路径
    const absolutePath = path.resolve('./data', requestedPath);

    // 确保在允许的目录内
    if (!absolutePath.startsWith(path.resolve('./data'))) {
        throw new Error('Path traversal detected');
    }

    return fs.readFile(absolutePath, 'utf-8');
}
```

## 速率限制

### 用户级限流

```typescript
class RateLimiter {
    private limits = new Map<string, TokenBucket>();

    async checkLimit(userId: string): Promise<boolean> {
        let bucket = this.limits.get(userId);

        if (!bucket) {
            // 每用户每分钟 10 条消息
            bucket = new TokenBucket(10, 60 * 1000);
            this.limits.set(userId, bucket);
        }

        return bucket.consume();
    }
}

class TokenBucket {
    private tokens: number;
    private lastRefill: number;

    constructor(
        private capacity: number,
        private refillInterval: number
    ) {
        this.tokens = capacity;
        this.lastRefill = Date.now();
    }

    consume(): boolean {
        this.refill();

        if (this.tokens > 0) {
            this.tokens--;
            return true;
        }

        return false;
    }

    private refill() {
        const now = Date.now();
        const elapsed = now - this.lastRefill;
        const tokensToAdd = Math.floor(elapsed / this.refillInterval) * this.capacity;

        if (tokensToAdd > 0) {
            this.tokens = Math.min(this.capacity, this.tokens + tokensToAdd);
            this.lastRefill = now;
        }
    }
}
```

### 全局限流

```typescript
// 使用 Redis 实现分布式限流
class DistributedRateLimiter {
    async checkLimit(key: string, limit: number, window: number): Promise<boolean> {
        const current = await redis.incr(key);

        if (current === 1) {
            await redis.expire(key, window);
        }

        return current <= limit;
    }
}

// 使用示例
const allowed = await limiter.checkLimit('global:messages', 1000, 60);
```

## API 密钥管理

### 密钥轮换

```json
// auth-profiles.json (加密存储)
{
  "profiles": [
    {
      "id": "primary",
      "provider": "anthropic",
      "apiKey": "sk-ant-xxx",
      "rotateAfter": "2024-02-01T00:00:00Z"
    },
    {
      "id": "backup",
      "provider": "openai",
      "apiKey": "sk-xxx",
      "rotateAfter": "2024-03-01T00:00:00Z"
    }
  ]
}
```

### 环境变量

```bash
# .env (不提交到 Git)
ANTHROPIC_API_KEY=sk-ant-xxx
OPENAI_API_KEY=sk-xxx
TELEGRAM_BOT_TOKEN=123456:ABC-DEF
WHATSAPP_SESSION_ID=xxx
```

### 加密存储

```typescript
import crypto from 'crypto';

class SecretManager {
    private key: Buffer;

    constructor(masterPassword: string) {
        // 从主密码派生密钥
        this.key = crypto.scryptSync(masterPassword, 'salt', 32);
    }

    encrypt(plaintext: string): string {
        const iv = crypto.randomBytes(16);
        const cipher = crypto.createCipheriv('aes-256-cbc', this.key, iv);

        let encrypted = cipher.update(plaintext, 'utf-8', 'hex');
        encrypted += cipher.final('hex');

        return iv.toString('hex') + ':' + encrypted;
    }

    decrypt(ciphertext: string): string {
        const [ivHex, encrypted] = ciphertext.split(':');
        const iv = Buffer.from(ivHex, 'hex');

        const decipher = crypto.createDecipheriv('aes-256-cbc', this.key, iv);

        let decrypted = decipher.update(encrypted, 'hex', 'utf-8');
        decrypted += decipher.final('utf-8');

        return decrypted;
    }
}
```

## 审计日志

### 记录关键操作

```typescript
class AuditLogger {
    async log(event: AuditEvent) {
        await db.run(
            `INSERT INTO audit_log
             (timestamp, user_id, action, details, ip_address)
             VALUES (?, ?, ?, ?, ?)`,
            [
                new Date().toISOString(),
                event.userId,
                event.action,
                JSON.stringify(event.details),
                event.ipAddress,
            ]
        );
    }
}

// 使用示例
await auditLogger.log({
    userId: 'alice@example.com',
    action: 'whitelist.add',
    details: {
        addedUser: '1234567890@s.whatsapp.net',
        platform: 'whatsapp',
    },
    ipAddress: '192.168.1.100',
});
```

### 审计查询

```sql
-- 查看最近的高风险操作
SELECT * FROM audit_log
WHERE action IN ('whitelist.add', 'tool.execute', 'config.update')
ORDER BY timestamp DESC
LIMIT 100;

-- 查看特定用户的操作历史
SELECT * FROM audit_log
WHERE user_id = 'alice@example.com'
ORDER BY timestamp DESC;
```

## 安全最佳实践

### 部署检查清单

- ☐ 启用 DM 配对机制
- ☐ 配置工具白名单
- ☐ 启用高风险操作审批
- ☐ 设置速率限制
- ☐ 加密存储 API 密钥
- ☐ 定期轮换密钥
- ☐ 启用审计日志
- ☐ 使用 HTTPS/WSS
- ☐ 限制网络访问（防火墙）
- ☐ 定期备份数据

### 监控告警

```typescript
// 异常检测
if (failedLoginAttempts > 5) {
    await sendAlert('Multiple failed login attempts detected');
}

if (apiCallsPerMinute > 100) {
    await sendAlert('Unusual API usage detected');
}

if (diskUsage > 90) {
    await sendAlert('Low disk space');
}
```

---

**相关文档**:
- [安全模块实现](../04-go-implementation/security-impl.md)
- [部署安全指南](../05-roadmap/implementation-plan.md#security)
