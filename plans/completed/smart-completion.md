# 智能补全系统

> 创建: 2026-02-07
> 状态: 已完成

## 目标

实现 Claude Code 风格的智能补全：输入框内 ghost text 提示 + Tab 接受 + 大小模型分离架构

## 问题分析

当前补全机制的问题：
1. **hint 只在状态栏** - `showInlineHint()` 更新 `statusContent`，用户看不到
2. **textarea 不支持 ghost text** - bubbletea 的 textarea 组件没有 suggestion 功能
3. **无 AI 补全** - 只有本地静态前缀匹配，没有智能预测
4. **无大小模型分离** - 所有请求都用同一个模型

## 方案设计

### 核心架构：三层补全 + 双模型

```
用户输入
  |
  v
[第一层] 本地即时补全 (0ms)
  - /命令前缀匹配
  - @文件路径匹配
  |
  v (如果第一层无结果)
[第二层] AI 智能补全 (500ms 防抖)
  - 小模型快速预测
  - 结合对话上下文
  |
  v
[textinput.SetSuggestions()] → ghost text 显示
  |
  v
[Tab] → 接受补全
```

### 1. 输入组件：textarea → textinput

**为什么换？**
- `textinput` 原生支持 `SetSuggestions()` + `ShowSuggestions`
- 自动显示 ghost text（灰色补全文本）
- Tab 键内置接受补全逻辑
- `textarea` 没有任何 suggestion 支持，hack 成本高且不稳定

**trade-off：**
- 失去多行输入（textarea 支持 3 行）
- 但 Claude Code 本身也是单行输入，chat 工具单行足够
- 未来可加 Shift+Enter 多行模式

### 2. 双模型架构

```
环境变量:
  OPENAI_MODEL       → 大模型（主对话）    默认 gpt-4o
  KELE_SMALL_MODEL   → 小模型（补全用）    默认回落到大模型

示例配置:
  export OPENAI_MODEL=gpt-4o
  export KELE_SMALL_MODEL=gpt-4o-mini
```

LLM Client 改动：
- 新增 `smallModel` 字段
- 新增 `Complete(prompt string, maxTokens int) (string, error)` - 非流式快速补全
- `Complete` 使用 smallModel，低 temperature，短 maxTokens

### 3. 补全引擎（internal/tui/completion.go）

```go
type CompletionEngine struct {
    brain       *agent.Brain
    debounceMs  int           // 防抖时间（默认 500ms）
    lastInput   string        // 上次输入（去重）
    cache       map[string]string // 补全缓存
}

// LocalComplete 本地即时补全（斜杠命令 + @引用）
func (e *CompletionEngine) LocalComplete(input string) []string

// AIComplete 请求 AI 补全（异步，返回 tea.Cmd）
func (e *CompletionEngine) AIComplete(input string, history []Message) tea.Cmd
```

补全策略：
- 斜杠命令 `/mo` → `["/model", "/models", "/model-reset"]` → 即时
- @引用 `@mai` → `["@main.go"]` → 即时
- 普通文本 `帮我写一个` → AI 预测 `帮我写一个Python脚本来处理JSON` → 500ms 防抖

AI 补全 Prompt：
```
系统: 你是输入补全助手。根据对话上下文和用户当前输入，预测用户接下来要输入的内容。
     只返回补全的完整文本（包含已输入部分），简短（10-30字）。无法预测则返回空。
用户: [最近2条对话]\n当前输入: {input}
```

### 4. 界面布局（关键 UX 决策）

补全信息放在**输入框正上方**，而非顶部状态栏。用户视线在底部，补全就在眼前。

```
+------------------------------------------+
| 状态栏: Kele v0.1.3 | 模型: gpt-4o       |  ← 只放系统状态
|                                          |
|   对话消息区 (viewport)                    |
|   You: 帮我分析一下代码                     |
|   Assistant: 好的，我来看看...              |
|                                          |
|------------------------------------------|
| /model  /models  /model-reset            |  ← 补全候选行（输入框正上方）
| > /mo|del                                |  ← 输入框（textinput, ghost text 内联）
| Tab 补全 | ESC 中断 | Ctrl+C 退出         |  ← 底部帮助行
+------------------------------------------+
```

View() 布局从上到下：
1. statusBar - 系统状态（模型名、连接状态）
2. chatArea (viewport) - 对话消息
3. separator - 分隔线
4. **completionLine** - 补全候选（新增，输入框正上方）
5. inputArea (textinput) - 用户输入 + ghost text
6. helpText - 快捷键提示

补全候选行的显示规则：
- 无匹配时：空行（不占空间或显示空）
- 斜杠命令多个匹配：`/model  /models  /model-reset`
- @引用多个匹配：`@main.go  @Makefile  @memory/`
- AI 补全等待中：`...` 或空
- AI 补全就绪：不额外显示（ghost text 已在输入框内）

### 5. textinput suggestion 工作流

```
用户输入 "/mo"
  → LocalComplete → ["/model ", "/models ", "/model-reset "]
  → textinput.SetSuggestions(["/model ", "/models ", "/model-reset "])
  → ghost text 内联: "/mo|del "（灰色部分）
  → 补全候选行: "/model  /models  /model-reset"
  → Tab → 接受 → 输入变成 "/model "

用户输入 "帮我写"
  → LocalComplete → 无匹配
  → 补全候选行清空
  → 启动 500ms 防抖
  → AI 返回 "帮我写一个排序算法"
  → textinput.SetSuggestions(["帮我写一个排序算法"])
  → ghost text 内联: "帮我写|一个排序算法"
  → Tab → 接受
```

## 步骤

- [x] 步骤 1: 双模型支持 - 修改 `internal/llm/client.go`
  - 新增 `smallModel` 字段
  - 读取 `KELE_SMALL_MODEL` 环境变量
  - 新增 `Complete()` 方法（非流式，用 smallModel，低 temperature，短 maxTokens）
  - 新增 `GetSmallModel()` 方法

- [x] 步骤 2: Brain 层暴露补全接口 - 修改 `internal/agent/brain.go`
  - 新增 `Complete(input string, recentHistory []llm.Message) (string, error)`
  - 透传到 llmClient.Complete
  - 新增 `GetSmallModel()` / `SetSmallModel()`

- [x] 步骤 3: 补全引擎 - 新建 `internal/tui/completion.go`
  - CompletionEngine 结构体
  - LocalComplete() - 斜杠命令 + @文件路径即时匹配
  - AIComplete() - 异步 AI 补全（返回 tea.Cmd）
  - 防抖机制 + 缓存 + 过期失效

- [x] 步骤 4: TUI 重构 - 修改 `internal/tui/app.go`
  - textarea → textinput（含 ShowSuggestions 配置）
  - 集成 CompletionEngine
  - 每次按键：先更新本地 suggestions，再触发防抖 AI 补全
  - 收到 completionMsg 时更新 suggestions
  - View() 布局调整：
    - statusBar → chatArea → separator → **completionLine** → inputArea → helpText
    - completionLine 新增：在输入框正上方显示候选列表
    - 补全候选行无内容时显示为空行，不干扰布局
  - 保持 Enter/ESC/Ctrl+C 快捷键行为

- [x] 步骤 5: 配置展示 - 更新命令
  - `/config` 显示大模型 + 小模型
  - `/help` 说明补全功能
  - `/model-small <name>` 命令切换小模型
  - `/status` 显示小模型信息

- [x] 步骤 6: 测试验证
  - 编译通过
  - 本地补全测试（斜杠命令、@引用）
  - AI 补全测试（需要 API key）
  - 回落机制测试（不设 KELE_SMALL_MODEL）
  - 全部测试通过

## 完成标准

- [ ] 输入 `/mo` 时输入框内显示灰色 ghost text `del`
- [ ] 输入 `@mai` 时显示灰色 `n.go`
- [ ] 输入普通文本停顿后，AI 预测显示 ghost text
- [ ] Tab 键接受补全
- [ ] KELE_SMALL_MODEL 可配置，不配则回落到大模型
- [ ] /config 正确显示大小模型信息
- [ ] 编译通过，现有测试通过

## 文件变更清单

| 文件 | 操作 | 说明 |
|------|------|------|
| internal/llm/client.go | 修改 | 新增 smallModel + Complete() |
| internal/agent/brain.go | 修改 | 暴露 Complete() |
| internal/tui/completion.go | 新建 | 补全引擎 |
| internal/tui/app.go | 重构 | textarea→textinput，集成补全 |
| internal/tui/reference.go | 不变 | @引用解析逻辑不变 |
| internal/tui/styles.go | 可能微调 | 适配新布局 |

## 备注

- textinput 的 SetSuggestions 是同步的，AI 补全是异步的
- 每次按键先同步设置本地 suggestions，AI 结果回来后异步覆盖
- 防抖：用户连续打字时不触发 AI，停 500ms 后才请求
- 缓存：相同输入不重复请求
- AI 补全返回太慢时，如果用户已继续打字，丢弃过期结果
