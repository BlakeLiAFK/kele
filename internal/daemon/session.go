package daemon

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BlakeLiAFK/kele/internal/config"
	"github.com/BlakeLiAFK/kele/internal/llm"
	"github.com/BlakeLiAFK/kele/internal/memory"
	"github.com/BlakeLiAFK/kele/internal/tools"
)

// Session represents a daemon-side chat session with its own conversation history.
type Session struct {
	ID        string
	Name      string
	brain     *SessionBrain
	mu        sync.Mutex
	streaming bool
}

// SessionBrain holds per-session conversation state with shared resources.
type SessionBrain struct {
	mu              sync.RWMutex
	provider        *llm.ProviderManager
	executor        *tools.Executor
	memory          *memory.Store
	history         []llm.Message
	cfg             *config.Config
	injectedContext string // additional context prepended to system prompt
}

// SessionManager manages all active sessions.
type SessionManager struct {
	sessions map[string]*Session
	provider *llm.ProviderManager
	executor *tools.Executor
	memory   *memory.Store
	cfg      *config.Config
	counter  int
	mu       sync.RWMutex
}

// NewSessionManager creates a new session manager with shared resources.
func NewSessionManager(provider *llm.ProviderManager, executor *tools.Executor, store *memory.Store, cfg *config.Config) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		provider: provider,
		executor: executor,
		memory:   store,
		cfg:      cfg,
	}
}

// Create creates a new session and returns it.
func (sm *SessionManager) Create(name string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.counter++
	id := fmt.Sprintf("s%d", sm.counter)
	if name == "" {
		name = fmt.Sprintf("Chat %d", sm.counter)
	}

	sess := &Session{
		ID:   id,
		Name: name,
		brain: &SessionBrain{
			provider: sm.provider,
			executor: sm.executor,
			memory:   sm.memory,
			history:  []llm.Message{},
			cfg:      sm.cfg,
		},
	}
	sm.sessions[id] = sess
	return sess
}

// Get returns a session by ID.
func (sm *SessionManager) Get(id string) *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[id]
}

// Delete removes a session.
func (sm *SessionManager) Delete(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, id)
}

// List returns all active sessions.
func (sm *SessionManager) List() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	result := make([]*Session, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		result = append(result, s)
	}
	return result
}

// Count returns the number of active sessions.
func (sm *SessionManager) Count() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

// --- SessionBrain methods (mirror agent.Brain but with shared resources) ---

// ChatStream starts a streaming chat with tool auto-loop.
func (sb *SessionBrain) ChatStream(userInput string) (<-chan ChatEvent, error) {
	eventChan := make(chan ChatEvent, 100)

	go func() {
		defer close(eventChan)

		sb.addMessage("user", userInput)

		maxToolRounds := sb.cfg.LLM.MaxToolRounds
		var finalContent string

		for round := 0; round < maxToolRounds; round++ {
			llmEvents := sb.provider.ChatStream(sb.getMessages(), sb.executor.GetTools())

			roundContent := ""
			var pendingToolCalls []llm.ToolCall
			gotToolCalls := false

			for event := range llmEvents {
				switch event.Type {
				case "reasoning":
					eventChan <- ChatEvent{Type: "thinking", Content: event.ReasoningContent}
				case "content":
					roundContent += event.Content
					eventChan <- ChatEvent{Type: "content", Content: event.Content}
				case "tool_calls":
					gotToolCalls = true
					pendingToolCalls = event.ToolCalls
				case "error":
					errStr := ""
					if event.Error != nil {
						errStr = event.Error.Error()
					}
					eventChan <- ChatEvent{Type: "error", Error: errStr}
					return
				case "done":
					if roundContent != "" {
						sb.addMessage("assistant", roundContent)
						finalContent = roundContent
					}
					if sb.memory != nil {
						sb.memory.SaveMessage("user", userInput)
						if finalContent != "" {
							sb.memory.SaveMessage("assistant", finalContent)
						}
					}
					eventChan <- ChatEvent{Type: "done"}
					return
				}
			}

			if gotToolCalls {
				assistantMsg := llm.Message{
					Role:      "assistant",
					Content:   roundContent,
					ToolCalls: pendingToolCalls,
				}
				sb.appendRawMessage(assistantMsg)

				for _, tc := range pendingToolCalls {
					eventChan <- ChatEvent{
						Type:     "tool_call",
						ToolName: tc.Function.Name,
					}

					result, err := sb.executor.Execute(tc)
					if err != nil {
						result = fmt.Sprintf("Error: %v", err)
					}
					result = sb.compressToolOutput(result)

					sb.appendRawMessage(llm.Message{
						Role:       "tool",
						Content:    result,
						ToolCallID: tc.ID,
					})

					eventChan <- ChatEvent{
						Type:       "tool_result",
						ToolName:   tc.Function.Name,
						ToolResult: result,
					}
				}
				continue
			}

			if roundContent != "" {
				sb.addMessage("assistant", roundContent)
				finalContent = roundContent
			}
			if sb.memory != nil {
				sb.memory.SaveMessage("user", userInput)
				if finalContent != "" {
					sb.memory.SaveMessage("assistant", finalContent)
				}
			}
			eventChan <- ChatEvent{Type: "done"}
			return
		}

		sb.addMessage("assistant", "[reached max tool rounds]")
		eventChan <- ChatEvent{Type: "done"}
	}()

	return eventChan, nil
}

// ChatEvent is the daemon-side chat event (maps directly to proto.ChatEvent).
type ChatEvent struct {
	Type       string
	Content    string
	ToolName   string
	ToolResult string
	Error      string
}

// Complete performs AI input completion.
func (sb *SessionBrain) Complete(input string) (string, error) {
	// 在锁内拷贝 history
	sb.mu.RLock()
	historyCopy := make([]llm.Message, len(sb.history))
	copy(historyCopy, sb.history)
	sb.mu.RUnlock()

	systemMsg := llm.Message{
		Role: "system",
		Content: `你是输入补全助手。根据对话上下文和用户当前输入，预测用户接下来要输入的完整文本。
规则：
- 返回完整的预测文本（包含用户已输入的部分）
- 简短，10-30字即可
- 无法预测则返回空字符串
- 不要加引号、解释或其他内容，只返回预测文本本身`,
	}

	messages := []llm.Message{systemMsg}
	recent := historyCopy
	if len(recent) > 4 {
		recent = recent[len(recent)-4:]
	}
	messages = append(messages, recent...)
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: fmt.Sprintf("当前输入: %s", input),
	})

	return sb.provider.Complete(messages, 60)
}

// RunCommand executes a slash command and returns formatted output.
func (sb *SessionBrain) RunCommand(command string) (string, bool) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", false
	}

	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "/help":
		return fmt.Sprintf(`Kele v%s 命令帮助

对话控制
  /clear, /reset   清空对话历史

模型管理
  /model <name>     切换大模型（自动匹配供应商）
  /model-small <n>  切换小模型
  /models           列出可用模型
  /model-reset      重置为默认模型
  /model-info       显示模型详细信息

工具与记忆
  /tools            列出所有可用工具
  /remember <text>  添加到长期记忆
  /search <query>   搜索记忆
  /memory           查看记忆摘要

定时任务
  /cron             查看定时任务列表

配置管理
  /config           列出所有配置项
  /config set k v   设置配置项
  /config get k     获取配置项

信息查看
  /status           显示系统状态
  /history          显示完整对话历史
  /tokens           显示 token 估算

会话导出
  /save             保存当前会话
  /export           导出对话为 Markdown`, config.Version), false

	case "/clear", "/reset":
		sb.mu.Lock()
		sb.history = []llm.Message{}
		sb.mu.Unlock()
		return "对话已清空", false

	case "/model":
		if len(args) == 0 {
			return fmt.Sprintf("当前大模型: %s\n供应商: %s\n默认模型: %s\n小模型: %s\n\n使用 /model <name> 切换",
				sb.provider.GetModel(), sb.provider.GetActiveProviderName(),
				sb.provider.GetDefaultModel(), sb.provider.GetSmallModel()), false
		}
		modelName := strings.Join(args, " ")
		sb.provider.SetModel(modelName)
		return fmt.Sprintf("已切换模型: %s (供应商: %s)", modelName, sb.provider.GetActiveProviderName()), false

	case "/model-small":
		if len(args) == 0 {
			return fmt.Sprintf("当前小模型: %s\n\n使用 /model-small <name> 切换", sb.provider.GetSmallModel()), false
		}
		modelName := strings.Join(args, " ")
		sb.provider.SetSmallModel(modelName)
		return fmt.Sprintf("已切换小模型: %s", modelName), false

	case "/models":
		providers := sb.provider.ListProviders()
		var s strings.Builder
		s.WriteString("可用模型列表\n\n")
		s.WriteString(fmt.Sprintf("已注册供应商: %s\n", strings.Join(providers, ", ")))
		s.WriteString(fmt.Sprintf("当前: %s (%s)\n\n", sb.provider.GetModel(), sb.provider.GetActiveProviderName()))
		s.WriteString("OpenAI:\n  gpt-4o, gpt-4o-mini, gpt-4-turbo, o1-preview\n\n")
		s.WriteString("Anthropic Claude:\n  claude-sonnet-4-5-20250929, claude-haiku-4-5-20251001\n\n")
		s.WriteString("DeepSeek (OpenAI 兼容):\n  deepseek-chat, deepseek-reasoner\n\n")
		s.WriteString("Ollama 本地模型 (名称含 :):\n  llama3:8b, qwen2:7b, codellama:13b")
		return s.String(), false

	case "/model-reset":
		sb.provider.ResetModel()
		return fmt.Sprintf("已重置为默认模型: %s (%s)", sb.provider.GetDefaultModel(), sb.provider.GetActiveProviderName()), false

	case "/model-info":
		var s strings.Builder
		s.WriteString("模型详细信息\n\n")
		s.WriteString(fmt.Sprintf("  供应商:       %s\n", sb.provider.GetActiveProviderName()))
		s.WriteString(fmt.Sprintf("  当前模型:     %s\n", sb.provider.GetModel()))
		s.WriteString(fmt.Sprintf("  默认模型:     %s\n", sb.provider.GetDefaultModel()))
		s.WriteString(fmt.Sprintf("  小模型:       %s\n", sb.provider.GetSmallModel()))
		s.WriteString(fmt.Sprintf("  工具支持:     %v\n", sb.provider.ActiveSupportsTools()))
		s.WriteString(fmt.Sprintf("  已注册供应商: %s\n", strings.Join(sb.provider.ListProviders(), ", ")))
		return s.String(), false

	case "/tools":
		toolNames := sb.executor.ListTools()
		var s strings.Builder
		s.WriteString(fmt.Sprintf("可用工具 (%d 个)\n\n", len(toolNames)))
		for i, name := range toolNames {
			s.WriteString(fmt.Sprintf("  %d. %s\n", i+1, name))
		}
		s.WriteString("\nAI 会根据对话内容自动调用工具")
		return s.String(), false

	case "/status":
		return fmt.Sprintf(`系统状态

版本: Kele v%s
供应商: %s
可用供应商: %s
大模型: %s
小模型: %s
Token 估算: ~%d
时间: %s`,
			config.Version,
			sb.provider.GetActiveProviderName(),
			strings.Join(sb.provider.ListProviders(), ", "),
			sb.provider.GetModel(), sb.provider.GetSmallModel(),
			sb.estimateTokens(),
			time.Now().Format("2006-01-02 15:04:05")), false

	case "/config":
		if len(args) == 0 {
			all := config.AllSettings(sb.cfg)
			dbValues, _ := config.ListValues()

			keys := make([]string, 0, len(all))
			for k := range all {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			var s strings.Builder
			s.WriteString(fmt.Sprintf("Kele v%s 配置\n\n", config.Version))
			for _, k := range keys {
				v := all[k]
				if v == "" {
					v = "(未设置)"
				}
				tag := ""
				if _, ok := dbValues[k]; ok {
					tag = " [db]"
				}
				s.WriteString(fmt.Sprintf("  %-28s %s%s\n", k, v, tag))
			}
			s.WriteString("\n/config set <key> <value>  设置配置")
			s.WriteString("\n/config get <key>          获取配置")
			return s.String(), false
		}
		subCmd := args[0]
		subArgs := args[1:]
		switch subCmd {
		case "set":
			if len(subArgs) < 2 {
				return "用法: /config set <key> <value>", false
			}
			key := subArgs[0]
			val := strings.Join(subArgs[1:], " ")
			if err := config.SetValue(key, val); err != nil {
				return fmt.Sprintf("设置失败: %v", err), false
			}
			return fmt.Sprintf("%s = %s", key, val), false
		case "get":
			if len(subArgs) < 1 {
				return "用法: /config get <key>", false
			}
			val, err := config.GetValue(subArgs[0])
			if err != nil {
				return fmt.Sprintf("获取失败: %v", err), false
			}
			return fmt.Sprintf("%s = %s", subArgs[0], val), false
		default:
			return fmt.Sprintf("未知子命令: /config %s\n用法: /config [set|get]", subCmd), false
		}

	case "/history":
		sb.mu.RLock()
		historyCopy := make([]llm.Message, len(sb.history))
		copy(historyCopy, sb.history)
		sb.mu.RUnlock()

		var s strings.Builder
		s.WriteString("对话历史\n\n")
		for i, msg := range historyCopy {
			content := msg.Content
			if len([]rune(content)) > 100 {
				content = string([]rune(content)[:100]) + "..."
			}
			s.WriteString(fmt.Sprintf("%d. [%s] %s\n\n", i+1, msg.Role, content))
		}
		if len(historyCopy) == 0 {
			s.WriteString("(暂无历史记录)")
		}
		return s.String(), false

	case "/remember":
		if len(args) == 0 {
			return "用法: /remember <要记住的内容>", false
		}
		text := strings.Join(args, " ")
		key := fmt.Sprintf("note_%d", time.Now().Unix())
		if sb.memory == nil {
			return "记忆系统未初始化", false
		}
		if err := sb.memory.UpdateMemory(key, text); err != nil {
			return fmt.Sprintf("保存失败: %v", err), false
		}
		return "已添加到长期记忆", false

	case "/search":
		if len(args) == 0 {
			return "用法: /search <搜索关键词>", false
		}
		query := strings.Join(args, " ")
		if sb.memory == nil {
			return "记忆系统未初始化", false
		}
		results, err := sb.memory.Search(query, 5)
		if err != nil {
			return fmt.Sprintf("搜索失败: %v", err), false
		}
		if len(results) == 0 {
			return "未找到相关记忆", false
		}
		var s strings.Builder
		s.WriteString(fmt.Sprintf("搜索结果 (%d 条):\n\n", len(results)))
		for i, r := range results {
			s.WriteString(fmt.Sprintf("%d. %s\n\n", i+1, r))
		}
		return s.String(), false

	case "/memory":
		return fmt.Sprintf("记忆系统\n\n命令:\n  /remember <text>  添加到长期记忆\n  /search <query>   搜索记忆\n\n存储: %s", sb.cfg.Memory.DBPath), false

	case "/tokens":
		tokens := sb.estimateTokens()
		return fmt.Sprintf("Token 估算\n\n  历史消息数: %d\n  估算 Tokens: ~%d\n  模型: %s (%s)",
			len(sb.history), tokens, sb.provider.GetModel(), sb.provider.GetActiveProviderName()), false

	case "/cron":
		return "定时任务通过 daemon 管理\n\n通过对话让 AI 帮你创建，例如：\n  \"每5分钟检查一次磁盘空间\"", false

	case "/exit", "/quit":
		return "再见!", true

	default:
		return fmt.Sprintf("未知命令: %s\n输入 /help 查看可用命令", command), false
	}
}

// --- TaskBoard integration ---

// InjectContext prepends additional context to the session's system prompt.
func (sb *SessionBrain) InjectContext(ctx string) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.injectedContext = ctx
}

// GetID returns the session's ID.
func (s *Session) GetID() string {
	return s.ID
}

// InjectContext delegates to the brain.
func (s *Session) InjectContext(ctx string) {
	s.brain.InjectContext(ctx)
}

// ChatStreamForTask wraps ChatStream, converting events to taskboard.SessionEvent.
func (s *Session) ChatStreamForTask(input string) (<-chan TaskSessionEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	eventChan, err := s.brain.ChatStream(input)
	if err != nil {
		return nil, err
	}

	outCh := make(chan TaskSessionEvent, 100)
	go func() {
		defer close(outCh)
		for ev := range eventChan {
			outCh <- TaskSessionEvent{
				Type:       ev.Type,
				Content:    ev.Content,
				ToolName:   ev.ToolName,
				ToolResult: ev.ToolResult,
				Error:      ev.Error,
			}
		}
	}()
	return outCh, nil
}

// TaskSessionEvent mirrors taskboard.SessionEvent to avoid import cycle.
type TaskSessionEvent struct {
	Type       string
	Content    string
	ToolName   string
	ToolResult string
	Error      string
}

// --- internal helpers ---

func (sb *SessionBrain) addMessage(role, content string) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.history = append(sb.history, llm.Message{Role: role, Content: content})
	sb.trimHistory()
}

func (sb *SessionBrain) appendRawMessage(msg llm.Message) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.history = append(sb.history, msg)
	sb.trimHistory()
}

func (sb *SessionBrain) trimHistory() {
	maxTurns := sb.cfg.LLM.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 20
	}
	if len(sb.history) > maxTurns*2 {
		sb.history = sb.history[len(sb.history)-maxTurns*2:]
	}
}

func (sb *SessionBrain) getMessages() []llm.Message {
	// 在锁内拷贝 history 和 injectedContext
	sb.mu.RLock()
	historyCopy := make([]llm.Message, len(sb.history))
	copy(historyCopy, sb.history)
	injected := sb.injectedContext
	sb.mu.RUnlock()

	// 后续操作无需持锁
	toolNames := sb.executor.ListTools()
	toolList := strings.Join(toolNames, ", ")

	systemContent := fmt.Sprintf(`你是 Kele，一个智能的终端 AI 助手。你可以：
1. 回答问题和进行对话
2. 使用工具执行操作（可用工具: %s）
3. 管理定时任务（cron）

请用中文回答，保持简洁专业。当需要执行操作时，主动使用工具。`, toolList)

	if injected != "" {
		systemContent += "\n\n## 工作区上下文\n" + injected
	}

	if sb.memory != nil {
		memories, err := sb.memory.GetRecentMemories(5)
		if err == nil && len(memories) > 0 {
			systemContent += "\n\n## 用户长期记忆\n"
			for _, m := range memories {
				systemContent += "- " + m + "\n"
			}
		}
	}

	messages := []llm.Message{{Role: "system", Content: systemContent}}
	messages = append(messages, historyCopy...)
	return messages
}

// HistoryLen 返回历史消息数量（并发安全）
func (sb *SessionBrain) HistoryLen() int {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	return len(sb.history)
}

func (sb *SessionBrain) estimateTokens() int {
	total := 0
	for _, msg := range sb.getMessages() {
		total += len(msg.Content)/4 + 4
	}
	return total
}

func (sb *SessionBrain) compressToolOutput(output string) string {
	maxSize := sb.cfg.Tools.MaxOutputSize
	if maxSize <= 0 {
		maxSize = 51200
	}
	if len(output) > maxSize {
		output = output[:maxSize] + fmt.Sprintf("\n\n... [输出被截断，原始 %d 字节]", len(output))
	}
	compressThreshold := 2048
	if len(output) > compressThreshold {
		headSize := compressThreshold * 3 / 4
		tailSize := compressThreshold / 4
		omitted := len(output) - headSize - tailSize
		output = output[:headSize] +
			fmt.Sprintf("\n\n... [省略 %d 字节] ...\n\n", omitted) +
			output[len(output)-tailSize:]
	}
	return output
}
