# MVP: TUI 版 OpenClaw

> 创建: 2026-02-06
> 状态: 进行中

## 目标

构建一个类似 Claude Code 的终端交互式 AI 助手，结合 OpenClaw 的自主能力，使用纯 Go 实现。

## 核心特性对比

### Claude Code 借鉴点
- ✅ React + Ink 风格的 TUI（Go 用 Bubble Tea）
- ✅ 实时状态栏（模型、成本、token 计数）
- ✅ 自动创建 plan.md 和 todo list
- ✅ 流式响应显示
- ✅ Slash 命令系统

### OpenClaw 借鉴点
- ✅ 本地优先，完全自主
- ✅ 执行 shell 命令能力
- ✅ 文件读写能力
- ✅ 记忆系统（MEMORY.md）
- ✅ 心跳机制（简化版）

## MVP 功能清单

### Phase 1: 基础 TUI 框架（第 1-2 天）

- [ ] **TUI 框架搭建**
  - [ ] 使用 Bubble Tea 构建基础界面
  - [ ] 实现三区域布局：状态栏、对话区、输入区
  - [ ] 支持 Markdown 渲染（Glamour）
  - [ ] 支持代码高亮

- [ ] **基础交互**
  - [ ] 输入框：多行编辑、历史记录
  - [ ] 对话区：滚动、复制
  - [ ] 快捷键：Ctrl+C 退出、Ctrl+L 清屏

### Phase 2: LLM 集成（第 3-4 天）

- [ ] **AI 对话**
  - [ ] 集成 Anthropic Claude API
  - [ ] 流式响应显示（打字机效果）
  - [ ] 上下文管理（最近 10 轮对话）
  - [ ] Token 计数和成本显示

- [ ] **工具执行**
  - [ ] 实现 3 个基础工具：
    - `bash`: 执行 shell 命令
    - `read`: 读取文件
    - `write`: 写入文件
  - [ ] 工具调用可视化

### Phase 3: 文件操作（第 5 天）

- [ ] **智能文件管理**
  - [ ] 自动识别项目根目录
  - [ ] 文件树导航（可选显示）
  - [ ] 文件内容缓存
  - [ ] 支持 .gitignore 过滤

### Phase 4: 记忆与持久化（第 6 天）

- [ ] **会话管理**
  - [ ] 会话保存到 JSONL
  - [ ] 会话列表查看 `/sessions`
  - [ ] 加载历史会话 `/load <id>`

- [ ] **记忆系统**
  - [ ] MEMORY.md 自动更新
  - [ ] 记忆检索 `/remember <query>`

### Phase 5: Slash 命令（第 7 天）

- [ ] **内置命令**
  - `/help` - 显示帮助
  - `/clear` - 清空对话
  - `/model <name>` - 切换模型
  - `/save` - 保存会话
  - `/exit` - 退出
  - `/tasks` - 查看任务列表

- [ ] **自定义命令**
  - 支持 `.kele/commands/*.md` 格式

## 技术栈

```
┌─────────────────────────────────┐
│     Bubble Tea (TUI)            │
│  ┌──────────┐  ┌──────────┐    │
│  │ Glamour  │  │ Lipgloss │    │
│  │(Markdown)│  │  (样式)  │    │
│  └──────────┘  └──────────┘    │
└─────────────────────────────────┘
              ↓
┌─────────────────────────────────┐
│       Core Logic                │
│  ┌──────────┐  ┌──────────┐    │
│  │   Chat   │  │  Tools   │    │
│  │  Engine  │  │ Executor │    │
│  └──────────┘  └──────────┘    │
└─────────────────────────────────┘
              ↓
┌─────────────────────────────────┐
│     Infrastructure              │
│  ┌──────────┐  ┌──────────┐    │
│  │Anthropic │  │ SQLite   │    │
│  │   API    │  │ (Memory) │    │
│  └──────────┘  └──────────┘    │
└─────────────────────────────────┘
```

## 依赖库

```go
// TUI
github.com/charmbracelet/bubbletea
github.com/charmbracelet/lipgloss
github.com/charmbracelet/glamour

// API
github.com/anthropics/anthropic-sdk-go

// 工具
github.com/mattn/go-sqlite3
gopkg.in/yaml.v3
```

## 界面设计

```
┌────────────────────────────────────────────────────────────────┐
│ 🥤 Kele v0.1.0 | Claude 3.5 Sonnet | $0.03 | 2.4k tokens | ⚡ │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│ You: 帮我分析这个项目的架构                                    │
│                                                                │
│ Assistant: 让我来分析项目结构...                              │
│                                                                │
│ 🔧 使用工具: bash                                              │
│ $ ls -la                                                       │
│ total 128                                                      │
│ drwxr-xr-x  15 user  staff   480 Feb  6 12:00 .               │
│ drwxr-xr-x   8 user  staff   256 Feb  6 11:00 ..              │
│ ...                                                            │
│                                                                │
│ 🔧 使用工具: read                                              │
│ 📄 reading: go.mod                                             │
│                                                                │
│ 这是一个 Go 项目，主要模块包括...                             │
│                                                                │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ │
│                                                                │
├────────────────────────────────────────────────────────────────┤
│ > 继续分析 cmd/ 目录的代码结构_                                │
│                                                                │
│ 💡 /help 查看命令 | Ctrl+C 退出 | Ctrl+L 清屏                 │
└────────────────────────────────────────────────────────────────┘
```

## 项目结构

```
kele/
├── cmd/
│   └── kele/
│       └── main.go              # 入口
├── internal/
│   ├── tui/                     # TUI 组件
│   │   ├── app.go               # 主应用
│   │   ├── chat.go              # 对话视图
│   │   ├── status.go            # 状态栏
│   │   ├── input.go             # 输入框
│   │   └── styles.go            # 样式定义
│   ├── agent/                   # AI 代理
│   │   ├── chat.go              # 对话引擎
│   │   ├── tools.go             # 工具执行器
│   │   └── context.go           # 上下文管理
│   ├── llm/                     # LLM 客户端
│   │   └── anthropic.go
│   ├── memory/                  # 记忆系统
│   │   ├── session.go           # 会话管理
│   │   └── store.go             # 持久化
│   └── commands/                # Slash 命令
│       └── registry.go
├── .kele/                       # 配置目录
│   ├── config.yaml              # 配置文件
│   ├── MEMORY.md                # 记忆文件
│   └── commands/                # 自定义命令
│       └── deploy.md
├── go.mod
└── README.md
```

## 实施步骤

### Day 1: TUI 框架

**目标**：能在终端显示基础界面

```bash
# 安装依赖
go get github.com/charmbracelet/bubbletea
go get github.com/charmbracelet/lipgloss
go get github.com/charmbracelet/glamour

# 运行
go run cmd/kele/main.go
```

**验收**：
- ✅ 显示三区域布局
- ✅ 输入框可以输入文本
- ✅ 按 Enter 显示在对话区

### Day 2: LLM 集成

**目标**：能与 Claude 对话

```bash
# 设置 API Key
export ANTHROPIC_API_KEY=sk-ant-xxx

# 运行
go run cmd/kele/main.go
```

**验收**：
- ✅ 发送消息后收到 AI 回复
- ✅ 流式显示打字机效果
- ✅ 状态栏显示 token 计数

### Day 3-4: 工具系统

**目标**：AI 可以执行操作

**验收**：
- ✅ AI 可以执行 `ls` 命令
- ✅ AI 可以读取文件内容
- ✅ AI 可以创建新文件

### Day 5-6: 记忆与会话

**目标**：持久化对话历史

**验收**：
- ✅ 会话保存到 `.kele/sessions/`
- ✅ 可以加载历史会话
- ✅ MEMORY.md 自动更新

### Day 7: Polish

**目标**：完善用户体验

**验收**：
- ✅ 支持常用 slash 命令
- ✅ 错误处理友好
- ✅ 文档完整

## 开发优先级

### P0 (必须有)
- TUI 基础框架
- Claude API 集成
- bash/read/write 工具
- 会话保存

### P1 (应该有)
- 流式响应
- Token 计数
- Slash 命令
- MEMORY.md

### P2 (可以有)
- 文件树导航
- 语法高亮
- 自定义命令
- 会话搜索

## 配置文件

```yaml
# .kele/config.yaml
llm:
  provider: anthropic
  model: claude-3-5-sonnet-20241022
  api_key: ${ANTHROPIC_API_KEY}
  max_tokens: 4096

memory:
  enabled: true
  file: .kele/MEMORY.md
  session_dir: .kele/sessions

tools:
  enabled:
    - bash
    - read
    - write

  bash:
    allowed_commands:
      - ls
      - cat
      - grep
      - find
    forbidden_commands:
      - rm -rf
      - dd

ui:
  theme: auto
  syntax_highlight: true
  typing_effect: true
```

## 参考资源

- [Claude Code TUI 架构](https://kotrotsos.medium.com/claude-code-internals-part-11-terminal-ui-542fe17db016)
- [OpenClaw 完整指南](https://www.jitendrazaa.com/blog/ai/clawdbot-complete-guide-open-source-ai-assistant-2026/)
- [Bubble Tea 文档](https://github.com/charmbracelet/bubbletea)

## 成功指标

1. **功能完整性**：能完成基本的代码分析和修改任务
2. **响应速度**：流式响应延迟 < 500ms
3. **用户体验**：界面流畅，无卡顿
4. **稳定性**：连续运行 1 小时不崩溃

## Next Steps

完成 MVP 后可以考虑：
- 支持多会话并发（类似 Claude Code）
- Web UI 版本（可选）
- 插件系统
- 多 LLM 支持（OpenAI、Gemini）
