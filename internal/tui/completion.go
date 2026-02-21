package tui

import (
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/BlakeLiAFK/kele/internal/agent"
	"github.com/BlakeLiAFK/kele/internal/llm"
)

// completionMsg AI 补全结果消息
type completionMsg struct {
	input      string // 发起请求时的输入（用于校验是否过期）
	suggestion string // 补全建议（完整文本，含已输入部分）
	err        error  // 补全错误
}

// CompletionEngine 补全引擎
type CompletionEngine struct {
	brain           *agent.Brain  // standalone 模式
	daemonClient    *DaemonClient // daemon 模式
	daemonSessionID string        // daemon 模式下的会话 ID
	debounceMs      int
	mu              sync.Mutex
	cache           map[string]string
	lastInput       string
}

// NewCompletionEngine 创建补全引擎（standalone 模式）
func NewCompletionEngine(brain *agent.Brain) *CompletionEngine {
	return &CompletionEngine{
		brain:      brain,
		debounceMs: 500,
		cache:      make(map[string]string),
	}
}

// NewCompletionEngineWithClient 创建补全引擎（daemon 模式）
func NewCompletionEngineWithClient(client *DaemonClient, sessionID string) *CompletionEngine {
	return &CompletionEngine{
		daemonClient:    client,
		daemonSessionID: sessionID,
		debounceMs:      500,
		cache:           make(map[string]string),
	}
}

// isDaemonMode 是否使用 daemon 模式补全
func (e *CompletionEngine) isDaemonMode() bool {
	return e.daemonClient != nil
}

// LocalComplete 本地即时补全（斜杠命令 + @文件路径）
// 返回：suggestions 列表（完整文本，供 textinput.SetSuggestions 使用）+ 候选显示列表
func (e *CompletionEngine) LocalComplete(input string) (suggestions []string, candidates []string) {
	if input == "" {
		return nil, nil
	}

	// 斜杠命令补全
	if strings.HasPrefix(input, "/") {
		return e.completeSlashCommand(input)
	}

	// @引用补全：找到最后一个 @ 并补全其后的路径
	lastAt := strings.LastIndex(input, "@")
	if lastAt >= 0 && lastAt == len(input)-1 {
		// 刚输入 @，列出当前目录
		_, fileCandidates := completeFilePath("")
		for _, c := range fileCandidates {
			candidates = append(candidates, "@"+c)
		}
		return nil, candidates
	}
	if lastAt >= 0 {
		partial := input[lastAt+1:]
		// 确保 partial 不含空格（空格后 @ 引用结束）
		if !strings.Contains(partial, " ") {
			prefix := input[:lastAt+1]
			completed, fileCandidates := completeFilePath(partial)
			if len(fileCandidates) == 1 {
				suggestions = []string{prefix + completed}
			} else if len(fileCandidates) > 1 && len(completed) > len(partial) {
				suggestions = []string{prefix + completed}
			}
			for _, c := range fileCandidates {
				candidates = append(candidates, "@"+c)
			}
			return suggestions, candidates
		}
	}

	return nil, nil
}

// completeSlashCommand 斜杠命令补全
func (e *CompletionEngine) completeSlashCommand(input string) (suggestions []string, candidates []string) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil, nil
	}
	// 只补全命令本身（第一个 token），如果已有参数则不补全
	if len(parts) > 1 {
		return nil, nil
	}

	prefix := strings.ToLower(parts[0])
	for _, cmd := range allCommands {
		if strings.HasPrefix(strings.ToLower(cmd), prefix) {
			suggestions = append(suggestions, cmd+" ")
			candidates = append(candidates, cmd)
		}
	}

	// 精确匹配则不再提示候选
	if len(suggestions) == 1 && strings.TrimSpace(suggestions[0]) == input {
		return nil, nil
	}

	return suggestions, candidates
}

// AIComplete 异步 AI 补全（返回 tea.Cmd，带防抖）
// history 参数在 daemon 模式下被忽略（daemon 自行管理历史）
func (e *CompletionEngine) AIComplete(input string, history []llm.Message) tea.Cmd {
	if input == "" || strings.HasPrefix(input, "/") || strings.Contains(input, "@") {
		return nil
	}

	// 太短不触发
	if len([]rune(input)) < 3 {
		return nil
	}

	e.mu.Lock()
	e.lastInput = input
	// 查缓存
	if cached, ok := e.cache[input]; ok {
		e.mu.Unlock()
		return func() tea.Msg {
			return completionMsg{input: input, suggestion: cached}
		}
	}
	e.mu.Unlock()

	// 防抖：延迟后请求
	debounce := time.Duration(e.debounceMs) * time.Millisecond

	if e.isDaemonMode() {
		client := e.daemonClient
		sessID := e.daemonSessionID
		return func() tea.Msg {
			time.Sleep(debounce)

			e.mu.Lock()
			if e.lastInput != input {
				e.mu.Unlock()
				return completionMsg{input: input}
			}
			e.mu.Unlock()

			result, err := client.Complete(sessID, input)
			if err != nil {
				return completionMsg{input: input, err: err}
			}
			return e.processResult(input, result)
		}
	}

	// Standalone 模式
	if e.brain == nil {
		return nil
	}
	return func() tea.Msg {
		time.Sleep(debounce)

		// 检查输入是否已变化（防抖核心）
		e.mu.Lock()
		if e.lastInput != input {
			e.mu.Unlock()
			return completionMsg{input: input}
		}
		e.mu.Unlock()

		// 调用小模型
		result, err := e.brain.Complete(input, history)
		if err != nil {
			return completionMsg{input: input, err: err}
		}
		return e.processResult(input, result)
	}
}

// ForceComplete 强制 AI 补全（无防抖、无最小长度限制）
func (e *CompletionEngine) ForceComplete(input string, history []llm.Message) tea.Cmd {
	if input == "" || strings.HasPrefix(input, "/") {
		return nil
	}

	e.mu.Lock()
	e.lastInput = input
	if cached, ok := e.cache[input]; ok {
		e.mu.Unlock()
		return func() tea.Msg {
			return completionMsg{input: input, suggestion: cached}
		}
	}
	e.mu.Unlock()

	if e.isDaemonMode() {
		client := e.daemonClient
		sessID := e.daemonSessionID
		return func() tea.Msg {
			result, err := client.Complete(sessID, input)
			if err != nil {
				return completionMsg{input: input, err: err}
			}
			return e.processResult(input, result)
		}
	}

	// Standalone 模式
	if e.brain == nil {
		return nil
	}
	return func() tea.Msg {
		result, err := e.brain.Complete(input, history)
		if err != nil {
			return completionMsg{input: input, err: err}
		}
		return e.processResult(input, result)
	}
}

// processResult 处理补全结果（缓存 + 格式化）
func (e *CompletionEngine) processResult(input, result string) completionMsg {
	if result == "" {
		return completionMsg{input: input}
	}

	// 确保返回值以当前输入为前缀
	if !strings.HasPrefix(result, input) {
		result = input + result
	}

	// 结果与输入相同则无意义
	if strings.TrimSpace(result) == strings.TrimSpace(input) {
		return completionMsg{input: input}
	}

	// 缓存
	e.mu.Lock()
	e.cache[input] = result
	if len(e.cache) > 100 {
		e.cache = make(map[string]string)
	}
	e.mu.Unlock()

	return completionMsg{input: input, suggestion: result}
}

// ClearCache 清理缓存
func (e *CompletionEngine) ClearCache() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cache = make(map[string]string)
}
