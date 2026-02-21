# 更新日志

## v0.4.0 - 2026-02-21

### ✨ 新功能

- **TaskBoard 看板任务系统**: 完整的任务编排与管理系统
  - Workspace（工作区）隔离管理，独立并发控制
  - Task 生命周期管理（backlog → ready → running → done/failed）
  - DAG 依赖解析，任务完成后自动提升下游任务
  - 事件总线（event bus）实时广播状态变更

- **AI Planner（规划器）**: 模糊目标自动分解为结构化任务
  - `kele board plan "目标"` 一句话启动，AI 读代码分析架构
  - 输出结构化 JSON 任务计划（workspace + tasks + 依赖关系）
  - 用户可审阅、修改后批准执行
  - `kele board approve` 批准计划并启动调度

- **跨任务上下文注入**: 有依赖的任务自动获取前置任务结果
  - `buildTaskPrompt` 自动将依赖任务的 result 注入到当前任务 prompt
  - 每个依赖结果最多 2000 字符，防止 prompt 超长

- **Synthesizer（结果汇总）**: 工作区全部完成后自动生成报告
  - 聚合各任务 result，AI 生成简洁的完成报告
  - 报告存储在 workspace.summary，可通过 `kele workspace summary` 查看

- **调度器**: 事件驱动 + 定时兜底的自动任务调度
  - 每个 workspace 独立并发控制（max_concurrent）
  - 任务完成后自动解析依赖、提升下游任务、检查工作区完成
  - Daemon 重启后自动恢复 running → ready

- **18 个新 gRPC RPC**: 完整的 TaskBoard API
  - Workspace CRUD（5 个）
  - Task CRUD + 执行控制（8 个）
  - Planner: PlanWorkspace（streaming）+ ApprovePlan
  - Board: GetBoardOverview + WatchBoard（streaming）
  - TaskLog: GetTaskLog

- **CLI 命令**: 三组新命令
  - `kele board` — 看板总览 + plan + approve + watch
  - `kele workspace` (alias: ws) — create/list/show/pause/resume/delete/summary
  - `kele task` — create/list/show/start/cancel/retry/log

### 🔧 技术改进

- 新增 `internal/taskboard/` 包（types.go, store.go, board.go, scheduler.go, planner.go）
- 独立 SQLite 数据库 `~/.kele/taskboard.db`，与聊天记忆解耦
- `SessionBrain` 新增 `InjectContext` 方法，支持工作区上下文注入
- `TaskSessionManager` 接口设计避免 taskboard ↔ daemon 循环依赖
- `TaskSessionAdapter` 桥接 daemon.SessionManager 与 taskboard 接口
- gRPC proto 从 8 个 RPC 扩展至 26 个 RPC
- 新增 `internal/cli/board.go`, `workspace.go`, `task.go`
- 13 个新增单元测试（types, store CRUD, 依赖解析, plan 解析, JSON 提取）

---

## v0.3.0 - 2026-02-20

### ✨ 新功能

- **Cobra CLI 框架**: 单二进制多模式架构
  - `kele` — 交互式 TUI（默认）
  - `kele daemon start/stop/status` — 后台守护进程管理
  - `kele agent "prompt"` — 无头 Agent 模式（脚本/CI 集成）
  - `kele version` — 版本信息

- **gRPC 守护进程**: 持久化后台服务，通过 Unix Socket 通信
  - 基于 Protobuf 的 8 个 RPC（含服务端流式 Chat）
  - Unix Socket (`~/.kele/kele.sock`) IPC，PID 文件管理
  - 共享资源（ProviderManager、Executor、Memory.Store、Scheduler）
  - SessionBrain 模式：每会话独立历史，共享底层资源
  - TUI 自动启动守护进程（fork `kele daemon start --foreground`）

- **TUI 双模式**: 支持 standalone 和 daemon 两种运行模式
  - Standalone 模式：直接使用本地 Brain（测试/回退）
  - Daemon 模式：通过 gRPC 客户端与守护进程通信
  - 命令自动路由：TUI-local 命令本地处理，其余转发至守护进程
  - 补全引擎同步支持双模式

- **Heartbeat 系统**: AI 驱动的定期系统监控
  - 动态频率调度（夜间 60 分钟，工作时间 15 分钟，其他 30 分钟）
  - 系统快照（goroutine 数、内存用量、CPU 信息、活跃会话数）
  - HEARTBEAT.md 配置文件（支持 CWD、~/、~/.kele/ 三级查找）
  - LLM 自主决策 + 工具调用执行
  - 执行记录追踪（最近 100 条）
  - gRPC 状态查询 API

- **Agent 模式**: 无头非交互式 Agent
  - 单次 prompt 执行，支持流式输出
  - 自动启动守护进程连接
  - 适用于脚本、CI/CD、管道集成

### 🔧 技术改进

- 使用 Protobuf + gRPC 替代之前的进程内调用，支持跨进程通信
- 服务端流式 Chat 消除了之前的 4 层事件转换链
- Cobra 命令框架提供标准化的 CLI 体验
- 新增 `internal/daemon/` 包（daemon.go, session.go, server.go）
- 新增 `internal/cli/` 包（root.go, daemon.go, agent.go, version.go）
- 新增 `internal/heartbeat/` 包（heartbeat.go, snapshot.go）
- 新增 `internal/tui/client.go` gRPC 客户端封装
- 新增 `internal/proto/` 生成的 gRPC/Protobuf 代码
- 新增 `proto/kele.proto` 服务定义（8 RPCs）
- 重构 `cmd/kele/main.go` 使用 Cobra 入口
- 重构 TUI app.go, commands.go, completion.go, view.go 支持双模式
- 跨平台守护进程 fork 支持（Unix SysProcAttr / Windows 占位）

### 📦 新增依赖

- `google.golang.org/grpc` v1.79.1
- `google.golang.org/protobuf` v1.36.11
- `github.com/spf13/cobra` v1.10.2

---

## v0.2.0 - 2026-02-18

### ✨ 新功能

- **多供应商 LLM 支持**: 新增 Anthropic Claude 和 Ollama 本地模型供应商
  - 自动根据模型名推断供应商 (gpt-* → OpenAI, claude-* → Anthropic, 含 : → Ollama)
  - 运行时无缝切换模型和供应商
  - Ollama 模型自动发现 (`ListModels` 查询 `/api/tags`)

- **FTS5 全文搜索**: 记忆系统恢复并增强 FTS5 支持
  - BM25 排序的全文搜索
  - AND → OR 自动降级策略
  - FTS5 不可用时自动降级为 LIKE 查询
  - `unicode61` 分词器支持

- **审计日志系统**: 工具调用全程记录
  - JSONL 格式写入 `.kele/audit.log`
  - 记录工具名、参数摘要、结果摘要、耗时、错误
  - 线程安全的追加写入

- **API 自动重试**: LLM 调用失败自动重试
  - 最多 3 次重试，指数退避 (1s, 2s, 4s)
  - 智能判断可重试错误 (网络超时/429/5xx)

- **工具输出智能压缩**: 大输出自动头尾保留
  - 超过 2KB 阈值自动压缩
  - 保留前 75% 头部 + 后 25% 尾部
  - 标注省略字节数

- **动态记忆注入**: System prompt 自动包含最近记忆
  - 最近 5 条记忆条目注入到 system prompt
  - 增强 AI 对用户偏好的感知

- **新增 CLI 参数**
  - `--config` 指定配置文件路径
  - `--debug` 启用调试模式（日志写入 `.kele/debug.log`）

- **新增 Slash 命令**
  - `/model-info` 查看模型和供应商详细信息
  - `/load [session-id]` 列出/加载已保存的会话

- **启动时会话恢复提示**: 检测到上次会话时自动提示恢复

- **Git diff 语法高亮**: diff/show 输出添加 ANSI 颜色标记
  - 新增行 (绿色)、删除行 (红色)、位置标记 (青色)

### 🔧 技术改进

- `memory.NewStore` 改为返回 `(*Store, error)`，不再 panic
- `config.ApplyFlags` 新增 `configPath` 参数
- Brain 层所有记忆操作增加 nil 安全检查
- 新增 `internal/tools/audit.go` 审计日志模块
- 新增 `internal/agent/brain_test.go` 单元测试
- 更新 `config_test.go` 和 `store_test.go` 适配 API 变更
- 新增 `GetRecentMemories`、`GetLatestSession`、`HasFTS5` 等方法

### 🐛 Bug 修复

- 修复 memory store 初始化失败导致程序 panic 的问题
- 修复 FTS5 模块编译问题（v0.1.1 移除后重新实现，带优雅降级）

---

## v0.1.3 - 2026-02-06

### ✨ 新功能

- **环境变量支持模型配置**: 添加 `OPENAI_MODEL` 环境变量支持
  - 默认值: `gpt-4o`
  - 可通过环境变量自定义模型名称

- **丰富的 Slash 命令系统**: 参考 Claude Code，大幅扩展命令系统
  - **模型管理**: `/model`, `/models`, `/model-reset` - 运行时切换模型
  - **记忆系统**: `/remember`, `/search`, `/memory` - 完整记忆管理
  - **信息查看**: `/status`, `/config`, `/history`, `/tokens`, `/debug` - 详细状态信息
  - **会话管理**: `/save`, `/export` - 导出和保存对话
  - **对话控制**: `/clear`, `/reset`, `/exit`, `/quit` - 基础控制

- **运行时模型切换**: 无需重启即可切换不同的 AI 模型
  - 支持 OpenAI 系列 (gpt-4o, gpt-4-turbo, gpt-3.5-turbo)
  - 支持 Claude 系列 (claude-3-5-sonnet, claude-3-opus)
  - 支持任意兼容 OpenAI API 的模型

- **命令自动补全**: 按 Tab 键自动补全命令
  - 智能匹配命令前缀
  - 单个匹配时自动补全
  - 多个匹配时显示候选列表
  - 支持最长公共前缀补全

- **多任务支持**: ESC 键改为中断任务而非退出
  - ESC 键: 中断当前流式响应，保持程序运行
  - Ctrl+C: 退出程序
  - /exit, /quit: 退出程序
  - 支持随时中断任务开始新对话

### 🔧 技术改进

- 在 LLM Client 中添加模型切换支持
- 在 Agent Brain 中暴露模型管理接口
- 完全重写 `handleCommand` 函数，支持参数解析
- 添加辅助函数: `getEnv`, `truncateString`, `findCommonPrefix`
- 实现命令补全算法（前缀匹配 + 公共前缀）
- 改进按键处理逻辑，支持任务中断

### 🎨 界面改进

- 将品牌 emoji 从 🦞 改为 🥤（可乐）
- 更新状态栏和帮助文本
- 优化输入提示信息

### 📝 文档更新

- 更新所有文档添加 `OPENAI_MODEL` 说明
- 更新配置文件示例
- 创建命令使用指南

---

## v0.1.2 - 2026-02-06

### 🐛 Bug 修复

- **修复流式响应错误**: 修复"无法找到用户输入"错误
  - 重新设计流式响应架构，使用 event channel 传递
  - 添加 `streamInitMsg` 类型用于初始化流式响应
  - 修复 `startStream` 和 `continueStream` 函数逻辑
  - 在 App 结构体中保存 event channel

### 🔧 技术改进

- 优化流式响应流程，避免重复调用 `ChatStream`
- 改进错误处理，确保 channel 正确关闭

---

## v0.1.1 - 2026-02-06

### 🐛 Bug 修复

- **修复 FTS5 模块错误**: 移除 SQLite FTS5 依赖，改用普通索引和 LIKE 查询
- **修复 Module 名称**: 将 `github.com/yourusername/kele` 更正为 `github.com/BlakeLiAFK/kele`
- **添加验证脚本**: 新增 `verify.sh` 用于快速验证程序功能

### 🔧 技术改进

- 简化数据库结构，移除 FTS5 虚拟表
- 在 `memory_entries` 表添加索引优化查询性能
- 所有功能验证通过，确保程序可正常运行

---

## v0.1.0 - 2026-02-06

### 🎉 首次发布

完整实现 TUI 版 OpenClaw MVP！

### ✨ 新功能

#### 核心功能
- ✅ **LLM 集成**: 支持 OpenAI 兼容 API，流式响应
- ✅ **工具系统**: bash/read/write 三个基础工具
- ✅ **Agent 大脑**: 智能对话处理，工具调用
- ✅ **记忆系统**: SQLite + MEMORY.md + 会话日志
- ✅ **TUI 界面**: 基于 Bubble Tea 的精美终端界面

#### 交互特性
- ✅ 流式响应（打字机效果）
- ✅ 工具调用可视化
- ✅ Slash 命令系统
- ✅ 上下文管理（20 轮历史）
- ✅ 自动会话保存

#### 工具能力
- ✅ **bash**: 执行 shell 命令（带安全检查）
- ✅ **read**: 读取文件内容
- ✅ **write**: 创建/修改文件

#### 记忆功能
- ✅ SQLite 全文检索（FTS5）
- ✅ MEMORY.md 文件同步
- ✅ 会话 JSONL 日志
- ✅ 记忆搜索

### 🛠️ 技术栈

- **语言**: Go 1.25
- **TUI**: Bubble Tea + Lipgloss
- **数据库**: SQLite3
- **LLM**: OpenAI 兼容 API

### 📦 依赖

```
github.com/charmbracelet/bubbletea v1.3.10
github.com/charmbracelet/bubbles v0.21.1
github.com/charmbracelet/lipgloss v1.1.1
github.com/mattn/go-sqlite3 v1.14.33
```

### 🎯 支持的平台

- macOS (arm64/amd64)
- Linux (amd64/arm64)
- 需要 CGO 支持（SQLite）

### 📝 已知限制

- 暂不支持图片/文件上传
- 暂不支持多会话并发
- 工具调用为串行执行
- 仅支持 OpenAI 格式 API

### 🔜 计划功能

- [ ] 多 LLM 提供商支持
- [ ] 向量检索（Embeddings）
- [ ] 心跳机制（定时任务）
- [ ] Web UI 控制面板
- [ ] 插件系统

### 🐛 Bug 修复

无（首次发布）

### 📚 文档

- ✅ README.md - 项目介绍
- ✅ QUICKSTART.md - 快速开始
- ✅ USAGE.md - 使用指南
- ✅ docs/ - 完整架构文档
- ✅ plans/ - 开发计划

### 🙏 致谢

基于以下项目的灵感：
- Claude Code - TUI 交互设计
- OpenClaw - 自主能力架构
- Bubble Tea - 优秀的 TUI 框架

---

## 开发统计

- **开发时间**: 1 天
- **代码行数**: ~1500 行 Go 代码
- **测试状态**: ✅ 编译通过
- **文档完整度**: ✅ 100%
