# LLM 编排系统

## 模型无关设计

OpenClaw 并不绑定单一的 AI 模型，而是作为一个**模型无关（Model-Agnostic）**的编排层。

### 设计哲学

```
应用层 → 抽象层 → 提供商层
         ↓
    统一接口屏蔽差异
```

## 支持的模型提供商

| 提供商 | 模型示例 | 特点 |
|--------|----------|------|
| **OpenAI** | GPT-4o, GPT-4o-mini | 通用能力强，API 稳定 |
| **Anthropic** | Claude 3.5 Sonnet, Haiku | 推理能力出色，上下文长 |
| **Google** | Gemini 2.0 Flash | 多模态，速度快 |
| **Local** | Llama 3, Qwen | 数据私密，无网络依赖 |

## 统一接口抽象

### 接口定义

```typescript
interface LLMProvider {
    name: string;
    models: string[];

    // 聊天补全
    chat(params: ChatParams): Promise<ChatResponse>;

    // 流式响应
    chatStream(params: ChatParams): AsyncIterable<ChatChunk>;

    // 工具调用
    callTools(params: ToolCallParams): Promise<ToolCallResponse>;
}
```

### 参数标准化

```typescript
interface ChatParams {
    model: string;
    messages: Message[];
    temperature?: number;
    maxTokens?: number;
    tools?: Tool[];
    systemPrompt?: string;
}

interface Message {
    role: 'user' | 'assistant' | 'system';
    content: string;
}
```

### 提供商实现

#### OpenAI 适配器

```typescript
class OpenAIProvider implements LLMProvider {
    name = 'openai';
    models = ['gpt-4o', 'gpt-4o-mini'];

    async chat(params: ChatParams): Promise<ChatResponse> {
        const response = await fetch('https://api.openai.com/v1/chat/completions', {
            method: 'POST',
            headers: {
                'Authorization': `Bearer ${this.apiKey}`,
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                model: params.model,
                messages: params.messages,
                temperature: params.temperature || 0.7,
                max_tokens: params.maxTokens || 2000,
            }),
        });

        const data = await response.json();

        return {
            content: data.choices[0].message.content,
            usage: {
                promptTokens: data.usage.prompt_tokens,
                completionTokens: data.usage.completion_tokens,
            },
        };
    }
}
```

#### Anthropic 适配器

```typescript
class AnthropicProvider implements LLMProvider {
    name = 'anthropic';
    models = ['claude-3-5-sonnet-20241022', 'claude-3-5-haiku-20241022'];

    async chat(params: ChatParams): Promise<ChatResponse> {
        // 注意：Anthropic 的 system prompt 是单独参数
        const { systemPrompt, messages, ...rest } = params;

        const response = await fetch('https://api.anthropic.com/v1/messages', {
            method: 'POST',
            headers: {
                'x-api-key': this.apiKey,
                'anthropic-version': '2023-06-01',
                'content-type': 'application/json',
            },
            body: JSON.stringify({
                model: params.model,
                system: systemPrompt,
                messages: messages.filter(m => m.role !== 'system'),
                max_tokens: params.maxTokens || 4096,
                temperature: params.temperature || 1.0,
            }),
        });

        const data = await response.json();

        return {
            content: data.content[0].text,
            usage: {
                promptTokens: data.usage.input_tokens,
                completionTokens: data.usage.output_tokens,
            },
        };
    }
}
```

## 认证配置文件轮换

为了保证生产环境的高可用性，OpenClaw 实现了认证配置轮换机制。

### 配置文件结构

```json
// auth-profiles.json
{
  "profiles": [
    {
      "id": "primary",
      "provider": "anthropic",
      "model": "claude-3-5-sonnet-20241022",
      "apiKey": "sk-ant-xxx",
      "priority": 1,
      "rateLimit": {
        "requestsPerMinute": 50,
        "tokensPerMinute": 40000
      }
    },
    {
      "id": "fallback",
      "provider": "openai",
      "model": "gpt-4o",
      "apiKey": "sk-xxx",
      "priority": 2,
      "rateLimit": {
        "requestsPerMinute": 100,
        "tokensPerMinute": 90000
      }
    },
    {
      "id": "local",
      "provider": "ollama",
      "model": "llama3:70b",
      "endpoint": "http://localhost:11434",
      "priority": 3
    }
  ]
}
```

### 轮换逻辑

```typescript
class AuthProfileManager {
    private profiles: AuthProfile[];
    private currentIndex = 0;

    async getActiveProfile(): Promise<AuthProfile> {
        const profile = this.profiles[this.currentIndex];

        // 检查速率限制
        if (await this.isRateLimited(profile)) {
            logger.warn(`Profile ${profile.id} rate limited, switching...`);
            return this.switchToNextProfile();
        }

        // 检查健康状态
        if (!(await this.healthCheck(profile))) {
            logger.error(`Profile ${profile.id} unhealthy, switching...`);
            return this.switchToNextProfile();
        }

        return profile;
    }

    private switchToNextProfile(): AuthProfile {
        this.currentIndex = (this.currentIndex + 1) % this.profiles.length;
        return this.profiles[this.currentIndex];
    }

    private async healthCheck(profile: AuthProfile): Promise<boolean> {
        try {
            const provider = this.getProvider(profile);
            await provider.chat({
                model: profile.model,
                messages: [{ role: 'user', content: 'ping' }],
                maxTokens: 10,
            });
            return true;
        } catch (error) {
            return false;
        }
    }
}
```

### 自动降级（Failover）

```typescript
async function callLLMWithFailover(params: ChatParams): Promise<ChatResponse> {
    const maxAttempts = authManager.profiles.length;

    for (let attempt = 0; attempt < maxAttempts; attempt++) {
        const profile = await authManager.getActiveProfile();
        const provider = getProvider(profile.provider);

        try {
            logger.info(`Using profile: ${profile.id}`);

            const response = await provider.chat({
                ...params,
                model: profile.model,
            });

            return response;
        } catch (error) {
            logger.error(`Profile ${profile.id} failed:`, error);

            if (attempt < maxAttempts - 1) {
                logger.info('Failing over to next profile...');
                await authManager.switchToNextProfile();
            } else {
                throw new Error('All profiles exhausted');
            }
        }
    }
}
```

## 智能路由

系统支持基于任务复杂度的路由，优化成本和效果。

### 路由策略

```typescript
interface RoutingRule {
    condition: (params: ChatParams) => boolean;
    targetModel: string;
    reason: string;
}

const routingRules: RoutingRule[] = [
    {
        condition: (params) => params.messages.length <= 2,
        targetModel: 'gpt-4o-mini',
        reason: '简单对话，使用低成本模型',
    },
    {
        condition: (params) => params.tools && params.tools.length > 0,
        targetModel: 'gpt-4o',
        reason: '工具调用，需要高精度',
    },
    {
        condition: (params) => estimateComplexity(params) > 0.8,
        targetModel: 'claude-3-5-sonnet-20241022',
        reason: '复杂推理任务，使用最强模型',
    },
];
```

### 复杂度评估

```typescript
function estimateComplexity(params: ChatParams): number {
    let score = 0;

    // 对话轮次
    score += params.messages.length * 0.1;

    // 输入长度
    const totalLength = params.messages.reduce(
        (sum, m) => sum + m.content.length,
        0
    );
    score += (totalLength / 10000) * 0.3;

    // 工具使用
    if (params.tools && params.tools.length > 0) {
        score += 0.4;
    }

    // 特定关键词
    const content = params.messages.map(m => m.content).join(' ');
    const complexKeywords = ['分析', '推理', '设计', '架构'];
    if (complexKeywords.some(k => content.includes(k))) {
        score += 0.2;
    }

    return Math.min(score, 1.0);
}
```

### 路由执行

```typescript
function selectModel(params: ChatParams): string {
    for (const rule of routingRules) {
        if (rule.condition(params)) {
            logger.info(`Routing: ${rule.reason} → ${rule.targetModel}`);
            return rule.targetModel;
        }
    }

    // 默认模型
    return 'gpt-4o';
}
```

## 成本优化

### Token 计数

```typescript
import { encoding_for_model } from 'tiktoken';

function countTokens(text: string, model: string): number {
    const encoding = encoding_for_model(model);
    const tokens = encoding.encode(text);
    encoding.free();
    return tokens.length;
}
```

### 成本跟踪

```typescript
interface UsageStats {
    provider: string;
    model: string;
    promptTokens: number;
    completionTokens: number;
    cost: number;
}

class CostTracker {
    private stats: Map<string, UsageStats> = new Map();

    track(provider: string, model: string, usage: Usage) {
        const cost = this.calculateCost(provider, model, usage);

        const key = `${provider}:${model}`;
        const current = this.stats.get(key) || {
            provider,
            model,
            promptTokens: 0,
            completionTokens: 0,
            cost: 0,
        };

        current.promptTokens += usage.promptTokens;
        current.completionTokens += usage.completionTokens;
        current.cost += cost;

        this.stats.set(key, current);
    }

    private calculateCost(provider: string, model: string, usage: Usage): number {
        const pricing = {
            'openai:gpt-4o': {
                prompt: 2.5 / 1_000_000,    // $2.50 per 1M tokens
                completion: 10 / 1_000_000, // $10 per 1M tokens
            },
            'anthropic:claude-3-5-sonnet-20241022': {
                prompt: 3 / 1_000_000,
                completion: 15 / 1_000_000,
            },
        };

        const price = pricing[`${provider}:${model}`];
        if (!price) return 0;

        return (
            usage.promptTokens * price.prompt +
            usage.completionTokens * price.completion
        );
    }

    getSummary(): UsageStats[] {
        return Array.from(this.stats.values());
    }
}
```

## 缓存策略

### 语义缓存

```typescript
class SemanticCache {
    private cache: Map<string, CacheEntry> = new Map();

    async get(prompt: string): Promise<string | null> {
        // 计算 embedding
        const embedding = await getEmbedding(prompt);

        // 查找相似缓存
        for (const [key, entry] of this.cache) {
            const similarity = cosineSimilarity(embedding, entry.embedding);

            if (similarity > 0.95) {
                logger.info('Cache hit', { similarity });
                return entry.response;
            }
        }

        return null;
    }

    set(prompt: string, response: string, embedding: number[]) {
        this.cache.set(prompt, {
            response,
            embedding,
            timestamp: Date.now(),
        });

        // 清理过期缓存
        this.evictOldEntries();
    }

    private evictOldEntries() {
        const maxAge = 24 * 60 * 60 * 1000; // 24 小时
        const now = Date.now();

        for (const [key, entry] of this.cache) {
            if (now - entry.timestamp > maxAge) {
                this.cache.delete(key);
            }
        }
    }
}
```

---

**相关文档**:
- [LLM 客户端实现](../04-go-implementation/gateway-impl.md#llm-clients)
- [认证配置管理](../03-core-features/security.md#api-key-management)
