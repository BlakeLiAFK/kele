# Kele 架构审视：过度设计分析 & OpenClaw 网关模式借鉴

> 本文仅为讨论文档，不涉及代码变更。

---

## 一、当前代码量全景

```
总计: ~7,200 行代码 (不含测试)，~1,750 行测试

按层分布:
  tui/       3,242 行  (45%)  ← 占比过高
  llm/       1,228 行  (17%)
  cron/        633 行  (9%)
  tools/       629 行  (9%)
  memory/      450 行  (6%)
  agent/       369 行  (5%)
  config/      187 行  (3%)
  cmd/          57 行  (1%)
```

**核心问题：TUI 占了近一半代码。一个 CLI AI 助手，界面逻辑比大脑还重。**

---

## 二、确认存在的过度设计

### 2.1 tools/registry.go — 存在但被绕过的抽象 (90 行)

Registry 定义了 `ToolHandler` 接口和注册机制，但 cron 工具完全绕过它，直接在 `executor.go` 里硬编码了 5 个 `executeCron*` 方法。

**问题**：这个抽象要么全用，要么不用。当前一半的工具走注册表、一半不走，等于抽象不成立。

**建议**：要么把 cron 工具也做成 ToolHandler 注册进去，要么干脆砍掉 Registry，直接用 `map[string]ToolHandler`。90 行代码对应的功能是一个 map 加一个 slice。

### 2.2 tools/executor.go — 上帝类 (381 行)

Executor 同时承担：
- 工具注册表管理
- 6 个内置工具的实例化
- 5 个 cron 专用函数
- 审计日志集成
- 工具调用分发

**问题**：一个 struct 做了 4 件独立的事。这是经典的上帝类。

**建议**：Executor 应该只做"接收 ToolCall → 找到 handler → 执行 → 记日志"这件事，不超过 100 行。

### 2.3 tui/commands.go — 510 行单函数 switch

`handleCommand()` 用一个 switch 处理 40+ 个命令，每个 case 直接操作 App 和 Session 的内部状态。

**问题**：
- 无法单独测试某个命令
- 每加一个命令就增长这个函数
- 所有命令共享同一个函数作用域

**但也要注意**：如果为此引入 Command 接口 + CommandRegistry + CommandContext……那就是另一种过度设计。对于 40 个命令来说，分文件（按类别分 3-4 个文件）比引入框架更实际。

### 2.4 tui/app.go — 725 行混合关注点

app.go 同时处理：UI 布局、键盘事件路由、流式事件适配、会话管理、补全协调、状态更新。

**其中最突出的问题**是流式事件的双重转换：

```
llm.StreamEvent → agent.StreamEvent → tui.streamEvent → tui.streamMsg
```

四层事件类型，三次转换，做的是同一件事。这不是架构，是层层包装。

### 2.5 ollama.go ListModels() — 死代码

新增的 `ListModels()` 方法没有任何地方调用。写了但没接入 `/models` 命令。

### 2.6 审计日志 — 写了但没人看

`audit.go` 写入 `.kele/audit.log`，但没有任何命令可以查看或分析审计日志。功能完整但产品意义为零。

---

## 三、确认没有过度设计的部分

| 模块 | 行数 | 评价 |
|------|------|------|
| `llm/Provider` 接口 | 3 方法 | 恰到好处。最小接口，三个实现各自独立 |
| `agent/brain.go` | 369 行 | 干净的编排器。接 LLM、工具、记忆，不多不少 |
| `memory/store.go` | 450 行 | 功能丰富但内聚。FTS5 降级设计是防御性编程 |
| `cron/` | 633 行 | 调度器 + 解析器，清晰分离，没有冗余 |
| `config/` | 187 行 | 环境变量加载 + 默认值，简洁 |

---

## 四、OpenClaw 的网关设计：为什么它需要，以及它解决了什么

### 4.1 OpenClaw 是什么

OpenClaw（原 Clawdbot / Moltbot）是一个 **多渠道 AI 代理框架**，核心场景是：

> 一个 AI 同时接入 WhatsApp、Telegram、Discord、Slack、iMessage、Web……
> 从任意渠道收消息，统一处理，再回到对应渠道。

### 4.2 它的网关做了什么

```
WhatsApp ─┐
Telegram ─┤
Discord ──┤──→ [Channel Adapter] ──→ [Gateway Server] ──→ [Lane Queue]
Slack ────┤         归一化消息              会话路由           并发控制
Web ──────┘
                                              │
                                              ▼
                                     [Agent Runner] ──→ [Agentic Loop]
                                      模型选择/Prompt       工具调用循环
```

Gateway 是一个长驻 WebSocket 服务 (`ws://127.0.0.1:18789`)，充当**控制平面**：

1. **渠道归一化**：每个平台适配器把消息统一为 `{sender, text, attachments}` 格式
2. **会话路由**：不同账号/群组映射到不同 Agent 实例，各自独立工作区
3. **并发控制（Lane Queue）**：同一会话内默认串行执行，防止状态冲突
4. **安全边界**：Gateway 是信任边界，在此做访问控制（号码白名单等）
5. **工具编排**：统一管理工具调用、沙箱、浏览器控制、文件 I/O

### 4.3 它的核心哲学

OpenClaw 的文档原话：

> "Instead of trying to make an LLM 'remember' context or behave safely through clever prompts, OpenClaw builds a structured execution environment around the model. **The LLM provides intelligence; OpenClaw provides the operating system.**"

翻译：不要靠 prompt 工程解决架构问题。LLM 负责智能，框架负责一切其他。

### 4.4 网关带来的具体好处

| 好处 | 说明 |
|------|------|
| **加新渠道只写一个适配器** | Agent 逻辑完全不变 |
| **会话隔离** | 不同用户/渠道的会话互不干扰 |
| **模型路由** | 可接 ClawRouter 按请求复杂度选模型，降 60-90% 成本 |
| **可观测性** | 所有消息经过网关，天然审计点 |
| **远程访问** | 网关暴露 WebSocket，CLI/Web/移动端通过 RPC 连入 |
| **失败隔离** | 一个会话崩了不影响其他 |

---

## 五、Kele 是否需要借鉴网关模式？

### 5.1 先搞清楚 Kele 是什么

Kele 是一个**单用户、本地运行的 TUI AI 助手**。

- 没有多渠道（只有终端）
- 没有多用户（只有当前用户）
- 没有远程访问需求
- 没有平台适配器需求

### 5.2 直接抄网关？完全不需要

如果 Kele 现在引入 Gateway Server + Channel Adapter + Lane Queue，那不是架构优化，是自找麻烦。**一个单进程 TUI 程序不需要 WebSocket 控制平面。**

这就好比给一辆自行车装 ABS 防抱死系统——技术上可以做，但完全没有场景。

### 5.3 但是，有一些思想值得借鉴

#### 思想 1：归一化消息格式

OpenClaw 的渠道适配器把不同平台的消息统一为一个格式。Kele 虽然只有终端这一个"渠道"，但内部有 **四层事件类型做同一件事**：

```
llm.StreamEvent → agent.StreamEvent → tui.streamEvent → tui.streamMsg
```

借鉴：**定义一个统一的内部事件类型**，从 LLM 层到 TUI 层一路传递，不做转换。如果未来真的加 Web UI 或 API，这个统一事件也直接能用。

#### 思想 2：Agent 与渠道解耦

OpenClaw 的 Agent 逻辑完全不知道消息来自哪个平台。Kele 现在的 `agent/brain.go` 基本做到了这一点——Brain 不知道 TUI 的存在。**这是 Kele 做得好的地方，应该保持。**

但有一个破坏点：`tui/app.go` 里的 `startStream()` 方法做了 agent.StreamEvent → streamEvent 的适配。这个适配应该是零行代码——如果事件类型统一的话。

#### 思想 3：控制平面的轻量化版本

OpenClaw 的网关本质是一个**事件总线 + 会话路由器**。Kele 不需要 WebSocket 服务，但如果未来要加：

- HTTP API 接口（让外部脚本调用 Kele）
- Web UI 前端
- 多实例协作

那么需要的不是"网关"，而是一个**事件总线**。当前 Kele 的 Brain 已经是事件生产者（StreamEvent），TUI 是消费者。如果把这个关系抽象为接口：

```go
type EventConsumer interface {
    OnStreamEvent(event StreamEvent)
}
```

那 TUI 实现它，未来的 Web UI 也实现它，Brain 不需要任何修改。

**但现在不需要做这件事。** 只需要确保 Brain 不依赖 TUI 即可（现在已经做到）。

#### 思想 4：LLM 路由（成本优化）

OpenClaw 社区搞了多个模型路由器（ClawRouter、ClawRoute、OpenClaw Hub），核心思路：

> 简单请求走便宜模型，复杂请求走贵模型。

Kele 已经有"大模型 + 小模型"的双模型机制（`SmallModel` 用于补全），但没有**自动路由**。如果要借鉴，可以在 `ProviderManager` 里加一个轻量级分类器：

- 补全/简单问答 → 小模型
- 工具调用/代码生成 → 大模型

但这是 v0.3.0 的事，不是当务之急。

---

## 六、真正该做的事（而不是加网关）

基于以上分析，Kele 当前阶段真正需要的是**减法**，不是加法：

### 优先级 P0：消除冗余

1. **统一事件类型**：砍掉 `tui.streamEvent` 和 `tui.streamMsg`，直接用 `agent.StreamEvent`。三次转换变零次。预计删除 50-80 行代码。

2. **清理死代码**：`ollama.ListModels()` 要么接入 `/models` 命令，要么删掉。

### 优先级 P1：拆分膨胀模块

3. **拆 tui/commands.go**：按功能分成 3-4 个文件（model_commands.go、memory_commands.go、session_commands.go、info_commands.go），不需要引入 Command 接口，只需分文件。

4. **拆 tools/executor.go**：把 cron 工具做成 ToolHandler 注册进 Registry，或者彻底把 Registry 砍掉用 map。不要两边都存在。

5. **拆 tui/app.go**：把流式处理逻辑抽出为 `stream_handler.go`（~100 行），把会话管理抽出为 `session_manager.go`（~80 行），app.go 只保留 Init/Update/View 骨架。

### 优先级 P2：补全产品闭环

6. **审计日志可查看**：加 `/audit` 命令或者让 `/debug` 展示最近 N 条审计记录。否则 audit.go 是自嗨。

7. **ListModels 接入**：让 `/models` 命令在 Ollama 可用时自动查询本地已安装的模型并展示。

---

## 七、总结

| 问题 | 回答 |
|------|------|
| Kele 有过度设计吗？ | **有，但不严重。** 主要在 TUI 层（事件转换冗余、大文件）和工具层（Registry 被绕过）|
| OpenClaw 的网关好吗？ | **对 OpenClaw 来说好。** 因为它要接 6+ 个消息平台，网关是必需品 |
| Kele 需要网关吗？ | **不需要。** 单用户 TUI 程序引入网关是典型的过度设计 |
| 该借鉴什么？ | **统一事件格式**（消除转换层）、**Agent 与渠道解耦**（已做到，保持）、**未来预留 EventConsumer 接口**（现在不做，但架构上别堵死） |
| 现在该做什么？ | **减法：砍冗余、拆大文件、补闭环。不加新抽象。** |

一句话：**OpenClaw 的网关是给分布式多渠道系统设计的。Kele 是一个单进程 TUI。借鉴其思想（统一事件、解耦），不借鉴其形式（Gateway Server）。**
