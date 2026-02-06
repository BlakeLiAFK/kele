# 记忆系统

## 设计哲学

AI 的连贯性依赖于记忆。OpenClaw 采用了一套独特的**文件系统与数据库混合**的记忆架构，兼顾了人类可读性与机器检索效率。

### 核心原则

```
文件即真理 (File-First Philosophy)
    +
高效检索 (Hybrid Search)
    =
透明且强大的记忆系统
```

## 文件即真理

### 设计理念

所有的长期记忆、用户画像和会话记录都以 Markdown 或 JSONL 文件形式存储在本地磁盘上。

**优势**：

- ✅ **人类可读**：用文本编辑器直接查看和修改
- ✅ **透明性**：用户完全了解 AI 知道什么
- ✅ **可移植**：纯文本格式，永不过时
- ✅ **版本控制**：可用 Git 跟踪记忆变化
- ✅ **隐私保护**：数据完全在本地，不依赖云端

### 文件结构

```
data/
├── MEMORY.md                # 长期知识和用户画像
├── HEARTBEAT.md             # 心跳任务配置
├── sessions/                # 会话记录
│   ├── whatsapp_123.jsonl
│   ├── telegram_456.jsonl
│   └── discord_789.jsonl
└── knowledge/               # 知识库
    ├── projects/
    │   └── openclaw.md
    └── contacts/
        └── alice.md
```

## MEMORY.md 结构

### 内容示例

```markdown
# 长期记忆

## 用户信息

**姓名**: Alice Chen
**职业**: 软件工程师
**兴趣**: Go 语言, 分布式系统, 机器学习
**时区**: Asia/Shanghai

## 偏好设置

- 喜欢简洁的代码风格
- 倾向于使用 Go 而非 Node.js
- 工作时间：09:00 - 18:00（避免在此时段外打扰）

## 重要事项

- 2024-01-15: 正在开发 GoClaw 项目，目标是用 Go 复刻 OpenClaw
- 2024-01-20: 遇到 SQLite FTS5 集成问题，需要研究 CGO

## 常用命令

- `make build`: 编译项目
- `make test`: 运行测试
- `docker-compose up`: 启动开发环境

## 历史对话要点

### 2024-01-15
讨论了泳道并发模型的设计，决定使用 Go 的 Channel 实现。

### 2024-01-18
解决了 WhatsApp 连接问题，原因是 whatsmeow 版本不兼容。
```

### 更新机制

```typescript
class MemoryManager {
    async updateMemory(key: string, value: string) {
        // 读取当前记忆
        let memory = await fs.readFile('./data/MEMORY.md', 'utf-8');

        // 使用 LLM 智能更新
        const prompt = `
更新以下记忆文档，将新信息整合进去：

当前记忆：
${memory}

新信息：
${key}: ${value}

请保持 Markdown 格式，智能地更新或添加信息。
        `.trim();

        const response = await llm.chat({
            messages: [{ role: 'user', content: prompt }],
        });

        // 写回文件
        await fs.writeFile('./data/MEMORY.md', response.content);

        // 同步到数据库
        await this.syncToDatabase(response.content);
    }
}
```

## 会话日志 (JSONL)

### 格式定义

每条消息一行，使用 JSON Lines 格式：

```jsonl
{"timestamp":"2024-01-15T10:30:00Z","role":"user","content":"你好"}
{"timestamp":"2024-01-15T10:30:05Z","role":"assistant","content":"你好！有什么可以帮你的？"}
{"timestamp":"2024-01-15T10:31:00Z","role":"user","content":"帮我查询订单"}
{"timestamp":"2024-01-15T10:31:10Z","role":"assistant","content":"好的，你的订单号是多少？","tool_calls":[{"name":"query_orders","args":{}}]}
```

### 读取和追加

```typescript
class SessionLogger {
    private sessionPath(sessionId: string): string {
        return `./data/sessions/${sessionId}.jsonl`;
    }

    async append(sessionId: string, message: Message) {
        const line = JSON.stringify({
            timestamp: new Date().toISOString(),
            role: message.role,
            content: message.content,
            metadata: message.metadata,
        });

        await fs.appendFile(this.sessionPath(sessionId), line + '\n');
    }

    async getHistory(sessionId: string, limit: number = 100): Promise<Message[]> {
        const content = await fs.readFile(this.sessionPath(sessionId), 'utf-8');
        const lines = content.trim().split('\n');

        return lines
            .slice(-limit)
            .map(line => JSON.parse(line));
    }
}
```

### 压缩和归档

```typescript
async function archiveOldSessions() {
    const sessions = await fs.readdir('./data/sessions');

    for (const session of sessions) {
        const stats = await fs.stat(`./data/sessions/${session}`);
        const ageInDays = (Date.now() - stats.mtimeMs) / (1000 * 60 * 60 * 24);

        // 归档超过 30 天的会话
        if (ageInDays > 30) {
            await compressFile(
                `./data/sessions/${session}`,
                `./data/archive/${session}.gz`
            );

            await fs.unlink(`./data/sessions/${session}`);
        }
    }
}
```

## 混合检索架构

为了在海量记忆中快速检索，OpenClaw 使用 SQLite 构建了混合检索引擎。

### 检索类型

| 类型 | 原理 | 适用场景 | 示例 |
|------|------|----------|------|
| **全文检索 (FTS5)** | 关键词匹配 | 精确查找 | "OpenClaw 部署文档" |
| **向量检索 (Vector)** | 语义相似度 | 模糊查找 | "系统崩溃" → "服务器无响应" |
| **RRF 融合** | 结合两者 | 兼顾精确和语义 | 最佳综合结果 |

### 数据库 Schema

```sql
-- 全文检索表
CREATE VIRTUAL TABLE memory_fts USING fts5(
    content,              -- 文本内容
    source,               -- 来源文件
    timestamp UNINDEXED   -- 时间戳（不索引）
);

-- 向量检索表 (使用 sqlite-vec 扩展)
CREATE VIRTUAL TABLE memory_vec USING vec0(
    embedding float[1536]  -- 1536 维向量 (OpenAI text-embedding-3-small)
);

-- 关联表
CREATE TABLE memory_refs (
    rowid INTEGER PRIMARY KEY,
    source_file TEXT NOT NULL,
    chunk_index INTEGER,
    content TEXT NOT NULL,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (rowid) REFERENCES memory_fts(rowid),
    FOREIGN KEY (rowid) REFERENCES memory_vec(rowid)
);
```

### 索引构建

```typescript
class MemoryIndexer {
    async indexDocument(filePath: string) {
        // 1. 读取文件
        const content = await fs.readFile(filePath, 'utf-8');

        // 2. 分块
        const chunks = this.splitIntoChunks(content, 500); // 每块 500 tokens

        for (const [index, chunk] of chunks.entries()) {
            // 3. 生成 Embedding
            const embedding = await this.getEmbedding(chunk);

            // 4. 插入数据库
            const result = await db.run(
                'INSERT INTO memory_refs (source_file, chunk_index, content) VALUES (?, ?, ?)',
                [filePath, index, chunk]
            );

            const rowid = result.lastID;

            // 5. 插入 FTS5
            await db.run(
                'INSERT INTO memory_fts (rowid, content, source) VALUES (?, ?, ?)',
                [rowid, chunk, filePath]
            );

            // 6. 插入 Vector
            await db.run(
                'INSERT INTO memory_vec (rowid, embedding) VALUES (?, ?)',
                [rowid, JSON.stringify(embedding)]
            );
        }
    }

    private splitIntoChunks(text: string, maxTokens: number): string[] {
        // 使用 tiktoken 分词
        const tokens = encoding.encode(text);
        const chunks: string[] = [];

        for (let i = 0; i < tokens.length; i += maxTokens) {
            const chunkTokens = tokens.slice(i, i + maxTokens);
            chunks.push(encoding.decode(chunkTokens));
        }

        return chunks;
    }

    private async getEmbedding(text: string): Promise<number[]> {
        const response = await fetch('https://api.openai.com/v1/embeddings', {
            method: 'POST',
            headers: {
                'Authorization': `Bearer ${process.env.OPENAI_API_KEY}`,
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                model: 'text-embedding-3-small',
                input: text,
            }),
        });

        const data = await response.json();
        return data.data[0].embedding;
    }
}
```

### 混合搜索

```typescript
class HybridSearchEngine {
    async search(query: string, limit: number = 10): Promise<SearchResult[]> {
        // 1. 生成查询 Embedding
        const queryEmbedding = await this.getEmbedding(query);

        // 2. 全文检索 (FTS5)
        const ftsResults = await db.all(`
            SELECT rowid, rank
            FROM memory_fts
            WHERE content MATCH ?
            ORDER BY rank
            LIMIT 20
        `, [query]);

        // 3. 向量检索
        const vecResults = await db.all(`
            SELECT rowid, distance
            FROM memory_vec
            WHERE embedding MATCH ?
            ORDER BY distance
            LIMIT 20
        `, [JSON.stringify(queryEmbedding)]);

        // 4. RRF 融合
        const merged = this.reciprocalRankFusion(ftsResults, vecResults);

        // 5. 获取完整内容
        const results = await Promise.all(
            merged.slice(0, limit).map(async item => {
                const row = await db.get(
                    'SELECT * FROM memory_refs WHERE rowid = ?',
                    [item.rowid]
                );

                return {
                    content: row.content,
                    source: row.source_file,
                    score: item.score,
                };
            })
        );

        return results;
    }

    private reciprocalRankFusion(
        ftsResults: Array<{ rowid: number; rank: number }>,
        vecResults: Array<{ rowid: number; distance: number }>
    ): Array<{ rowid: number; score: number }> {
        const k = 60; // RRF 常数
        const scores = new Map<number, number>();

        // FTS 分数
        ftsResults.forEach((result, index) => {
            const score = 1 / (k + index + 1);
            scores.set(result.rowid, (scores.get(result.rowid) || 0) + score);
        });

        // Vector 分数
        vecResults.forEach((result, index) => {
            const score = 1 / (k + index + 1);
            scores.set(result.rowid, (scores.get(result.rowid) || 0) + score);
        });

        // 排序
        return Array.from(scores.entries())
            .map(([rowid, score]) => ({ rowid, score }))
            .sort((a, b) => b.score - a.score);
    }
}
```

### 搜索示例

```typescript
// 使用示例
const results = await searchEngine.search('OpenClaw 的泳道并发模型是怎么工作的？');

console.log(results);
// [
//   {
//     content: '泳道并发模型...每个会话被分配到独立的逻辑泳道...',
//     source: '/docs/02-architecture/concurrency-model.md',
//     score: 0.85
//   },
//   ...
// ]
```

## 记忆同步

### 文件到数据库

```typescript
class MemorySyncService {
    async syncAll() {
        // 监听文件变化
        const watcher = fs.watch('./data', { recursive: true });

        for await (const event of watcher) {
            if (event.filename.endsWith('.md')) {
                await this.syncFile(`./data/${event.filename}`);
            }
        }
    }

    async syncFile(filePath: string) {
        logger.info('Syncing file', { filePath });

        // 重新索引
        await indexer.indexDocument(filePath);

        logger.info('File synced', { filePath });
    }
}
```

---

**相关文档**:
- [记忆系统实现](../04-go-implementation/memory-impl.md)
- [SQLite 优化](../04-go-implementation/memory-impl.md#performance)
