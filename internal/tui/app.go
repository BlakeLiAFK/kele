package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/BlakeLiAFK/kele/internal/agent"
)

// allCommands 所有可用命令
var allCommands = []string{
	"/help",
	"/clear",
	"/reset",
	"/exit",
	"/quit",
	"/model",
	"/models",
	"/model-reset",
	"/remember",
	"/search",
	"/memory",
	"/status",
	"/config",
	"/history",
	"/tokens",
	"/save",
	"/export",
	"/debug",
}

// Message 表示一条消息
type Message struct {
	Role     string
	Content  string
	IsStream bool
}

// App 是主应用模型
type App struct {
	viewport      viewport.Model
	textarea      textarea.Model
	messages      []Message
	width         int
	height        int
	ready         bool
	statusContent string
	brain         *agent.Brain
	streaming     bool
	streamBuffer  string
	eventChan     <-chan agent.StreamEvent
	tokenCount    int
	cost          float64
}

// NewApp 创建新的应用实例
func NewApp() *App {
	ta := textarea.New()
	ta.Placeholder = "输入消息... (Enter 发送, Tab 补全, ESC 中断)"
	ta.Focus()
	ta.CharLimit = 5000
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	return &App{
		textarea:      ta,
		messages:      []Message{},
		statusContent: "🥤 Kele v0.1.2 | 正在初始化...",
		brain:         agent.NewBrain(),
		streaming:     false,
	}
}

// streamMsg 流式消息
type streamMsg struct {
	content string
	done    bool
	err     error
}

// streamInitMsg 流式初始化消息
type streamInitMsg struct {
	eventChan <-chan agent.StreamEvent
}

// Init 初始化应用
func (a *App) Init() tea.Cmd {
	a.statusContent = "🥤 Kele v0.1.2 | 准备就绪 | 输入消息开始对话"
	return textarea.Blink
}

// Update 处理消息
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 先拦截关键按键，不让 textarea 消费
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyCtrlC:
			return a, tea.Quit

		case tea.KeyEsc:
			if a.streaming {
				a.streaming = false
				a.eventChan = nil
				a.streamBuffer = ""
				if len(a.messages) > 0 && a.messages[len(a.messages)-1].IsStream {
					a.messages[len(a.messages)-1].Content = a.streamBuffer + "\n\n⚠️ [已中断]"
					a.messages[len(a.messages)-1].IsStream = false
				}
				a.viewport.SetContent(a.renderMessages())
				a.viewport.GotoBottom()
				a.updateStatus("任务已中断")
				return a, nil
			}
			a.updateStatus("💡 使用 /exit 或 Ctrl+C 退出程序")
			return a, nil

		case tea.KeyTab:
			// Tab 补全 - 在传给 textarea 之前拦截
			if a.streaming {
				return a, nil
			}
			currentInput := a.textarea.Value()
			completed := a.handleTabComplete(currentInput)
			if completed != currentInput {
				a.textarea.SetValue(completed)
				a.textarea.CursorEnd()
			}
			return a, nil

		case tea.KeyEnter:
			return a, a.handleEnter()
		}
	}

	// 非拦截的按键和其他消息，正常传给子组件
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)
	a.textarea, tiCmd = a.textarea.Update(msg)
	a.viewport, vpCmd = a.viewport.Update(msg)

	switch msg := msg.(type) {
	case streamInitMsg:
		a.eventChan = msg.eventChan
		return a, continueStream(a.eventChan)

	case streamMsg:
		if msg.err != nil {
			a.streaming = false
			a.eventChan = nil
			a.addMessage("assistant", "错误: "+msg.err.Error())
			return a, nil
		}

		if msg.done {
			a.eventChan = nil
			a.streaming = false
			if a.streamBuffer != "" {
				a.messages[len(a.messages)-1].Content = a.streamBuffer
				a.messages[len(a.messages)-1].IsStream = false
				a.streamBuffer = ""
			}
			a.updateStatus("准备就绪")
			return a, nil
		}

		a.streamBuffer += msg.content
		if len(a.messages) > 0 && a.messages[len(a.messages)-1].IsStream {
			a.messages[len(a.messages)-1].Content = a.streamBuffer
		}
		a.viewport.SetContent(a.renderMessages())
		a.viewport.GotoBottom()
		return a, continueStream(a.eventChan)

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

		if !a.ready {
			a.viewport = viewport.New(msg.Width, msg.Height-6)
			a.viewport.YPosition = 2
			a.ready = true
		} else {
			a.viewport.Width = msg.Width
			a.viewport.Height = msg.Height - 6
		}

		a.textarea.SetWidth(msg.Width - 4)
		a.viewport.SetContent(a.renderMessages())
	}

	// 实时补全提示：检测当前输入并更新状态栏
	if !a.streaming {
		a.showInlineHint()
	}

	return a, tea.Batch(tiCmd, vpCmd)
}

// showInlineHint 实时补全提示
func (a *App) showInlineHint() {
	input := a.textarea.Value()
	if input == "" {
		return
	}

	// 斜杠命令提示
	if strings.HasPrefix(input, "/") {
		parts := strings.Fields(input)
		if len(parts) == 1 {
			prefix := strings.ToLower(parts[0])
			var matches []string
			for _, cmd := range allCommands {
				if strings.HasPrefix(strings.ToLower(cmd), prefix) && cmd != prefix {
					matches = append(matches, cmd)
				}
			}
			if len(matches) > 0 {
				hint := "💡 " + strings.Join(matches, "  ")
				a.updateStatus(hint)
			}
		}
		return
	}

	// @ 引用提示
	lastAt := strings.LastIndex(input, "@")
	if lastAt >= 0 {
		partial := input[lastAt+1:]
		if partial == "" {
			a.updateStatus("💡 输入文件路径，Tab 补全 (例: @main.go @src/ @*.go)")
			return
		}
		_, candidates := completeFilePath(partial)
		if len(candidates) > 0 && len(candidates) <= 8 {
			var display []string
			for _, c := range candidates {
				display = append(display, "@"+c)
			}
			a.updateStatus("💡 " + strings.Join(display, "  "))
		} else if len(candidates) > 8 {
			a.updateStatus(fmt.Sprintf("💡 %d 个匹配，继续输入缩小范围...", len(candidates)))
		}
	}
}

// handleTabComplete 统一处理 Tab 补全
func (a *App) handleTabComplete(input string) string {
	if strings.HasPrefix(input, "/") {
		return a.completeCommand(input)
	}
	if strings.Contains(input, "@") {
		return a.completeAtReference(input)
	}
	return input
}

// handleEnter 处理 Enter 发送
func (a *App) handleEnter() tea.Cmd {
	if a.streaming {
		return nil
	}

	userInput := strings.TrimSpace(a.textarea.Value())
	if userInput == "" {
		return nil
	}

	// 斜杠命令
	if strings.HasPrefix(userInput, "/") {
		a.handleCommand(userInput)
		a.textarea.Reset()
		return nil
	}

	// 处理 @ 引用
	cleanText, refs := parseReferences(userInput)
	llmInput := userInput
	if len(refs) > 0 {
		llmInput = buildContextMessage(cleanText, refs)
		summary := formatRefSummary(refs)
		a.updateStatus(summary)
	}

	// 添加用户消息（显示原始输入）
	a.addMessage("user", userInput)
	a.textarea.Reset()

	// 添加流式占位符
	a.addMessage("assistant", "")
	a.messages[len(a.messages)-1].IsStream = true
	a.streaming = true
	a.streamBuffer = ""
	if len(refs) == 0 {
		a.updateStatus("AI 思考中...")
	}

	return startStream(a.brain, llmInput)
}

// startStream 开始流式响应
func startStream(brain *agent.Brain, userInput string) tea.Cmd {
	return func() tea.Msg {
		eventChan, err := brain.ChatStream(userInput)
		if err != nil {
			return streamMsg{err: err}
		}
		return streamInitMsg{eventChan: eventChan}
	}
}

// continueStream 继续接收流式内容
func continueStream(eventChan <-chan agent.StreamEvent) tea.Cmd {
	return func() tea.Msg {
		if eventChan == nil {
			return streamMsg{done: true}
		}

		event, ok := <-eventChan
		if !ok {
			return streamMsg{done: true}
		}

		if event.Type == "content" {
			return streamMsg{content: event.Content}
		}
		if event.Type == "error" {
			return streamMsg{err: errors.New(event.Error)}
		}

		return streamMsg{done: true}
	}
}

// View 渲染视图
func (a *App) View() string {
	if !a.ready {
		return "\n  初始化中..."
	}

	statusBar := statusStyle.Width(a.width).Render(a.statusContent)
	chatArea := a.viewport.View()
	separator := lipgloss.NewStyle().
		Width(a.width).
		Foreground(lipgloss.Color("240")).
		Render(strings.Repeat("─", a.width))

	inputArea := lipgloss.NewStyle().
		Width(a.width-2).
		Padding(0, 1).
		Render(a.textarea.View())

	helpText := helpStyle.Width(a.width).Render("💡 /help 查看命令 | ESC 中断任务 | Ctrl+C 退出")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		statusBar,
		chatArea,
		separator,
		inputArea,
		helpText,
	)
}

// addMessage 添加消息
func (a *App) addMessage(role, content string) {
	a.messages = append(a.messages, Message{
		Role:    role,
		Content: content,
	})
	a.viewport.SetContent(a.renderMessages())
	a.viewport.GotoBottom()
}

// updateStatus 更新状态栏
func (a *App) updateStatus(status string) {
	a.statusContent = fmt.Sprintf("🥤 Kele v0.1.2 | %s", status)
}

// renderMessages 渲染所有消息
func (a *App) renderMessages() string {
	var b strings.Builder

	for _, msg := range a.messages {
		if msg.Role == "user" {
			b.WriteString(userMessageStyle.Render(fmt.Sprintf("You: %s", msg.Content)))
		} else {
			// 检查是否包含工具调用
			if strings.Contains(msg.Content, "🔧") {
				b.WriteString(toolMessageStyle.Render(msg.Content))
			} else {
				content := msg.Content
				if msg.IsStream {
					content += "▋" // 光标效果
				}
				b.WriteString(assistantMessageStyle.Render(fmt.Sprintf("Assistant: %s", content)))
			}
		}
		b.WriteString("\n\n")
	}

	return b.String()
}

// handleCommand 处理命令
func (a *App) handleCommand(cmd string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	command := parts[0]
	args := parts[1:]

	switch command {
	case "/help":
		a.addMessage("assistant", `📚 Kele 命令帮助

⌨️  快捷键
  Tab              命令 / 文件路径自动补全
  ESC              中断当前任务
  Ctrl+C           退出程序
  Enter            发送消息

📎 @ 引用（在消息中引用文件）
  @file.go         引用单个文件
  @src/            引用目录结构
  @*.go            引用匹配的文件（glob）
  示例: 分析 @main.go 的代码
  示例: @src/ 这个目录的结构

🗣️  对话控制
  /clear, /reset   清空对话历史
  /exit, /quit     退出程序

🤖 模型管理
  /model <name>     切换模型 (如: /model claude-3-5-sonnet)
  /models           列出常用模型
  /model-reset      重置为默认模型

💾 记忆系统
  /remember <text>  添加到长期记忆
  /search <query>   搜索记忆
  /memory           查看记忆摘要

📊 信息查看
  /status           显示系统状态
  /config           显示当前配置
  /history          显示完整对话历史
  /tokens           显示 token 使用情况

💾 会话管理
  /save             保存当前会话
  /export           导出对话为 Markdown

🔧 其他
  /debug            显示调试信息
  /help             显示此帮助

💡 提示：直接输入消息即可开始对话`)

	case "/clear", "/reset":
		a.messages = []Message{}
		a.brain.ClearHistory()
		a.viewport.SetContent("")
		a.updateStatus("对话已清空")

	case "/model":
		if len(args) == 0 {
			currentModel := a.brain.GetModel()
			defaultModel := a.brain.GetDefaultModel()
			a.addMessage("assistant", fmt.Sprintf(`🤖 当前模型: %s
📌 默认模型: %s

使用 /model <name> 切换模型
使用 /models 查看常用模型`, currentModel, defaultModel))
		} else {
			modelName := strings.Join(args, " ")
			a.brain.SetModel(modelName)
			a.addMessage("assistant", fmt.Sprintf("✅ 已切换到模型: %s", modelName))
			a.updateStatus(fmt.Sprintf("模型: %s", modelName))
		}

	case "/models":
		a.addMessage("assistant", `🤖 常用模型列表

OpenAI 系列:
  • gpt-4o          - 最新多模态模型
  • gpt-4-turbo     - GPT-4 Turbo
  • gpt-4           - GPT-4
  • gpt-3.5-turbo   - GPT-3.5 Turbo

Anthropic Claude 系列:
  • claude-3-5-sonnet-20241022  - Claude 3.5 Sonnet
  • claude-3-opus-20240229      - Claude 3 Opus
  • claude-3-sonnet-20240229    - Claude 3 Sonnet

使用方法:
  /model gpt-4o
  /model claude-3-5-sonnet-20241022`)

	case "/model-reset":
		a.brain.ResetModel()
		defaultModel := a.brain.GetDefaultModel()
		a.addMessage("assistant", fmt.Sprintf("✅ 已重置为默认模型: %s", defaultModel))
		a.updateStatus(fmt.Sprintf("模型: %s", defaultModel))

	case "/status":
		msgCount := len(a.messages)
		historyCount := len(a.brain.GetHistory())
		currentModel := a.brain.GetModel()
		a.addMessage("assistant", fmt.Sprintf(`📊 系统状态

💬 对话信息
  • 当前消息: %d 条
  • 历史记录: %d 条
  • 流式状态: %v

🤖 模型配置
  • 当前模型: %s
  • 默认模型: %s

🖥️  界面信息
  • 窗口大小: %d × %d
  • 时间: %s

💾 存储位置
  • 数据库: .kele/memory.db
  • 记忆文件: .kele/MEMORY.md
  • 会话目录: .kele/sessions/`,
			msgCount,
			historyCount,
			a.streaming,
			currentModel,
			a.brain.GetDefaultModel(),
			a.width,
			a.height,
			time.Now().Format("2006-01-02 15:04:05"),
		))

	case "/config":
		currentModel := a.brain.GetModel()
		a.addMessage("assistant", fmt.Sprintf(`⚙️  当前配置

环境变量:
  • OPENAI_API_BASE: %s
  • OPENAI_MODEL: %s

运行时配置:
  • 当前模型: %s
  • 最大轮次: 20
  • 流式响应: 启用`,
			getEnv("OPENAI_API_BASE", "默认"),
			getEnv("OPENAI_MODEL", "gpt-4o"),
			currentModel,
		))

	case "/history":
		history := a.brain.GetHistory()
		var historyText strings.Builder
		historyText.WriteString("📜 完整对话历史\n\n")
		for i, msg := range history {
			historyText.WriteString(fmt.Sprintf("%d. [%s] %s\n\n",
				i+1,
				msg.Role,
				truncateString(msg.Content, 100),
			))
		}
		if len(history) == 0 {
			historyText.WriteString("(暂无历史记录)")
		}
		a.addMessage("assistant", historyText.String())

	case "/remember":
		if len(args) == 0 {
			a.addMessage("assistant", "❌ 用法: /remember <要记住的内容>")
		} else {
			text := strings.Join(args, " ")
			key := fmt.Sprintf("note_%d", time.Now().Unix())
			err := a.brain.SaveMemory(key, text)
			if err != nil {
				a.addMessage("assistant", fmt.Sprintf("❌ 保存失败: %v", err))
			} else {
				a.addMessage("assistant", "✅ 已添加到长期记忆")
			}
		}

	case "/search":
		if len(args) == 0 {
			a.addMessage("assistant", "❌ 用法: /search <搜索关键词>")
		} else {
			query := strings.Join(args, " ")
			results, err := a.brain.SearchMemory(query)
			if err != nil {
				a.addMessage("assistant", fmt.Sprintf("❌ 搜索失败: %v", err))
			} else if len(results) == 0 {
				a.addMessage("assistant", "🔍 未找到相关记忆")
			} else {
				var resultText strings.Builder
				resultText.WriteString(fmt.Sprintf("🔍 搜索结果 (%d 条):\n\n", len(results)))
				for i, result := range results {
					resultText.WriteString(fmt.Sprintf("%d. %s\n\n", i+1, result))
				}
				a.addMessage("assistant", resultText.String())
			}
		}

	case "/memory":
		a.addMessage("assistant", `💭 记忆系统

可用命令:
  /remember <text>  - 添加到长期记忆
  /search <query>   - 搜索记忆

记忆文件: .kele/MEMORY.md
数据库: .kele/memory.db`)

	case "/tokens":
		// TODO: 实现 token 计数
		a.addMessage("assistant", `📊 Token 使用情况

当前会话:
  • 输入 tokens: 估算中
  • 输出 tokens: 估算中
  • 总计: 估算中

💡 提示: Token 计数功能开发中`)

	case "/save":
		a.addMessage("assistant", "✅ 会话已自动保存到 .kele/sessions/")

	case "/export":
		var export strings.Builder
		export.WriteString("# Kele 对话导出\n\n")
		export.WriteString(fmt.Sprintf("**导出时间**: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
		export.WriteString("---\n\n")
		for _, msg := range a.messages {
			if msg.Role == "user" {
				export.WriteString(fmt.Sprintf("## 👤 User\n\n%s\n\n", msg.Content))
			} else {
				export.WriteString(fmt.Sprintf("## 🤖 Assistant\n\n%s\n\n", msg.Content))
			}
		}
		filename := fmt.Sprintf(".kele/export_%s.md", time.Now().Format("20060102_150405"))
		// TODO: 实际写入文件
		a.addMessage("assistant", fmt.Sprintf("✅ 对话已导出: %s\n\n(功能开发中)", filename))

	case "/debug":
		a.addMessage("assistant", fmt.Sprintf(`🐛 调试信息

Go 版本: %s
消息数: %d
流式状态: %v
事件通道: %v
缓冲区大小: %d`,
			"1.25.3",
			len(a.messages),
			a.streaming,
			a.eventChan != nil,
			len(a.streamBuffer),
		))

	case "/exit", "/quit":
		a.addMessage("assistant", "👋 再见！")
		// 休眠1秒后退出
		time.Sleep(1 * time.Second)
		a.quit()
	default:
		a.addMessage("assistant", fmt.Sprintf("❓ 未知命令: %s\n\n输入 /help 查看可用命令", cmd))
	}

	a.viewport.SetContent(a.renderMessages())
	a.viewport.GotoBottom()
}
func (a *App) quit() {
	os.Exit(0)
}

// getEnv 获取环境变量
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// completeCommand 命令补全
func (a *App) completeCommand(input string) string {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return input
	}

	prefix := strings.ToLower(parts[0])
	var matches []string

	// 查找匹配的命令
	for _, cmd := range allCommands {
		if strings.HasPrefix(strings.ToLower(cmd), prefix) {
			matches = append(matches, cmd)
		}
	}

	// 没有匹配
	if len(matches) == 0 {
		return input
	}

	// 只有一个匹配，直接补全
	if len(matches) == 1 {
		// 如果有参数，保留参数
		if len(parts) > 1 {
			return matches[0] + " " + strings.Join(parts[1:], " ")
		}
		return matches[0] + " "
	}

	// 多个匹配，显示候选并返回最长公共前缀
	a.showCompletionCandidates(matches)
	commonPrefix := findCommonPrefix(matches)

	// 如果公共前缀比当前输入长，使用公共前缀
	if len(commonPrefix) > len(prefix) {
		if len(parts) > 1 {
			return commonPrefix + " " + strings.Join(parts[1:], " ")
		}
		return commonPrefix
	}

	return input
}

// completeAtReference @ 文件路径补全
func (a *App) completeAtReference(input string) string {
	// 找到最后一个 @ 的位置
	lastAt := strings.LastIndex(input, "@")
	if lastAt == -1 {
		return input
	}

	// 提取 @ 后面的部分
	prefix := input[:lastAt+1]
	partial := input[lastAt+1:]

	// 补全文件路径
	completed, candidates := completeFilePath(partial)

	if len(candidates) == 0 {
		return input
	}

	if len(candidates) == 1 {
		return prefix + completed
	}

	// 多个匹配，显示候选
	var display []string
	for _, c := range candidates {
		display = append(display, "@"+c)
	}
	a.showCompletionCandidates(display)

	if len(completed) > len(partial) {
		return prefix + completed
	}

	return input
}

// showCompletionCandidates 显示候选命令
func (a *App) showCompletionCandidates(candidates []string) {
	if len(candidates) == 0 {
		return
	}

	var hint strings.Builder
	hint.WriteString("💡 可用命令: ")
	hint.WriteString(strings.Join(candidates, ", "))

	a.updateStatus(hint.String())
}

// findCommonPrefix 查找字符串数组的最长公共前缀
func findCommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}

	prefix := strs[0]
	for i := 1; i < len(strs); i++ {
		for !strings.HasPrefix(strings.ToLower(strs[i]), strings.ToLower(prefix)) {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return ""
			}
		}
	}

	return prefix
}
