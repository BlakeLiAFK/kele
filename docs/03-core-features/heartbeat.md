# 心跳系统

## 核心理念

传统的自动化依赖 Cron 任务，按固定时间机械执行。OpenClaw 引入了更为智能的**心跳机制（Heartbeat）**。

### Heartbeat vs. Cron

| 特性 | Cron | Heartbeat |
|------|------|-----------|
| **触发方式** | 固定时间执行命令 | 周期性唤醒 AI 决策 |
| **上下文感知** | 无 | 携带系统快照 |
| **灵活性** | 静态脚本 | AI 动态判断 |
| **学习能力** | 无 | 可根据历史调整 |

### 工作原理

```
定时器触发
    ↓
生成系统快照
    ↓
唤醒 AI 大脑
    ↓
AI 决策：是否需要行动？
    ↓
执行 / 休眠
```

## 心跳配置

### HEARTBEAT.md 文件

这是 AI 的自主行为准则，包含自然语言描述的长期任务清单。

```markdown
# 心跳任务配置

## 每小时任务

- 检查服务器 CPU 使用率，如果超过 80% 发送告警
- 检查 /var/log/errors.log 是否有新错误

## 每天早上 9:00

- 生成昨日数据统计报告
- 发送到 WhatsApp 群组
- 检查备份是否成功

## 每周一早上

- 总结上周工作进展
- 发送周报到 Telegram

## 实时监控

- 如果检测到磁盘空间 < 10%，立即告警
- 如果网站响应时间 > 5s，立即告警
```

### 配置特点

- ✅ **自然语言**：无需编程，直接描述意图
- ✅ **灵活**：AI 理解上下文，而非死板执行
- ✅ **可维护**：修改文件即可更新任务

## 心跳流程

### 1. 定时触发

```typescript
class Scheduler {
    private heartbeatInterval: NodeJS.Timeout;

    startHeartbeat(intervalMinutes: number = 15) {
        this.heartbeatInterval = setInterval(
            () => this.triggerHeartbeat(),
            intervalMinutes * 60 * 1000
        );

        logger.info(`Heartbeat started`, { intervalMinutes });
    }

    private async triggerHeartbeat() {
        const event: HeartbeatEvent = {
            type: 'system.heartbeat',
            source: 'scheduler',
            sessionId: 'system_main',
            timestamp: new Date(),
            payload: await this.generateSnapshot(),
        };

        gateway.pushEvent(event);
    }
}
```

### 2. 生成系统快照

```typescript
interface SystemSnapshot {
    time: {
        current: Date;
        timezone: string;
        dayOfWeek: string;
    };
    system: {
        uptime: number;
        cpu: number;
        memory: number;
        disk: number;
    };
    notifications: {
        unreadMessages: number;
        pendingTasks: number;
    };
    context: {
        recentErrors: string[];
        activeSessions: number;
    };
}

async function generateSnapshot(): Promise<SystemSnapshot> {
    return {
        time: {
            current: new Date(),
            timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
            dayOfWeek: new Date().toLocaleDateString('zh-CN', { weekday: 'long' }),
        },
        system: {
            uptime: process.uptime(),
            cpu: await getCPUUsage(),
            memory: process.memoryUsage().heapUsed / 1024 / 1024,
            disk: await getDiskUsage(),
        },
        notifications: {
            unreadMessages: await countUnreadMessages(),
            pendingTasks: await countPendingTasks(),
        },
        context: {
            recentErrors: await getRecentErrors(1 * 60 * 60 * 1000), // 最近 1 小时
            activeSessions: gateway.getActiveSessionCount(),
        },
    };
}
```

### 3. AI 决策

```typescript
class AgentBrain {
    async handleHeartbeat(event: HeartbeatEvent) {
        // 读取心跳配置
        const heartbeatConfig = await fs.readFile('./HEARTBEAT.md', 'utf-8');

        // 构建提示词
        const prompt = `
你是 OpenClaw 智能助手。现在是系统心跳时刻。

当前时间: ${event.payload.time.current}
星期: ${event.payload.time.dayOfWeek}

系统状态:
- CPU 使用率: ${event.payload.system.cpu}%
- 内存使用: ${event.payload.system.memory.toFixed(2)} MB
- 磁盘使用率: ${event.payload.system.disk}%

未读通知: ${event.payload.notifications.unreadMessages}
待办任务: ${event.payload.notifications.pendingTasks}

最近错误: ${event.payload.context.recentErrors.join(', ') || '无'}

根据以下心跳配置，判断现在是否需要执行某些任务：

${heartbeatConfig}

如果有需要执行的任务，请说明并执行。如果没有，回复"无需行动"。
        `.trim();

        // 调用 LLM
        const response = await this.llm.chat({
            messages: [{ role: 'user', content: prompt }],
            tools: this.getAvailableTools(),
        });

        // 执行 AI 决策的操作
        if (response.toolCalls) {
            for (const toolCall of response.toolCalls) {
                await this.executeTool(toolCall);
            }
        }

        logger.info('Heartbeat processed', {
            decision: response.content,
        });
    }
}
```

### 4. 工具执行

AI 可以调用的工具：

```typescript
const tools = [
    {
        name: 'send_message',
        description: '发送消息到指定平台和用户',
        parameters: {
            type: 'object',
            properties: {
                platform: { type: 'string', enum: ['whatsapp', 'telegram'] },
                recipient: { type: 'string' },
                message: { type: 'string' },
            },
        },
    },
    {
        name: 'check_server_status',
        description: '检查服务器状态',
        parameters: {
            type: 'object',
            properties: {
                url: { type: 'string' },
            },
        },
    },
    {
        name: 'read_log_file',
        description: '读取日志文件',
        parameters: {
            type: 'object',
            properties: {
                path: { type: 'string' },
                lines: { type: 'number', default: 100 },
            },
        },
    },
];
```

## 主动交互示例

### 场景 1：磁盘空间告警

```
心跳检测 → 磁盘使用 95% → AI 判断需要告警

AI 执行:
  send_message({
    platform: 'whatsapp',
    recipient: 'admin@phone.number',
    message: '⚠️ 警告：服务器磁盘空间仅剩 5%，请立即清理！'
  })
```

### 场景 2：每日报告

```
心跳检测 → 当前时间 09:00 → AI 判断需要生成报告

AI 执行:
  1. read_log_file({ path: '/var/log/analytics.log' })
  2. 分析数据
  3. send_message({
       platform: 'telegram',
       recipient: 'team_group_id',
       message: '📊 昨日数据报告：\n访问量: 10,523\n新用户: 234\n...'
     })
```

### 场景 3：服务异常检测

```
心跳检测 → 最近错误日志有 "Connection refused"

AI 执行:
  1. check_server_status({ url: 'https://api.example.com' })
  2. 确认服务宕机
  3. send_message({
       platform: 'whatsapp',
       recipient: 'oncall_engineer',
       message: '🚨 紧急：API 服务无响应，请立即排查！'
     })
```

## 心跳调度策略

### 自适应间隔

根据系统负载动态调整心跳频率：

```typescript
class AdaptiveScheduler {
    private baseInterval = 15; // 基准 15 分钟
    private currentInterval = 15;

    adjustInterval() {
        const load = getSystemLoad();

        if (load > 0.8) {
            // 高负载，降低心跳频率
            this.currentInterval = Math.min(this.currentInterval * 1.5, 60);
        } else if (load < 0.3) {
            // 低负载，提高心跳频率
            this.currentInterval = Math.max(this.currentInterval * 0.8, 5);
        }

        logger.info('Heartbeat interval adjusted', {
            interval: this.currentInterval,
            load,
        });
    }
}
```

### 智能静默

夜间或用户离线时降低心跳频率：

```typescript
function getHeartbeatInterval(): number {
    const hour = new Date().getHours();

    // 夜间 (23:00 - 07:00) 降低频率
    if (hour >= 23 || hour < 7) {
        return 60; // 60 分钟
    }

    // 工作时间 (09:00 - 18:00) 正常频率
    if (hour >= 9 && hour < 18) {
        return 15; // 15 分钟
    }

    // 其他时间
    return 30; // 30 分钟
}
```

## 心跳历史记录

### 记录执行结果

```typescript
interface HeartbeatRecord {
    timestamp: Date;
    snapshot: SystemSnapshot;
    decision: string;
    actions: ToolCall[];
    duration: number;
}

class HeartbeatLogger {
    private records: HeartbeatRecord[] = [];

    async log(record: HeartbeatRecord) {
        this.records.push(record);

        // 持久化到数据库
        await db.insert('heartbeat_history', record);

        // 保留最近 100 条记录
        if (this.records.length > 100) {
            this.records.shift();
        }
    }

    async getRecentRecords(count: number = 10): Promise<HeartbeatRecord[]> {
        return this.records.slice(-count);
    }
}
```

### 统计分析

```typescript
async function analyzeHeartbeatEffectiveness() {
    const records = await db.query('SELECT * FROM heartbeat_history WHERE timestamp > ?', [
        new Date(Date.now() - 7 * 24 * 60 * 60 * 1000), // 最近 7 天
    ]);

    const stats = {
        totalHeartbeats: records.length,
        actionTaken: records.filter(r => r.actions.length > 0).length,
        avgResponseTime: records.reduce((sum, r) => sum + r.duration, 0) / records.length,
        topActions: countBy(records.flatMap(r => r.actions.map(a => a.name))),
    };

    return stats;
}
```

## 心跳与 Cron 的协作

在某些场景下，Heartbeat 和 Cron 可以协同工作：

```typescript
// Cron: 精确定时执行
cron.schedule('0 9 * * 1', () => {
    // 每周一早上 9:00 触发特殊心跳
    triggerHeartbeat({
        type: 'weekly_review',
    });
});

// Heartbeat: 智能决策
if (event.type === 'weekly_review') {
    // AI 生成周报，但具体内容由 AI 决定
}
```

---

**相关文档**:
- [自主运行机制](autonomous-runtime.md)
- [心跳实现](../04-go-implementation/heartbeat-impl.md)
