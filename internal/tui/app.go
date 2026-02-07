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

	"github.com/BlakeLiAFK/kele/internal/cron"
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
	"/cron",
}

// Message 消息
type Message struct {
	Role     string
	Content  string
	Thinking string // 推理/思考过程内容
	IsStream bool
}

// App 主应用
type App struct {
	// 全局调度器
	scheduler *cron.Scheduler

	// 多会话
	sessions      []*Session
	activeIdx     int
	nextSessionID int // 全局递增会话 ID，避免关闭后重复

	// UI 组件
	viewport viewport.Model
	textarea textarea.Model
	width    int
	height   int
	ready    bool
	quitting bool

	// 补全
	completion      *CompletionEngine
	completionHint  string
	suggestion      string
	aiPending       bool
	completionState string // "" | "pending" | "loading" | "done" | "error"
	completionError string // 补全错误信息

	// Thinking 展示
	thinkingExpanded bool // 全局切换：是否展开思考过程
	thinkingFrame    int  // spinner 动画帧

	// 状态
	statusContent string
	overlayMode   string // "" | "settings"

	// 双击检测
	lastCtrlC time.Time
	lastEsc   time.Time
}

// streamMsg 流式消息
type streamMsg struct {
	sessionID  int    // 绑定到发起流式请求的会话
	content    string
	thinking   string // 推理/思考内容
	done       bool
	err        error
	toolName   string // 工具调用名
	toolResult string // 工具执行结果
}

// streamInitMsg 流式初始化
type streamInitMsg struct {
	sessionID int
	eventChan <-chan streamEvent
}

// streamEvent 内部流式事件
type streamEvent struct {
	Type       string // content, thinking, tool_call, tool_result, error, done
	Content    string
	ToolName   string
	ToolResult string
	Error      string
}

// tickMsg 定时器消息（驱动 spinner 动画）
type tickMsg struct{}

// spinner 帧
var spinnerFrames = []string{"\u28cb", "\u2819", "\u2839", "\u2838", "\u283c", "\u2834", "\u2826", "\u2827", "\u2807", "\u280f"}

// NewApp 创建应用
func NewApp() *App {
	ta := textarea.New()
	ta.Placeholder = "输入消息... (Tab 补全, Ctrl+J 换行)"
	ta.Focus()
	ta.CharLimit = 5000
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	// 创建全局调度器
	wd, _ := os.Getwd()
	scheduler := cron.NewScheduler(".kele/memory.db", wd)
	scheduler.Start()

	// 创建第一个会话
	firstSession := NewSession(1, scheduler)

	app := &App{
		scheduler:     scheduler,
		sessions:      []*Session{firstSession},
		activeIdx:     0,
		nextSessionID: 2,
		textarea:      ta,
		completion:    NewCompletionEngine(firstSession.brain),
	}
	app.updateStatus("Ready")
	return app
}

// currentSession 获取当前活跃会话
func (a *App) currentSession() *Session {
	return a.sessions[a.activeIdx]
}

// findSession 按 ID 查找会话（会话可能已被关闭）
func (a *App) findSession(id int) *Session {
	for _, s := range a.sessions {
		if s.id == id {
			return s
		}
	}
	return nil
}

// tick 返回一个 100ms 后发送 tickMsg 的命令
func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

// shouldTick 是否需要继续 tick（有动画需要更新）
func (a *App) shouldTick() bool {
	sess := a.currentSession()
	// thinking 阶段才需要动画（content 开始后不再需要）
	thinkingPhase := sess.streaming && sess.streamBuffer == ""
	return thinkingPhase || a.completionState == "loading"
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
	case tickMsg:
		a.thinkingFrame = (a.thinkingFrame + 1) % len(spinnerFrames)
		if a.shouldTick() {
			a.refreshViewport()
			return a, tick()
		}
		return a, nil

	case streamInitMsg:
		sess := a.findSession(msg.sessionID)
		if sess == nil {
			return a, nil
		}
		sess.eventChan = msg.eventChan
		return a, tea.Batch(a.continueStreamFor(msg.sessionID), tick())

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
	hintLine := renderCompletionHintLine(a.completionHint, a.completionState, a.completionError, a.thinkingFrame, a.width)

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
		"Tab 补全 | Enter 发送 | Ctrl+J 换行 | Ctrl+E 思考 | Ctrl+O 设置 | Ctrl+C x2 退出")
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
	sess.thinkingBuffer = ""
	a.updateStatus("Thinking...")

	a.refreshViewport()

	return a.startStream(llmInput)
}

// startStream 开始流式响应（绑定到当前会话 ID）
func (a *App) startStream(userInput string) tea.Cmd {
	sess := a.currentSession()
	sessionID := sess.id
	return func() tea.Msg {
		eventChan, err := sess.brain.ChatStream(userInput)
		if err != nil {
			return streamMsg{sessionID: sessionID, err: err}
		}
		// 适配 agent.StreamEvent → streamEvent
		internalChan := make(chan streamEvent, 100)
		go func() {
			defer close(internalChan)
			for ev := range eventChan {
				switch ev.Type {
				case "reasoning":
					internalChan <- streamEvent{Type: "thinking", Content: ev.Content}
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
		return streamInitMsg{sessionID: sessionID, eventChan: internalChan}
	}
}

// continueStreamFor 继续接收指定会话的流
func (a *App) continueStreamFor(sessionID int) tea.Cmd {
	sess := a.findSession(sessionID)
	if sess == nil {
		return nil
	}
	ch := sess.eventChan
	return func() tea.Msg {
		if ch == nil {
			return streamMsg{sessionID: sessionID, done: true}
		}
		event, ok := <-ch
		if !ok {
			return streamMsg{sessionID: sessionID, done: true}
		}
		switch event.Type {
		case "thinking":
			return streamMsg{sessionID: sessionID, thinking: event.Content}
		case "content":
			return streamMsg{sessionID: sessionID, content: event.Content}
		case "tool_call":
			return streamMsg{sessionID: sessionID, toolName: event.ToolName}
		case "tool_result":
			return streamMsg{sessionID: sessionID, toolName: event.ToolName, toolResult: event.ToolResult}
		case "error":
			return streamMsg{sessionID: sessionID, err: errors.New(event.Error)}
		default:
			return streamMsg{sessionID: sessionID, done: true}
		}
	}
}

// handleStreamMsg 处理流式消息（按 sessionID 定位会话，支持跨会话切换）
func (a *App) handleStreamMsg(msg streamMsg) tea.Cmd {
	sess := a.findSession(msg.sessionID)
	if sess == nil {
		return nil // 会话已关闭，丢弃事件
	}
	isActive := sess.id == a.currentSession().id

	if msg.err != nil {
		sess.streaming = false
		sess.taskRunning = false
		sess.eventChan = nil
		sess.AddMessage("assistant", "Error: "+msg.err.Error())
		if isActive {
			a.refreshViewport()
		}
		return nil
	}

	// 推理/思考事件
	if msg.thinking != "" {
		sess.thinkingBuffer += msg.thinking
		lastIdx := len(sess.messages) - 1
		if lastIdx >= 0 && sess.messages[lastIdx].IsStream {
			sess.messages[lastIdx].Thinking = sess.thinkingBuffer
		}
		if isActive {
			a.refreshViewport()
		}
		return a.continueStreamFor(msg.sessionID)
	}

	// 工具调用事件
	if msg.toolName != "" && msg.toolResult == "" {
		sess.taskRunning = true
		lastIdx := len(sess.messages) - 1
		if lastIdx >= 0 && sess.messages[lastIdx].IsStream {
			if sess.messages[lastIdx].Content == "" {
				sess.messages = sess.messages[:lastIdx]
			} else {
				sess.messages[lastIdx].IsStream = false
			}
		}
		sess.AddMessage("assistant", fmt.Sprintf("tool: %s", msg.toolName))
		if isActive {
			a.updateStatus(fmt.Sprintf("executing %s...", msg.toolName))
			a.refreshViewport()
		}
		return a.continueStreamFor(msg.sessionID)
	}
	if msg.toolResult != "" {
		sess.AddMessage("assistant", fmt.Sprintf("tool: %s -> %s", msg.toolName, truncateStr(msg.toolResult, 200)))
		if isActive {
			a.refreshViewport()
		}
		return a.continueStreamFor(msg.sessionID)
	}

	if msg.done {
		sess.eventChan = nil
		sess.streaming = false
		sess.taskRunning = false
		// 将 thinking 内容保留到最后的 stream 消息中
		for i := range sess.messages {
			if sess.messages[i].IsStream {
				sess.messages[i].IsStream = false
				if sess.thinkingBuffer != "" && sess.messages[i].Thinking == "" {
					sess.messages[i].Thinking = sess.thinkingBuffer
				}
			}
		}
		sess.streamBuffer = ""
		sess.thinkingBuffer = ""
		if isActive {
			a.updateStatus("Ready")
			a.refreshViewport()
		}
		return nil
	}

	// 普通内容
	lastIdx := len(sess.messages) - 1
	if lastIdx >= 0 && sess.messages[lastIdx].IsStream {
		sess.streamBuffer += msg.content
		sess.messages[lastIdx].Content = sess.streamBuffer
	} else {
		sess.streamBuffer = msg.content
		sess.AddMessage("assistant", msg.content)
		sess.messages[len(sess.messages)-1].IsStream = true
	}
	if isActive {
		a.refreshViewport()
	}
	return a.continueStreamFor(msg.sessionID)
}

// onInputChanged 输入变化时触发补全
func (a *App) onInputChanged(input string) tea.Cmd {
	a.completionHint = ""
	a.suggestion = ""
	a.completionState = ""
	a.completionError = ""
	a.currentSession().ResetHistoryNav()

	if input == "" {
		return nil
	}

	// 本地补全
	suggestions, candidates := a.completion.LocalComplete(input)
	if len(suggestions) > 0 {
		a.suggestion = suggestions[0]
		a.completionState = "done"
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
			a.completionState = "loading"
			return tea.Batch(aiCmd, tick())
		}
	}

	return nil
}

// forceComplete Tab 强制触发 AI 补全（无防抖，覆盖之前的请求）
func (a *App) forceComplete() tea.Cmd {
	input := strings.TrimSpace(a.textarea.Value())
	if input == "" || strings.HasPrefix(input, "/") {
		return nil
	}

	sess := a.currentSession()
	history := sess.brain.GetHistory()
	var recent []llm.Message
	if len(history) > 4 {
		recent = history[len(history)-4:]
	} else {
		recent = history
	}

	a.completionState = "loading"
	a.aiPending = true
	cmd := a.completion.ForceComplete(input, recent)
	if cmd != nil {
		return tea.Batch(cmd, tick())
	}
	a.completionState = ""
	a.aiPending = false
	return nil
}

// handleCompletionMsg 处理 AI 补全结果
func (a *App) handleCompletionMsg(msg completionMsg) tea.Cmd {
	a.aiPending = false
	curInput := a.textarea.Value()
	if curInput != msg.input {
		a.completionState = ""
		return nil
	}
	if msg.err != nil {
		a.completionState = "error"
		a.completionError = msg.err.Error()
		return nil
	}
	if msg.suggestion != "" {
		a.completionState = "done"
		a.suggestion = msg.suggestion
		if strings.HasPrefix(msg.suggestion, curInput) {
			hint := msg.suggestion[len(curInput):]
			if hint != "" {
				a.completionHint = curInput + "[" + hint + "]"
			}
		} else {
			a.completionHint = msg.suggestion
		}
	} else {
		a.completionState = ""
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
	id := a.nextSessionID
	a.nextSessionID++
	s := NewSession(id, a.scheduler)
	if name != "" {
		s.name = name
	}
	a.sessions = append(a.sessions, s)
	a.switchSession(len(a.sessions) - 1)
	a.updateStatus(fmt.Sprintf("新建会话: %s", s.name))
}

// switchSession 切换会话（保存/恢复输入框内容）
func (a *App) switchSession(idx int) {
	if idx < 0 || idx >= len(a.sessions) {
		return
	}
	// 保存当前会话的输入框内容
	if a.activeIdx != idx {
		a.sessions[a.activeIdx].draftInput = a.textarea.Value()
	}
	a.activeIdx = idx
	sess := a.currentSession()
	// 恢复目标会话的输入框内容
	a.textarea.SetValue(sess.draftInput)
	a.textarea.CursorEnd()
	a.completion = NewCompletionEngine(sess.brain)
	a.completionHint = ""
	a.suggestion = ""
	a.updateStatus("Ready")
	a.refreshViewport()
}

// closeSession 关闭当前会话（优先切到左边）
func (a *App) closeSession() {
	if len(a.sessions) <= 1 {
		a.currentSession().AddMessage("assistant", "无法关闭最后一个会话")
		a.refreshViewport()
		return
	}
	closedIdx := a.activeIdx
	a.sessions = append(a.sessions[:closedIdx], a.sessions[closedIdx+1:]...)
	// 优先切到左边
	newIdx := closedIdx - 1
	if newIdx < 0 {
		newIdx = 0
	}
	a.activeIdx = newIdx
	// 直接恢复目标会话状态
	sess := a.currentSession()
	a.textarea.SetValue(sess.draftInput)
	a.textarea.CursorEnd()
	a.completion = NewCompletionEngine(sess.brain)
	a.completionHint = ""
	a.suggestion = ""
	a.updateStatus("Ready")
	a.refreshViewport()
}

// refreshViewport 刷新对话区
func (a *App) refreshViewport() {
	sess := a.currentSession()
	a.viewport.SetContent(renderMessages(sess.messages, a.width, a.thinkingExpanded, a.thinkingFrame))
	a.viewport.GotoBottom()
}

// updateStatus 更新状态栏
func (a *App) updateStatus(status string) {
	sess := a.currentSession()
	a.statusContent = fmt.Sprintf("Kele | %s | %s", sess.brain.GetModel(), status)
}
