package agent

import (
	"fmt"
	"strings"
	"sync"

	"github.com/BlakeLiAFK/kele/internal/config"
	"github.com/BlakeLiAFK/kele/internal/cron"
	"github.com/BlakeLiAFK/kele/internal/llm"
	"github.com/BlakeLiAFK/kele/internal/memory"
	"github.com/BlakeLiAFK/kele/internal/tools"
)

// Brain AI 大脑
type Brain struct {
	mu       sync.RWMutex
	provider *llm.ProviderManager
	executor *tools.Executor
	memory   *memory.Store
	history  []llm.Message
	cfg      *config.Config
}

// NewBrain 创建新大脑
func NewBrain(scheduler *cron.Scheduler, cfg *config.Config) *Brain {
	store, err := memory.NewStore(cfg)
	if err != nil {
		// 记忆系统初始化失败不应阻止启动，打印警告继续
		fmt.Printf("警告: 记忆系统初始化失败: %v\n", err)
	}
	return &Brain{
		provider: llm.NewProviderManager(cfg),
		executor: tools.NewExecutor(scheduler, cfg),
		memory:   store,
		history:  []llm.Message{},
		cfg:      cfg,
	}
}

// Chat 处理对话（非流式，带自动工具调用循环）
func (b *Brain) Chat(userInput string) (string, error) {
	b.addMessage("user", userInput)

	maxRounds := b.cfg.LLM.MaxToolRounds
	var allResults []string

	for round := 0; round < maxRounds; round++ {
		resp, err := b.provider.Chat(b.getMessages(), b.executor.GetTools())
		if err != nil {
			return strings.Join(allResults, "\n\n"), err
		}

		if len(resp.Choices) == 0 {
			return strings.Join(allResults, "\n\n"), fmt.Errorf("API 返回空响应")
		}

		choice := resp.Choices[0]

		if len(choice.Message.ToolCalls) > 0 {
			b.appendRawMessage(choice.Message)
			for _, tc := range choice.Message.ToolCalls {
				result, err := b.executor.Execute(tc)
				if err != nil {
					result = fmt.Sprintf("Error: %v", err)
				}
				result = b.compressToolOutput(result)
				allResults = append(allResults, fmt.Sprintf("tool %s:\n%s", tc.Function.Name, result))
				b.appendRawMessage(llm.Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
				})
			}
			continue
		}

		content := choice.Message.Content
		b.addMessage("assistant", content)
		if b.memory != nil {
			b.memory.SaveMessage("user", userInput)
			b.memory.SaveMessage("assistant", content)
		}
		allResults = append(allResults, content)
		return strings.Join(allResults, "\n\n"), nil
	}

	return strings.Join(allResults, "\n\n"), nil
}

// ChatStream 流式对话（支持工具调用自动循环）
func (b *Brain) ChatStream(userInput string) (<-chan StreamEvent, error) {
	eventChan := make(chan StreamEvent, 100)

	go func() {
		defer close(eventChan)

		b.addMessage("user", userInput)

		maxToolRounds := b.cfg.LLM.MaxToolRounds
		var finalContent string

		for round := 0; round < maxToolRounds; round++ {
			llmEvents := b.provider.ChatStream(b.getMessages(), b.executor.GetTools())

			roundContent := ""
			var pendingToolCalls []llm.ToolCall
			gotToolCalls := false

			for event := range llmEvents {
				switch event.Type {
				case "reasoning":
					eventChan <- StreamEvent{Type: "reasoning", Content: event.ReasoningContent}

				case "content":
					roundContent += event.Content
					eventChan <- StreamEvent{Type: "content", Content: event.Content}

				case "tool_calls":
					gotToolCalls = true
					pendingToolCalls = event.ToolCalls

				case "error":
					var errStr string
					if event.Error != nil {
						errStr = event.Error.Error()
					}
					eventChan <- StreamEvent{Type: "error", Error: errStr}
					return

				case "done":
					if roundContent != "" {
						b.addMessage("assistant", roundContent)
						finalContent = roundContent
					}
					if b.memory != nil {
						b.memory.SaveMessage("user", userInput)
						if finalContent != "" {
							b.memory.SaveMessage("assistant", finalContent)
						}
					}
					eventChan <- StreamEvent{Type: "done"}
					return
				}
			}

			if gotToolCalls {
				assistantMsg := llm.Message{
					Role:      "assistant",
					Content:   roundContent,
					ToolCalls: pendingToolCalls,
				}
				b.appendRawMessage(assistantMsg)

				for _, tc := range pendingToolCalls {
					eventChan <- StreamEvent{
						Type: "tool_start",
						Tool: &ToolExecution{Name: tc.Function.Name},
					}

					result, err := b.executor.Execute(tc)
					if err != nil {
						result = fmt.Sprintf("Error: %v", err)
					}
					result = b.compressToolOutput(result)

					b.appendRawMessage(llm.Message{
						Role:       "tool",
						Content:    result,
						ToolCallID: tc.ID,
					})

					eventChan <- StreamEvent{
						Type: "tool_result",
						Tool: &ToolExecution{
							Name:   tc.Function.Name,
							Result: result,
						},
					}
				}
				continue
			}

			if roundContent != "" {
				b.addMessage("assistant", roundContent)
				finalContent = roundContent
			}
			if b.memory != nil {
				b.memory.SaveMessage("user", userInput)
				if finalContent != "" {
					b.memory.SaveMessage("assistant", finalContent)
				}
			}
			eventChan <- StreamEvent{Type: "done"}
			return
		}

		b.addMessage("assistant", "[reached max tool rounds]")
		eventChan <- StreamEvent{Type: "done"}
	}()

	return eventChan, nil
}

// compressToolOutput 智能压缩工具输出
// 输出 > 2KB 时保留头尾，中间部分省略
func (b *Brain) compressToolOutput(output string) string {
	maxSize := b.cfg.Tools.MaxOutputSize
	if maxSize <= 0 {
		maxSize = 51200
	}

	// 先做硬截断
	if len(output) > maxSize {
		output = output[:maxSize] + fmt.Sprintf("\n\n... [输出被截断，原始 %d 字节]", len(output))
	}

	// 智能压缩：输出超过 2KB 时保留头尾
	compressThreshold := 2048
	if len(output) > compressThreshold {
		headSize := compressThreshold * 3 / 4 // 前 75%
		tailSize := compressThreshold / 4      // 后 25%
		omitted := len(output) - headSize - tailSize
		output = output[:headSize] +
			fmt.Sprintf("\n\n... [省略 %d 字节] ...\n\n", omitted) +
			output[len(output)-tailSize:]
	}

	return output
}

func (b *Brain) addMessage(role, content string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.history = append(b.history, llm.Message{Role: role, Content: content})
	b.trimHistory()
}

func (b *Brain) appendRawMessage(msg llm.Message) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.history = append(b.history, msg)
	b.trimHistory()
}

func (b *Brain) trimHistory() {
	maxTurns := b.cfg.LLM.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 20
	}
	if len(b.history) > maxTurns*2 {
		b.history = b.history[len(b.history)-maxTurns*2:]
	}
}

func (b *Brain) getMessages() []llm.Message {
	// 在锁内拷贝 history
	b.mu.RLock()
	historyCopy := make([]llm.Message, len(b.history))
	copy(historyCopy, b.history)
	b.mu.RUnlock()

	// 后续操作无需持锁
	toolNames := b.executor.ListTools()
	toolList := strings.Join(toolNames, ", ")

	systemContent := fmt.Sprintf(`你是 Kele，一个智能的终端 AI 助手。你可以：
1. 回答问题和进行对话
2. 使用工具执行操作（可用工具: %s）
3. 管理定时任务（cron）

请用中文回答，保持简洁专业。当需要执行操作时，主动使用工具。`, toolList)

	// 动态注入相关记忆到 system prompt
	if b.memory != nil {
		memories, err := b.memory.GetRecentMemories(5)
		if err == nil && len(memories) > 0 {
			systemContent += "\n\n## 用户长期记忆\n"
			for _, m := range memories {
				systemContent += "- " + m + "\n"
			}
		}
	}

	systemPrompt := llm.Message{
		Role:    "system",
		Content: systemContent,
	}

	messages := []llm.Message{systemPrompt}
	messages = append(messages, historyCopy...)
	return messages
}

func (b *Brain) GetHistory() []llm.Message {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cp := make([]llm.Message, len(b.history))
	copy(cp, b.history)
	return cp
}

func (b *Brain) ClearHistory() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.history = []llm.Message{}
}

// StreamEvent 流式事件（Agent 层）
type StreamEvent struct {
	Type    string
	Content string
	Tool    *ToolExecution
	Error   string
}

// ToolExecution 工具执行信息
type ToolExecution struct {
	Name   string
	Args   map[string]interface{}
	Result string
}

func (b *Brain) SaveMemory(key, value string) error {
	if b.memory == nil {
		return fmt.Errorf("记忆系统未初始化")
	}
	return b.memory.UpdateMemory(key, value)
}
func (b *Brain) SearchMemory(query string) ([]string, error) {
	if b.memory == nil {
		return nil, fmt.Errorf("记忆系统未初始化")
	}
	return b.memory.Search(query, 5)
}

// GetMemoryStore 获取底层记忆存储（用于会话恢复等）
func (b *Brain) GetMemoryStore() *memory.Store { return b.memory }

func (b *Brain) SetModel(model string)     { b.provider.SetModel(model) }
func (b *Brain) GetModel() string           { return b.provider.GetModel() }
func (b *Brain) GetDefaultModel() string    { return b.provider.GetDefaultModel() }
func (b *Brain) ResetModel()                { b.provider.ResetModel() }
func (b *Brain) GetSmallModel() string      { return b.provider.GetSmallModel() }
func (b *Brain) SetSmallModel(model string) { b.provider.SetSmallModel(model) }
func (b *Brain) GetProviderName() string    { return b.provider.GetActiveProviderName() }
func (b *Brain) ListProviders() []string    { return b.provider.ListProviders() }
func (b *Brain) ListTools() []string        { return b.executor.ListTools() }

// GetProviderInfo 获取当前供应商详细信息
func (b *Brain) GetProviderInfo() map[string]string {
	return map[string]string{
		"provider":     b.provider.GetActiveProviderName(),
		"model":        b.provider.GetModel(),
		"defaultModel": b.provider.GetDefaultModel(),
		"smallModel":   b.provider.GetSmallModel(),
		"supportsTools": fmt.Sprintf("%v", b.provider.ActiveSupportsTools()),
	}
}

// EstimateTokens 估算当前历史的 Token 数量
func (b *Brain) EstimateTokens() int {
	total := 0
	for _, msg := range b.getMessages() {
		total += len(msg.Content)/4 + 4
	}
	return total
}

// Complete 快速补全（用小模型，非流式）
func (b *Brain) Complete(input string, recentHistory []llm.Message) (string, error) {
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
	if len(recentHistory) > 4 {
		recentHistory = recentHistory[len(recentHistory)-4:]
	}
	messages = append(messages, recentHistory...)
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: fmt.Sprintf("当前输入: %s", input),
	})

	return b.provider.Complete(messages, 60)
}
