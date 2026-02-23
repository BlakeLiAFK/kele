package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/BlakeLiAFK/kele/internal/config"
	"github.com/BlakeLiAFK/kele/internal/cron"
	pb "github.com/BlakeLiAFK/kele/internal/proto"
)

// allCommands 所有可用命令
var allCommands = []string{
	"/help", "/clear", "/reset", "/exit", "/quit",
	"/model", "/models", "/model-reset", "/model-small", "/model-info",
	"/provider",
	"/remember", "/search", "/memory",
	"/status", "/config", "/history", "/tokens", "/tools",
	"/save", "/export", "/load", "/debug",
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
	// 全局配置
	cfg *config.Config

	// Daemon 客户端（nil = standalone 模式）
	client *DaemonClient

	// 全局调度器（standalone 模式）
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

	// 鼠标模式（启用时支持滚轮滚动，禁用时支持文本选中复制）
	mouseEnabled bool

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
	question   string // ask_user 问题 JSON
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

// NewApp 创建应用（standalone 模式）
func NewApp(cfg *config.Config) *App {
	ta := textarea.New()
	ta.Placeholder = "输入消息... (Tab 补全, Ctrl+J 换行)"
	ta.Focus()
	ta.CharLimit = cfg.TUI.MaxInputChars
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	// 创建全局调度器
	wd, _ := os.Getwd()
	scheduler, err := cron.NewScheduler(cfg.Memory.DBPath, wd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cron scheduler init failed: %v\n", err)
	} else {
		scheduler.Start()
	}

	// 创建第一个会话
	firstSession := NewSession(1, scheduler, cfg)

	app := &App{
		cfg:           cfg,
		scheduler:     scheduler,
		sessions:      []*Session{firstSession},
		activeIdx:     0,
		nextSessionID: 2,
		textarea:      ta,
		completion:    NewCompletionEngine(firstSession.brain),
		mouseEnabled:  true,
	}
	app.updateStatus("Ready")
	return app
}

// NewAppWithClient 创建应用（daemon 模式，通过 gRPC 连接）
func NewAppWithClient(cfg *config.Config, grpcClient pb.KeleServiceClient) *App {
	ta := textarea.New()
	ta.Placeholder = "输入消息... (Tab 补全, Ctrl+J 换行)"
	ta.Focus()
	ta.CharLimit = cfg.TUI.MaxInputChars
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	dc := NewDaemonClient(grpcClient)

	// 创建 daemon 侧的初始会话
	info, err := dc.CreateSession("Chat 1")
	var firstSession *Session
	if err != nil {
		// 回退：创建本地占位会话
		firstSession = &Session{
			id:         1,
			name:       "Chat 1",
			messages:   []Message{},
			historyIdx: -1,
		}
	} else {
		firstSession = NewDaemonSession(1, info.Name, info.Id, info.Model, info.Provider)
	}

	app := &App{
		cfg:           cfg,
		client:        dc,
		sessions:      []*Session{firstSession},
		activeIdx:     0,
		nextSessionID: 2,
		textarea:      ta,
		completion:    NewCompletionEngineWithClient(dc, firstSession.daemonSessID),
		mouseEnabled:  true,
	}
	app.updateStatus("Ready")
	return app
}

// Close 释放应用资源
func (a *App) Close() {
	if a.scheduler != nil {
		a.scheduler.Stop()
	}
}

// isDaemonMode 检查是否使用 daemon 模式
func (a *App) isDaemonMode() bool {
	return a.client != nil
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
	// 仅 standalone 模式尝试恢复会话
	if !a.isDaemonMode() {
		a.tryRestoreSession()
	}
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

	// question overlay
	sess := a.currentSession()
	if a.overlayMode == "question" && sess.pendingQuestion != nil {
		return renderQuestionOverlay(sess.pendingQuestion, sess.questionIdx, a.width, a.height)
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
	mouseHint := "Ctrl+G 选中"
	if !a.mouseEnabled {
		mouseHint = "Ctrl+G 滚轮"
	}
	return helpStyle.Width(a.width).Render(
		fmt.Sprintf("Tab 补全 | Enter 发送 | Ctrl+J 换行 | Ctrl+E 思考 | %s | Ctrl+C x2 退出", mouseHint))
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
	if a.isDaemonMode() {
		model := sess.model
		provider := sess.provider
		if model == "" {
			model = "unknown"
		}
		if provider == "" {
			provider = "daemon"
		}
		a.statusContent = fmt.Sprintf("Kele v%s | %s (%s) | %s",
			config.Version, model, provider, status)
		return
	}
	if sess.brain != nil {
		a.statusContent = fmt.Sprintf("Kele v%s | %s (%s) | %s",
			config.Version, sess.brain.GetModel(), sess.brain.GetProviderName(), status)
	} else {
		a.statusContent = fmt.Sprintf("Kele v%s | %s", config.Version, status)
	}
}
