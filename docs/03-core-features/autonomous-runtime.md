# 自主运行机制

## 设计目标

OpenClaw 区别于传统 Chatbot 的核心在于其**主动性（Proactivity）**：

```
传统 Bot：等待 → 响应 → 休眠
OpenClaw：监控 → 思考 → 行动 → 循环
```

## 守护进程架构

### 操作系统服务

通过操作系统的服务管理器，OpenClaw 被配置为开机自启的守护进程。

#### macOS (launchd)

```xml
<!-- ~/Library/LaunchAgents/com.openclaw.gateway.plist -->
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.openclaw.gateway</string>

    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/openclaw</string>
        <string>start</string>
    </array>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>

    <key>StandardOutPath</key>
    <string>/var/log/openclaw.log</string>

    <key>StandardErrorPath</key>
    <string>/var/log/openclaw.error.log</string>
</dict>
</plist>
```

**安装命令**：

```bash
launchctl load ~/Library/LaunchAgents/com.openclaw.gateway.plist
launchctl start com.openclaw.gateway
```

#### Linux (systemd)

```ini
# /etc/systemd/system/openclaw.service
[Unit]
Description=OpenClaw AI Gateway
After=network.target

[Service]
Type=simple
User=openclaw
WorkingDirectory=/opt/openclaw
ExecStart=/usr/local/bin/openclaw start
Restart=always
RestartSec=10

# 环境变量
Environment="NODE_ENV=production"
Environment="OPENCLAW_HOME=/opt/openclaw"

# 日志
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

**管理命令**：

```bash
sudo systemctl enable openclaw    # 开机自启
sudo systemctl start openclaw     # 启动服务
sudo systemctl status openclaw    # 查看状态
journalctl -u openclaw -f         # 查看日志
```

### 持久化意义

即便用户关闭了终端窗口，AI 依然在后台运行，维持着与外部世界的连接。

**应用场景**：

- 📧 后台监控邮箱，收到重要邮件立即通知
- 🔍 定时检查服务器状态，异常时主动告警
- 📊 每天早上生成昨日数据报告
- 💬 持续维护与 WhatsApp/Telegram 的连接

## 进程生命周期管理

### 启动流程

```
1. 加载配置文件
   ↓
2. 初始化数据库连接
   ↓
3. 启动网关 WebSocket 服务器
   ↓
4. 连接聊天平台（WhatsApp/Telegram/Discord）
   ↓
5. 启动心跳定时器
   ↓
6. 进入主事件循环
```

### 代码示例

```typescript
async function main() {
    // 1. 加载配置
    const config = await loadConfig();

    // 2. 初始化组件
    const database = await initDatabase(config.dbPath);
    const gateway = new Gateway(config.gateway);
    const scheduler = new Scheduler();

    // 3. 注册信号处理
    process.on('SIGTERM', async () => {
        logger.info('Received SIGTERM, shutting down gracefully...');
        await shutdown();
    });

    process.on('SIGINT', async () => {
        logger.info('Received SIGINT, shutting down gracefully...');
        await shutdown();
    });

    // 4. 启动服务
    await gateway.start();
    await scheduler.startHeartbeat();

    logger.info('OpenClaw started successfully');

    // 5. 保持进程运行
    await keepAlive();
}

async function shutdown() {
    // 停止接收新请求
    await gateway.stop();

    // 等待处理中的任务完成
    await gateway.drain();

    // 关闭数据库连接
    await database.close();

    // 退出进程
    process.exit(0);
}
```

### 优雅关闭

确保在进程退出时正确清理资源：

```typescript
class Gateway {
    private isShuttingDown = false;
    private activeRequests = 0;

    async drain() {
        this.isShuttingDown = true;

        // 等待所有活跃请求完成
        while (this.activeRequests > 0) {
            logger.info(`Waiting for ${this.activeRequests} requests to complete...`);
            await sleep(1000);
        }

        logger.info('All requests completed');
    }

    async handleRequest(request: Request) {
        if (this.isShuttingDown) {
            throw new Error('Server is shutting down');
        }

        this.activeRequests++;
        try {
            await this.processRequest(request);
        } finally {
            this.activeRequests--;
        }
    }
}
```

## 健康检查

### HTTP 健康端点

```typescript
// 暴露健康检查端点
app.get('/health', (req, res) => {
    const health = {
        status: 'ok',
        uptime: process.uptime(),
        timestamp: new Date().toISOString(),
        connections: {
            whatsapp: whatsappConnector.isConnected(),
            telegram: telegramConnector.isConnected(),
            discord: discordConnector.isConnected(),
        },
        memory: process.memoryUsage(),
    };

    const isHealthy = Object.values(health.connections).every(c => c);

    res.status(isHealthy ? 200 : 503).json(health);
});
```

### 外部监控集成

```bash
# Kubernetes liveness probe
kubectl exec openclaw-pod -- curl -f http://localhost:8080/health

# Docker Compose health check
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
  interval: 30s
  timeout: 10s
  retries: 3
```

## 日志系统

### 结构化日志

```typescript
import winston from 'winston';

const logger = winston.createLogger({
    level: process.env.LOG_LEVEL || 'info',
    format: winston.format.combine(
        winston.format.timestamp(),
        winston.format.json()
    ),
    transports: [
        // 控制台输出
        new winston.transports.Console({
            format: winston.format.combine(
                winston.format.colorize(),
                winston.format.simple()
            ),
        }),

        // 文件输出
        new winston.transports.File({
            filename: 'logs/error.log',
            level: 'error',
        }),
        new winston.transports.File({
            filename: 'logs/combined.log',
        }),
    ],
});
```

### 日志级别

| 级别 | 用途 | 示例 |
|------|------|------|
| **error** | 错误和异常 | API 调用失败 |
| **warn** | 警告信息 | 速率限制即将触发 |
| **info** | 重要事件 | 用户登录、任务完成 |
| **debug** | 调试信息 | 函数调用参数 |
| **trace** | 详细追踪 | 完整的请求/响应体 |

### 日志示例

```typescript
logger.info('Message received', {
    source: 'whatsapp',
    sessionId: '1234567890@s.whatsapp.net',
    messageLength: 150,
});

logger.error('Failed to send message', {
    error: error.message,
    stack: error.stack,
    sessionId: '...',
});
```

## 监控指标

### 核心指标

```typescript
import * as prometheus from 'prom-client';

// 请求计数器
const requestCounter = new prometheus.Counter({
    name: 'openclaw_requests_total',
    help: 'Total number of requests',
    labelNames: ['source', 'type'],
});

// 响应时间直方图
const responseTime = new prometheus.Histogram({
    name: 'openclaw_response_duration_seconds',
    help: 'Response duration in seconds',
    labelNames: ['source'],
    buckets: [0.1, 0.5, 1, 2, 5, 10],
});

// Gauge：当前活跃会话数
const activeSessions = new prometheus.Gauge({
    name: 'openclaw_active_sessions',
    help: 'Number of active sessions',
});
```

### Prometheus 导出

```typescript
app.get('/metrics', async (req, res) => {
    res.set('Content-Type', prometheus.register.contentType);
    res.end(await prometheus.register.metrics());
});
```

### Grafana 仪表盘

```yaml
# 示例查询
# 每秒请求数
rate(openclaw_requests_total[5m])

# P95 响应时间
histogram_quantile(0.95, openclaw_response_duration_seconds_bucket)

# 活跃会话数
openclaw_active_sessions
```

## 资源限制

### 内存限制

```bash
# Docker
docker run --memory=512m openclaw

# systemd
[Service]
MemoryLimit=512M
MemoryAccounting=true
```

### CPU 限制

```bash
# Docker
docker run --cpus=2 openclaw

# systemd
[Service]
CPUQuota=200%
```

### 文件描述符

```bash
# 检查当前限制
ulimit -n

# 设置更高的限制
ulimit -n 65536

# 永久生效 (/etc/security/limits.conf)
openclaw soft nofile 65536
openclaw hard nofile 65536
```

## 故障恢复

### 自动重启

```ini
# systemd
[Service]
Restart=always
RestartSec=10
StartLimitBurst=5
StartLimitIntervalSec=300
```

### 状态恢复

```typescript
class StatefulGateway {
    private stateFile = './data/gateway-state.json';

    async saveState() {
        const state = {
            activeSessions: Array.from(this.sessions.keys()),
            lastHeartbeat: this.lastHeartbeat,
            config: this.config,
        };

        await fs.writeFile(this.stateFile, JSON.stringify(state, null, 2));
    }

    async restoreState() {
        if (!fs.existsSync(this.stateFile)) {
            return;
        }

        const state = JSON.parse(await fs.readFile(this.stateFile, 'utf-8'));

        // 恢复会话
        for (const sessionId of state.activeSessions) {
            this.sessions.set(sessionId, this.createLane(sessionId));
        }

        logger.info('State restored', {
            sessions: state.activeSessions.length,
        });
    }
}
```

---

**相关文档**:
- [心跳系统](heartbeat.md)
- [部署指南](../05-roadmap/implementation-plan.md#deployment)
