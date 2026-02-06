# Kele 🥤

TUI 版 OpenClaw - 终端交互式 AI 助手

> Claude Code 的交互体验 + OpenClaw 的自主能力 = Kele

---

## 特性

- 🖥️ **现代 TUI**: 使用 Bubble Tea 构建的精美终端界面
- 🤖 **AI 驱动**: 集成 Claude 3.5 Sonnet，智能对话
- 🛠️ **工具执行**: 可以执行 shell 命令、读写文件
- 💾 **持久记忆**: 自动保存对话历史和长期记忆
- ⚡ **实时响应**: 流式显示 AI 回复
- 🎯 **Slash 命令**: 内置丰富的快捷命令

## 快速开始

### 安装依赖

```bash
make deps
```

### 运行（当前是基础框架演示）

```bash
make run
```

## 使用

### 基础对话

直接输入消息，按 Enter 发送：

```
> 分析这个项目的代码结构
```

### Slash 命令

```
/help   - 显示帮助
/clear  - 清空对话
/exit   - 退出程序
/status - 显示状态
```

### 快捷键

- `Enter`: 发送消息
- `Ctrl+C`: 退出

## 开发进度 - MVP 完成！🎉

- [x] Day 1: TUI 基础框架 ✅
- [x] Day 2: LLM 集成 ✅
- [x] Day 3-4: 工具系统（bash/read/write）✅
- [x] Day 5-6: 记忆与会话 ✅
- [x] Day 7: 完整测试与文档 ✅

**v0.1.0 已完成！** 详见 [CHANGELOG.md](CHANGELOG.md)

## 项目结构

```
kele/
├── cmd/kele/           # 入口
├── internal/
│   ├── tui/            # TUI 组件 ✅
│   ├── agent/          # AI 代理（待实现）
│   └── llm/            # LLM 客户端（待实现）
├── .kele/              # 配置和数据
│   ├── config.yaml     # 配置文件
│   └── MEMORY.md       # 长期记忆
└── docs/               # 文档
```

## 文档

- [完整文档](docs/README.md) - OpenClaw 架构深度解析
- [MVP 计划](plans/active/mvp-tui.md) - 7 天实施计划
- [实施路线图](docs/05-roadmap/implementation-plan.md)

## 参考

基于以下项目的最佳实践：

- [Claude Code](https://code.claude.com) - TUI 交互模式
- [OpenClaw](https://openclaw.ai) - 自主能力设计
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI 框架

### Sources
- [Claude Code TUI 架构分析](https://kotrotsos.medium.com/claude-code-internals-part-11-terminal-ui-542fe17db016)
- [OpenClaw 完整指南](https://www.jitendrazaa.com/blog/ai/clawdbot-complete-guide-open-source-ai-assistant-2026/)
- [Claude Code 官方文档](https://code.claude.com/docs/en/cli-reference)

---

## 附录：OpenClaw AI 智能体架构深度解析

以下是完整的技术研究报告（已迁移到 docs/ 目录）。

### 1. 执行摘要在人工智能从单纯的对话生成向自主代理（Autonomous Agents）演进的浪潮中，OpenClaw（前身为 Moltbot 和 Clawdbot）作为一个开源、本地优先（Local-First）的 AI 智能体框架，确立了其独特的生态地位。与依赖云端托管的封闭系统不同，OpenClaw 采用去中心化架构，将“大脑”部署在用户控制的基础设施上，通过一个名为“网关（Gateway）”的核心守护进程，实现了与 WhatsApp、Telegram、Discord 等主流通讯平台的无缝桥接。其核心创新在于摒弃了传统的被动响应模式，引入了基于状态感知的“心跳（Heartbeat）”机制，使智能体具备了主动监控、决策和发起交互的能力。本报告旨在对 OpenClaw 的工作原理进行穷尽式的技术解构，涵盖其自主持续运行机制、多 LLM（大型语言模型）调度的编排逻辑、以及跨平台聊天网关的实现细节。基于对 OpenClaw Node.js 原型架构的深刻剖析，本报告进一步提出了一套基于 Go 语言的复刻方案——“GoClaw”。该方案利用 Go 语言在并发处理（Goroutines）、静态类型安全及跨平台二进制编译方面的原生优势，旨在构建一个性能更优、资源占用更低且更易于企业级部署的智能体运行时环境。2. 引言：从对话机器人到数字劳动力2.1 智能体范式的转变传统的 AI 交互模式主要基于“请求-响应（Request-Response）”架构，即用户输入提示词，系统返回结果。这种模式在处理即时问答时表现优异，但在处理长期任务、多步骤工作流及环境监控时显得力不从心 。OpenClaw 的出现代表了向“代理型 AI（Agentic AI）”的转变。代理型 AI 不仅仅是回答问题，更是执行任务的数字劳动力。它们具备持久的记忆、工具使用能力以及一定程度的自主权，能够像人类员工一样在后台持续运行，处理邮件、管理日程甚至编写代码 。2.2 OpenClaw 的独特生态位OpenClaw 区别于 Microsoft Copilot 或 OpenAI GPTs 的核心在于其本地优先和自主权。它运行在用户的 MacBook、Raspberry Pi 或私有 VPS 上，这意味着用户拥有数据和执行环境的绝对控制权 。它不是某个应用内的插件，而是一个独立的操作系统级服务，通过标准的聊天软件接口与人类互动，这种“ChatOps”模式极大地降低了用户接入复杂 AI 能力的门槛 。3. OpenClaw 核心架构解析OpenClaw 的架构设计体现了高度的模块化和解耦思想，其核心围绕着“网关（Gateway）”这一单一控制平面展开。3.1 网关（The Gateway）：单一事实来源在分布式系统中，状态一致性是一个经典难题。OpenClaw 通过网关模式解决了这一问题。网关是一个长连接的 Node.js 进程，充当了所有外部通信和内部逻辑的枢纽 。3.1.1 WebSocket 控制平面网关并不直接处理业务逻辑，而是维护一个本地 WebSocket 服务器（默认为 ws://127.0.0.1:18789）。所有的组件，包括 CLI 工具、Web 控制台（Control UI）、移动端节点（iOS/Android Node）以及核心代理运行时（Agent Runtime），都作为客户端连接到这个 WebSocket 服务 。这种设计将“连接管理”与“智能处理”解耦，使得系统具有极高的扩展性。例如，当通过 CLI 发送命令时，实际上是将指令推送到 WebSocket 总线，再由网关路由到相应的处理单元 。3.1.2 统一事件总线无论是来自 WhatsApp 的文本消息，还是系统内部定时器触发的心跳信号，在进入网关后都会被标准化为统一的事件对象（Event Object）。这种标准化处理屏蔽了底层协议的差异（如 Telegram 的 Long Polling 与 Discord 的 Gateway Intent 差异），使得上层的智能体逻辑可以以统一的方式处理所有输入 。3.2 泳道并发模型（Lane-Based Concurrency）为了在处理高并发聊天时保持系统的响应性，OpenClaw 引入了**泳道（Lanes）**的概念。这是一种轻量级的应用层并发控制机制 。隔离性：每个会话（Session）被分配到一个独立的逻辑“泳道”中。这意味着处理用户 A 的复杂查询（如生成长报告）不会阻塞用户 B 的简单问候。优先级队列：泳道内部维护着优先级队列。用户的直接指令（Chat Lane）通常拥有最高优先级，而后台的定时任务或心跳检测（Cron/Heartbeat Lane）则在系统空闲或低负载时执行。状态锁定：在同一个泳道内，为了防止上下文竞争（Race Condition），消息通常是串行处理的。这确保了 AI 在回复前一条消息时，不会被后续的消息打断思路，模拟了人类的线性思维过程 。3.3 聊天软件网关实现细节OpenClaw 的强大之处在于其对主流聊天软件的深度集成，这主要依赖于一系列开源协议适配器。3.3.1 WhatsApp 集成（Baileys 协议）对于 WhatsApp，OpenClaw 使用了 Baileys 库。这是一个 TypeScript 编写的库，它逆向工程了 WhatsApp Web 的 WebSocket 协议 。工作原理：OpenClaw 模拟成一个浏览器客户端，通过扫描二维码与用户的手机配对。之后，它维护与 WhatsApp 服务器的长连接，解密收到的 Protobuf 格式消息，并将其转化为 OpenClaw 的标准事件。优势：无需申请 WhatsApp Business API，适合个人用户使用，且支持端到端加密 。3.3.2 Telegram 与 Discord 集成Telegram：利用 grammY 框架对接 Telegram Bot API。支持长轮询（Long Polling）模式，这使得 OpenClaw 即使在没有公网 IP 的内网环境中（如家用 NAS）也能接收消息，无需配置复杂的 Webhook 和内网穿透 。Discord：通过 discord.js 连接 Discord Gateway，支持分片（Sharding）和意图（Intents）管理，能够处理服务器中的提及（Mention）和私信 。3.4 多 LLM 调用与编排（Model Orchestration）OpenClaw 并不绑定单一的 AI 模型，而是作为一个**模型无关（Model-Agnostic）**的编排层。3.4.1 统一接口抽象系统内部定义了一套通用的 LLM 交互接口，支持 OpenAI（GPT-4o）、Anthropic（Claude 3.5 Sonnet）、Google（Gemini）以及本地模型（通过 Ollama 或 LM Studio）。这种抽象层处理了不同提供商 API 的差异（如 Token 计算方式、工具调用格式等），使得切换模型只需修改配置文件。3.4.2 认证配置文件轮换（Auth Profile Rotation）为了保证生产环境的高可用性，OpenClaw 实现了认证配置轮换机制。用户可以在 auth-profiles.json 中配置多个 API Key 或 OAuth 凭证。当主力模型（如 Claude）达到速率限制（Rate Limit）或服务宕机时，系统会自动降级（Failover）到备用模型（如 GPT-4），确保服务不中断 。3.4.3 智能路由系统支持基于任务复杂度的路由。简单的意图识别任务可路由至低成本模型（如 GPT-4o-mini），而复杂的推理任务则路由至高智力模型（如 Claude 3.5 Sonnet）。这种策略在保证效果的同时显著降低了运营成本 。4. 自主持续运行机制OpenClaw 区别于传统 Chatbot 的核心在于其主动性（Proactivity）。它不只是被动等待，而是主动观察、思考并采取行动。4.1 守护进程与持久化通过操作系统的服务管理器（如 macOS 的 launchd 或 Linux 的 systemd），OpenClaw 的网关被配置为开机自启的守护进程。这意味着即便用户关闭了终端窗口，AI 依然在后台运行，维持着与外部世界的连接 。4.2 心跳机制（The Heartbeat） vs. Cron传统的自动化依赖 Cron 任务，按固定时间机械执行。OpenClaw 引入了更为智能的心跳机制 。上下文感知：心跳是一个周期性的“唤醒”信号（例如每 15 分钟一次）。与 Cron 不同，当心跳触发时，AI 会获得当前的系统快照（时间、未读消息概览、系统状态）。决策层：AI 根据这个快照进行“判断”。例如，HEARTBEAT.md 文件中可能定义了“每天早上 9 点检查服务器日志”。当心跳发生在 9:00 时，AI 会决定执行该任务；而在 3:00 时，它可能会决定“无事可做”，继续休眠。主动交互：这种机制赋予了 AI 随时打断用户的能力。如果 AI 在后台检测到服务器异常，它会利用心跳周期主动向用户发送 WhatsApp 警告，而不是等待用户查询 。4.3 HEARTBEAT.md 的逻辑流程HEARTBEAT.md 文件是 AI 的自主行为准则。它包含了自然语言描述的长期任务清单。触发：定时器触发 SystemHeartbeat 事件。读取：Agent 读取 HEARTBEAT.md 内容。推理：Agent 结合当前时间和状态，评估哪些任务需要执行。执行：Agent 调用相应工具（如 curl 检查网页，或 fs 读取文件）。反馈：执行结果被写入日志或通过聊天软件发送给用户 。5. 记忆与上下文系统AI 的连贯性依赖于记忆。OpenClaw 采用了一套独特的文件系统与数据库混合的记忆架构，兼顾了人类可读性与机器检索效率。5.1 文件即真理（File-First Philosophy）OpenClaw 坚持“文件即真理”的原则。所有的长期记忆、用户画像和会话记录都以 Markdown 或 JSONL 文件形式存储在本地磁盘上 。MEMORY.md：存储经过 AI 提炼的长期知识和用户偏好。Session Logs (JSONL)：存储原始的对话流水，用于审计和回溯。
这种设计使得用户可以直接通过文本编辑器查看和修改 AI 的记忆，极大地增强了系统的透明度和可控性 。5.2 混合搜索架构（Hybrid Search）为了在海量记忆中快速检索，OpenClaw 使用 SQLite 构建了混合检索引擎 。全文检索（FTS5）：利用 SQLite 的 FTS5 扩展进行关键词匹配。这对于查找特定的错误代码、专有名词（如“OpenClaw 部署文档”）非常精准。向量检索（Vector Search）：利用 sqlite-vec 扩展存储文本的 Embeddings。通过余弦相似度（Cosine Similarity）进行语义搜索，能够找回措辞不同但含义相关的记忆（如搜索“系统崩溃”能匹配到“服务器无响应”的记录）。RRF 融合：系统通过倒数排名融合（Reciprocal Rank Fusion, RRF）算法，将 FTS5 和向量搜索的结果结合，从而获得兼具精确度和语义广度的检索结果 。6. 安全架构将 AI 接入个人数字生活带来了巨大的安全风险。OpenClaw 设计了多层防御机制。6.1 DM 配对机制（Pairing Code）默认情况下，OpenClaw 对所有未知的私信（DM）采取“拒绝并配对”策略。当一个陌生的 WhatsApp 账号发来消息时，系统不会直接响应，而是生成一个临时的 6 位数配对码，并在服务器日志中打印。管理员必须在 CLI 中运行 openclaw pairing approve whatsapp <code 才能授权该账号 。这有效防止了恶意用户滥用暴露的 Bot 接口。6.2 沙箱与权限控制为了防止 AI 执行危险命令（如 rm -rf /），OpenClaw 支持 Docker 容器化沙箱。工具白名单：用户可以配置允许 AI 使用的工具列表。执行审批：对于高风险操作（如文件删除、资金转账），系统可以配置为“人在回路（Human-in-the-loop）”模式，即 AI 必须先发送请求，获得用户点击“批准”按钮后方可执行 。7. Go 语言复刻方案设计：“GoClaw”虽然 OpenClaw 的 Node.js 实现功能强大，但 Go 语言（Golang）在构建高性能、高并发的网络服务方面具有天然优势。Go 的静态编译特性使得分发单一二进制文件成为可能，极大地简化了部署流程。本节详细阐述如何用 Go 复刻 OpenClaw 的核心功能。7.1 系统架构概览“GoClaw”将采用清晰架构（Clean Architecture），将业务逻辑与外部接口解耦。层级组件职责接入层 (Interfaces)WebSocket Server, HTTP API处理客户端连接，协议升级，鉴权。适配层 (Adapters)WhatsApp(whatsmeow), Telegram(tg-bot-api)将各平台私有协议转换为内部 Event 结构。核心层 (Core)Gateway, Brain (Agent Loop), Scheduler核心业务逻辑，状态管理，泳道调度。基础设施层 (Infra)SQLite (CGO), LLM Clients数据持久化，外部 API 调用。7.2 核心组件设计7.2.1 统一事件总线与类型定义Go 语言的强类型特性要求我们预先定义严格的事件结构。Gopackage types

import "time"

type EventType string

const (
    EventMessageReceived EventType = "message.received"
    EventHeartbeat       EventType = "system.heartbeat"
    EventSystemSignal    EventType = "system.signal"
)

// Event 定义了系统内部流转的标准数据包
type Event struct {
    ID        string      `json:"id"`
    Type      EventType   `json:"type"`
    Payload   interface{} `json:"payload"` // 具体的 Message 或 Signal 结构
    SessionID string      `json:"session_id"`
    Timestamp time.Time   `json:"timestamp"`
    Source    string      `json:"source"`  // e.g., "whatsapp", "system"
}

// Message 定义了聊天内容的标准格式
type Message struct {
    SenderID   string `json:"sender_id"`
    SenderName string `json:"sender_name"`
    Content    string `json:"content"`
    IsDirect   bool   `json:"is_direct"`
    Raw        any    `json:"-"` // 保留原始协议数据以便调试
}
7.2.2 网关与泳道调度器（Gateway & Dispatcher）利用 Go 的 Channel 和 Goroutine 实现高效的泳道并发模型。每个 SessionID 对应一个独立的 Goroutine，确保并发隔离。Gopackage gateway

import (
    "sync"
    "github.com/your-repo/goclaw/types"
)

type Gateway struct {
    eventBus  chan types.Event
    sessions  map[string]chan types.Event // 活跃泳道注册表
    mu        sync.RWMutex
    connectorsConnector
}

// Start 启动网关主循环
func (g *Gateway) Start() {
    // 启动所有连接器（WhatsApp, Telegram等）
    for _, c := range g.connectors {
        go c.Listen(g.eventBus)
    }

    // 主事件分发循环
    for event := range g.eventBus {
        g.dispatch(event)
    }
}

// dispatch 将事件路由到对应的泳道
func (g *Gateway) dispatch(evt types.Event) {
    g.mu.Lock()
    defer g.mu.Unlock()

    lane, exists := g.sessions
    if!exists {
        // 如果泳道不存在，创建一个新的 Goroutine 处理该 Session
        lane = make(chan types.Event, 100) // 缓冲通道防止阻塞
        g.sessions = lane
        go g.processLane(evt.SessionID, lane)
    }
    
    // 将事件推入泳道
    lane <- evt
}

// processLane 是每个 Session 的独立处理循环
func (g *Gateway) processLane(sessionID string, lane <-chan types.Event) {
    brain := NewAgentBrain()
    for evt := range lane {
        // 核心智能体逻辑：思考 -> 行动
        brain.Handle(evt) 
    }
}
7.3 聊天适配器实现方案7.3.1 WhatsApp：集成 whatsmeowwhatsmeow 是 Go 语言生态中最成熟的 WhatsApp 协议实现，不需要依赖浏览器环境，比 Node.js 的 Baileys 更轻量、性能更好 。实施步骤：存储层：实现 whatsmeow/store/sqlstore 接口，将会话密钥存储在 SQLite 中。事件监听：注册 client.AddEventHandler，监听 *events.Message。转换逻辑：将 events.Message 解包，提取文本、图片，封装为 types.Event 并发送到 eventBus。7.3.2 Telegram：集成 go-telegram-bot-api使用官方推荐的 go-telegram-bot-api 库。实施细节：Long Polling：配置 u := tgbotapi.NewUpdate(0); u.Timeout = 60 以启用长轮询，模拟 OpenClaw 在内网环境下的穿透能力。文件处理：利用 Go 的 io.Reader 流式处理 Telegram 发来的文件，直接对接本地存储或 OCR 服务，无需完全加载到内存。7.4 记忆系统：SQLite CGO 方案在 Go 中复刻混合搜索需要使用 CGO 调用 SQLite 的 C 扩展。技术选型：驱动：github.com/mattn/go-sqlite3。扩展：编译时链接 sqlite-vec（向量搜索）和启用 FTS5（全文搜索）。Schema 设计：SQL-- 存储文本块的全文索引表
CREATE VIRTUAL TABLE memory_fts USING fts5(
    content, 
    metadata UNINDEXED
);

-- 存储向量的表 (依赖 sqlite-vec)
CREATE VIRTUAL TABLE memory_vec USING vec0(
    embedding float
);

-- 关联表
CREATE TABLE memory_refs (
    rowid INTEGER PRIMARY KEY,
    source_file TEXT,
    timestamp DATETIME
);
Go 代码实现 RRF 融合搜索：
GoClaw 需要实现一个函数，执行复杂的 SQL 查询来融合 FTS 和 Vector 的结果。由于 sqlite-vec 还处于早期阶段，Go 的绑定可能需要手动处理 C 指针或使用原生的 SQL 接口 。Gofunc (m *MemoryStore) HybridSearch(query string, embeddingfloat32) (Result, error) {
    // 伪代码 SQL：结合 FTS5 rank 和 Vector distance
    sql := `
    WITH fts_results AS (
        SELECT rowid, rank FROM memory_fts WHERE content MATCH? ORDER BY rank LIMIT 20
    ),
    vec_results AS (
        SELECT rowid, distance FROM memory_vec WHERE embedding MATCH? AND k=20
    )
    SELECT... -- 执行 RRF 算法合并结果
    `
    // 执行查询...
}
7.5 心跳与主动性实现在 Go 中，使用 time.Ticker 可以非常高效地实现心跳，且资源消耗远低于 Node.js 的 setInterval。Gofunc (s *Scheduler) StartHeartbeat() {
    ticker := time.NewTicker(15 * time.Minute)
    for range ticker.C {
        // 构建心跳事件
        evt := types.Event{
            Type:      types.EventHeartbeat,
            SessionID: "system_main", // 路由到主系统泳道
            Timestamp: time.Now(),
        }
        s.gateway.Input(evt)
    }
}
7.6 安全模块：配对中间件在 dispatch 函数之前，引入中间件链（Middleware Chain）。配对逻辑实现：检查白名单：查询 allowlist 表，检查 evt.SenderID 是否存在。拦截：如果不存在，生成 6 位随机码，存入 Redis 或内存 Cache（带 TTL）。日志输出：log.Printf("New pairing request from %s. Code: %s", evt.SenderID, code)。阻断：返回 nil，阻止事件进入泳道。CLI 批准：实现 goclaw pairing approve <code> 命令，验证通过后将 ID 写入白名单。8. 实施路线图与总结8.1 实施阶段规划阶段一：核心网关（Core Gateway）搭建 Go 项目骨架，实现 EventBus 和泳道调度模型。集成 whatsmeow，实现最基础的收发消息。阶段二：大脑接入（The Brain）封装 OpenAI/Anthropic API 客户端。实现基于 Context 的对话历史管理。阶段三：记忆增强（Memory）集成 SQLite FTS5 + Vector。实现 Markdown 文件的双向同步逻辑。阶段四：自主性（Autonomy）实现 Heartbeat Ticker。解析 HEARTBEAT.md 的逻辑并在 Agent 循环中执行。8.2 总结OpenClaw 通过网关模式、混合记忆和主动心跳机制，定义了下一代个人 AI 助理的标准架构。使用 Go 语言复刻该系统，不仅能够完美重现其功能，还能在并发性能、内存占用和部署便捷性上实现质的飞跃。GoClaw 的架构设计充分利用了 Go 语言在系统编程领域的优势，为构建稳定、高效、可扩展的自主智能体提供了一条清晰的工程化路径。这一复刻方案将把 OpenClaw 从一个极客的实验性项目，转化为生产级的高性能数字基础设施，为未来的“人机共生”时代奠定坚实的技术基石。参考文献支持说明：关于 OpenClaw 本地优先与网关架构的分析支持自 。关于 WhatsApp Baileys 协议与 Telegram 实现的细节支持自 。关于心跳机制与 Cron 区别的论述支持自 。关于混合搜索（FTS5 + Vector）的技术细节支持自 。关于安全配对（Pairing）机制的描述支持自 。关于 Go 语言库选型（whatsmeow, sqlite-vec）的依据支持自 。
