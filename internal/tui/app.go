package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/BlakeLiAFK/kele/internal/llm"
)

// 最大会话数
const maxSessions = 9

// allCommands 所有可用命令
var allCommands = []string{
	"/help", "/clear", "/reset", "/exit", "/quit",
	"/model", "/models", "/model-reset", "/model-small",
	"/remember", "/search", "/memory",
	"/status", "/config", "/history", "/tokens",
	"/save", "/export", "/debug",
	"/new", "/sessions", "/switch", "/rename",
}

// Message 消息
type Message struct {
	Role     string
	Content  string
	IsStream bool
}

// App 主应用
type App struct {
	// 多会话
	sessions  []*Session
	activeIdx int

	// UI 组件
	viewport viewport.Model
	textarea textarea.Model
	width    int
	height   int
	ready    bool
	quitting bool

	// 补全
	completion     *CompletionEngine
	completionHint string
	suggestion     string
	aiPending      bool

	// 状态
	statusContent string
	overlayMode   string // "" | "settings"

	// 双击检测
	lastCtrlC time.Time
	lastEsc   time.Time
}

// streamMsg 流式消息
type streamMsg struct {
	content   string
	done      bool
	err       error
	toolName  string // 工具调用名
	toolResult string // 工具执行结果
}

// streamInitMsg 流式初始化
type streamInitMsg struct {
	eventChan <-chan streamEvent
}

// streamEvent 内部流式事件
type streamEvent struct {
	Type       string // content, tool_call, tool_result, error, done
	Content    string
	ToolName   string
	ToolResult string
	Error      string
}

// NewApp 创建应用
func NewApp() *App {
	ta := textarea.New()
	ta.Placeholder = "输入消息... (Tab 补全, Ctrl+J 换行)"
	ta.Focus()
	ta.CharLimit = 5000
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	// 创建第一个会话
	firstSession := NewSession(1)

	app := &App{
		sessions:  []*Session{firstSession},
		activeIdx: 0,
		textarea:  ta,
		completion: NewCompletionEngine(firstSession.brain),
	}
	app.updateStatus("Ready")
	return app
}

// currentSession 获取当前活跃会话
func (a *App) currentSession() *Session {
	return a.sessions[a.activeIdx]
}

// Init 初始化
func (a *App) Init() tea.Cmd {
	return textarea.Blink
}

// Update 处理消息
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 按键事件先由 keys.go 处理
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		consumed, cmd := a.handleKeyMsg(keyMsg)
		if consumed {
			return a, cmd
		}
	}

	// 传递给子组件
	var tiCmd, vpCmd tea.Cmd
	prevInput := a.textarea.Value()
	a.textarea, tiCmd = a.textarea.Update(msg)
	a.viewport, vpCmd = a.viewport.Update(msg)

	// 处理其他消息
	switch msg := msg.(type) {
	case streamInitMsg:
		sess := a.currentSession()
		sess.eventChan = msg.eventChan
		return a, a.continueStream()

	case streamMsg:
		return a, a.handleStreamMsg(msg)

	case completionMsg:
		return a, a.handleCompletionMsg(msg)

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		vpHeight := msg.Height - 7
		if vpHeight < 3 {
			vpHeight = 3
		}
		if !a.ready {
			a.viewport = viewport.New(msg.Width, vpHeight)
			a.viewport.YPosition = 1
			a.ready = true
		} else {
			a.viewport.Width = msg.Width
			a.viewport.Height = vpHeight
		}
		a.textarea.SetWidth(msg.Width - 4)
		a.refreshViewport()
	}

	// 输入变化时触发补全
	curInput := a.textarea.Value()
	if curInput != prevInput && !a.currentSession().streaming {
		completionCmd := a.onInputChanged(curInput)
		return a, tea.Batch(tiCmd, vpCmd, completionCmd)
	}

	return a, tea.Batch(tiCmd, vpCmd)
}

// View 渲染
func (a *App) View() string {
	if !a.ready {
		return "\n  Initializing..."
	}

	// Ctrl+O 叠加层
	if a.overlayMode == "settings" {
		return renderOverlay(a, a.width, a.height)
	}

	// Tab 栏
	tabBar := renderTabBar(a.sessions, a.activeIdx, a.width)

	// 对话区
	chatArea := a.viewport.View()

	// 分隔线
	separator := separatorStyle.Width(a.width).Render(strings.Repeat("-", a.width))

	// 补全提示行
	hintLine := renderCompletionHintLine(a.completionHint, a.width)

	// 输入区
	inputArea := lipgloss.NewStyle().
		Width(a.width - 2).
		Padding(0, 1).
		Render(a.textarea.View())

	// 帮助行
	helpText := a.renderHelpLine()

	return lipgloss.JoinVertical(
		lipgloss.Left,
		tabBar,
		chatArea,
		separator,
		hintLine,
		inputArea,
		helpText,
	)
}

// renderHelpLine 渲染底部帮助行
func (a *App) renderHelpLine() string {
	return helpStyle.Width(a.width).Render(
		"Tab 补全 | Enter 发送 | Ctrl+J 换行 | Ctrl+O 设置 | Ctrl+C x2 退出")
}

// handleEnter 处理 Enter 发送
func (a *App) handleEnter() tea.Cmd {
	sess := a.currentSession()
	if sess.streaming {
		return nil
	}

	userInput := strings.TrimSpace(a.textarea.Value())
	if userInput == "" {
		return nil
	}

	// 清除补全状态
	a.completionHint = ""
	a.suggestion = ""
	a.completion.ClearCache()

	// 保存到输入历史
	sess.PushHistory(userInput)
	sess.ResetHistoryNav()

	// 斜杠命令
	if strings.HasPrefix(userInput, "/") {
		a.handleCommand(userInput)
		a.textarea.Reset()
		if a.quitting {
			return tea.Quit
		}
		return nil
	}

	// 处理 @ 引用
	cleanText, refs := parseReferences(userInput)
	llmInput := userInput
	if len(refs) > 0 {
		llmInput = buildContextMessage(cleanText, refs)
		a.updateStatus(formatRefSummary(refs))
	}

	// 添加用户消息
	sess.AddMessage("user", userInput)
	a.textarea.Reset()

	// 流式占位
	sess.AddMessage("assistant", "")
	sess.messages[len(sess.messages)-1].IsStream = true
	sess.streaming = true
	sess.streamBuffer = ""
	if len(refs) == 0 {
		a.updateStatus("thinking...")
	}

	a.refreshViewport()

	return a.startStream(llmInput)
}

// startStream 开始流式响应
func (a *App) startStream(userInput string) tea.Cmd {
	sess := a.currentSession()
	return func() tea.Msg {
		eventChan, err := sess.brain.ChatStream(userInput)
		if err != nil {
			return streamMsg{err: err}
		}
		// 适配 agent.StreamEvent → streamEvent
		internalChan := make(chan streamEvent, 100)
		go func() {
			defer close(internalChan)
			for ev := range eventChan {
				switch ev.Type {
				case "content":
					internalChan <- streamEvent{Type: "content", Content: ev.Content}
				case "tool_start":
					name := ""
					if ev.Tool != nil {
						name = ev.Tool.Name
					}
					internalChan <- streamEvent{Type: "tool_call", ToolName: name}
				case "tool_result":
					name, result := "", ""
					if ev.Tool != nil {
						name = ev.Tool.Name
						result = ev.Tool.Result
					}
					internalChan <- streamEvent{Type: "tool_result", ToolName: name, ToolResult: result}
				case "error":
					internalChan <- streamEvent{Type: "error", Error: ev.Error}
				case "done":
					internalChan <- streamEvent{Type: "done"}
				}
			}
		}()
		return streamInitMsg{eventChan: internalChan}
	}
}

// continueStream 继续接收流
func (a *App) continueStream() tea.Cmd {
	sess := a.currentSession()
	ch := sess.eventChan
	return func() tea.Msg {
		if ch == nil {
			return streamMsg{done: true}
		}
		event, ok := <-ch
		if !ok {
			return streamMsg{done: true}
		}
		switch event.Type {
		case "content":
			return streamMsg{content: event.Content}
		case "tool_call":
			return streamMsg{toolName: event.ToolName}
		case "tool_result":
			return streamMsg{toolName: event.ToolName, toolResult: event.ToolResult}
		case "error":
			return streamMsg{err: errors.New(event.Error)}
		default:
			return streamMsg{done: true}
		}
	}
}

// handleStreamMsg 处理流式消息
func (a *App) handleStreamMsg(msg streamMsg) tea.Cmd {
	sess := a.currentSession()

	if msg.err != nil {
		sess.streaming = false
		sess.taskRunning = false
		sess.eventChan = nil
		sess.AddMessage("assistant", "Error: "+msg.err.Error())
		a.refreshViewport()
		return nil
	}

	// 工具调用事件
	if msg.toolName != "" && msg.toolResult == "" {
		sess.taskRunning = true
		// 如果有空的流式占位，先定形
		lastIdx := len(sess.messages) - 1
		if lastIdx >= 0 && sess.messages[lastIdx].IsStream {
			if sess.messages[lastIdx].Content == "" {
				sess.messages = sess.messages[:lastIdx]
			} else {
				sess.messages[lastIdx].IsStream = false
			}
		}
		sess.AddMessage("assistant", fmt.Sprintf("tool: %s", msg.toolName))
		a.updateStatus(fmt.Sprintf("executing %s...", msg.toolName))
		a.refreshViewport()
		return a.continueStream()
	}
	if msg.toolResult != "" {
		sess.AddMessage("assistant", fmt.Sprintf("tool: %s -> %s", msg.toolName, truncateStr(msg.toolResult, 200)))
		a.refreshViewport()
		return a.continueStream()
	}

	if msg.done {
		sess.eventChan = nil
		sess.streaming = false
		sess.taskRunning = false
		// 定形所有流式消息
		for i := range sess.messages {
			if sess.messages[i].IsStream {
				sess.messages[i].IsStream = false
			}
		}
		sess.streamBuffer = ""
		a.updateStatus("Ready")
		a.refreshViewport()
		return nil
	}

	// 普通内容
	lastIdx := len(sess.messages) - 1
	if lastIdx >= 0 && sess.messages[lastIdx].IsStream {
		// 已有流式消息，追加内容
		sess.streamBuffer += msg.content
		sess.messages[lastIdx].Content = sess.streamBuffer
	} else {
		// 工具执行后的新内容块，创建新流式消息
		sess.streamBuffer = msg.content
		sess.AddMessage("assistant", msg.content)
		sess.messages[len(sess.messages)-1].IsStream = true
	}
	a.refreshViewport()
	return a.continueStream()
}

// onInputChanged 输入变化时触发补全
func (a *App) onInputChanged(input string) tea.Cmd {
	a.completionHint = ""
	a.suggestion = ""
	a.currentSession().ResetHistoryNav()

	if input == "" {
		return nil
	}

	// 本地补全
	suggestions, candidates := a.completion.LocalComplete(input)
	if len(suggestions) > 0 {
		a.suggestion = suggestions[0]
	}
	if len(candidates) > 0 {
		display := candidates
		if len(display) > 8 {
			display = display[:8]
			display = append(display, fmt.Sprintf("... +%d", len(candidates)-8))
		}
		a.completionHint = strings.Join(display, "  ")
	}

	// AI 补全
	if len(suggestions) == 0 && len(candidates) == 0 {
		sess := a.currentSession()
		history := sess.brain.GetHistory()
		var recent []llm.Message
		if len(history) > 4 {
			recent = history[len(history)-4:]
		} else {
			recent = history
		}
		aiCmd := a.completion.AIComplete(input, recent)
		if aiCmd != nil {
			a.aiPending = true
			return aiCmd
		}
	}

	return nil
}

// handleCompletionMsg 处理 AI 补全结果
func (a *App) handleCompletionMsg(msg completionMsg) tea.Cmd {
	a.aiPending = false
	curInput := a.textarea.Value()
	if curInput != msg.input {
		return nil
	}
	if msg.suggestion != "" {
		a.suggestion = msg.suggestion
		if strings.HasPrefix(msg.suggestion, curInput) {
			hint := msg.suggestion[len(curInput):]
			if hint != "" {
				a.completionHint = curInput + "[" + hint + "]"
			}
		} else {
			a.completionHint = msg.suggestion
		}
	}
	return nil
}

// -- 会话管理 --

// createSession 创建新会话
func (a *App) createSession(name string) {
	if len(a.sessions) >= maxSessions {
		a.currentSession().AddMessage("assistant", fmt.Sprintf("已达最大会话数 %d", maxSessions))
		a.refreshViewport()
		return
	}
	id := len(a.sessions) + 1
	s := NewSession(id)
	if name != "" {
		s.name = name
	}
	a.sessions = append(a.sessions, s)
	a.switchSession(len(a.sessions) - 1)
	a.updateStatus(fmt.Sprintf("新建会话: %s", s.name))
}

// switchSession 切换会话
func (a *App) switchSession(idx int) {
	if idx < 0 || idx >= len(a.sessions) {
		return
	}
	a.activeIdx = idx
	sess := a.currentSession()
	a.completion = NewCompletionEngine(sess.brain)
	a.completionHint = ""
	a.suggestion = ""
	a.updateStatus("Ready")
	a.refreshViewport()
}

// closeSession 关闭当前会话
func (a *App) closeSession() {
	if len(a.sessions) <= 1 {
		a.currentSession().AddMessage("assistant", "无法关闭最后一个会话")
		a.refreshViewport()
		return
	}
	a.sessions = append(a.sessions[:a.activeIdx], a.sessions[a.activeIdx+1:]...)
	if a.activeIdx >= len(a.sessions) {
		a.activeIdx = len(a.sessions) - 1
	}
	a.switchSession(a.activeIdx)
}

// refreshViewport 刷新对话区
func (a *App) refreshViewport() {
	sess := a.currentSession()
	a.viewport.SetContent(renderMessages(sess.messages, a.width))
	a.viewport.GotoBottom()
}

// updateStatus 更新状态栏
func (a *App) updateStatus(status string) {
	sess := a.currentSession()
	a.statusContent = fmt.Sprintf("Kele | %s | %s", sess.brain.GetModel(), status)
}
