package agent

import (
	"fmt"
	"strings"

	"github.com/BlakeLiAFK/kele/internal/llm"
	"github.com/BlakeLiAFK/kele/internal/memory"
	"github.com/BlakeLiAFK/kele/internal/tools"
)

// Brain AI 大脑
type Brain struct {
	llmClient *llm.Client
	executor  *tools.Executor
	memory    *memory.Store
	history   []llm.Message
	maxTurns  int
}

// NewBrain 创建新大脑
func NewBrain() *Brain {
	return &Brain{
		llmClient: llm.NewClient(),
		executor:  tools.NewExecutor(),
		memory:    memory.NewStore(),
		history:   []llm.Message{},
		maxTurns:  20, // 保留最近 20 轮对话
	}
}

// Chat 处理对话（非流式）
func (b *Brain) Chat(userInput string) (string, error) {
	// 添加用户消息
	b.addMessage("user", userInput)

	// 调用 LLM
	resp, err := b.llmClient.Chat(b.getMessages(), b.executor.GetTools())
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("API 返回空响应")
	}

	choice := resp.Choices[0]

	// 处理工具调用
	if len(choice.ToolCalls) > 0 {
		return b.handleToolCalls(choice.ToolCalls)
	}

	// 添加助手响应
	b.addMessage("assistant", choice.Message.Content)

	// 保存到内存
	b.memory.SaveMessage("user", userInput)
	b.memory.SaveMessage("assistant", choice.Message.Content)

	return choice.Message.Content, nil
}

// ChatStream 流式对话
func (b *Brain) ChatStream(userInput string) (<-chan StreamEvent, error) {
	eventChan := make(chan StreamEvent, 100)

	go func() {
		defer close(eventChan)

		// 添加用户消息
		b.addMessage("user", userInput)

		// 获取流式响应
		contentChan, errChan := b.llmClient.ChatStream(b.getMessages(), b.executor.GetTools())

		fullContent := ""
		for {
			select {
			case content, ok := <-contentChan:
				if !ok {
					// 流结束
					b.addMessage("assistant", fullContent)
					b.memory.SaveMessage("user", userInput)
					b.memory.SaveMessage("assistant", fullContent)
					return
				}
				fullContent += content
				eventChan <- StreamEvent{
					Type:    "content",
					Content: content,
				}

			case err := <-errChan:
				if err != nil {
					eventChan <- StreamEvent{
						Type:  "error",
						Error: err.Error(),
					}
					return
				}
			}
		}
	}()

	return eventChan, nil
}

// handleToolCalls 处理工具调用
func (b *Brain) handleToolCalls(toolCalls []llm.ToolCall) (string, error) {
	var results []string

	for _, tc := range toolCalls {
		// 执行工具
		result, err := b.executor.Execute(tc)
		if err != nil {
			result = fmt.Sprintf("错误: %v", err)
		}

		results = append(results, fmt.Sprintf("🔧 %s:\n%s", tc.Function.Name, result))

		// 添加工具调用到历史
		b.addMessage("assistant", fmt.Sprintf("使用工具: %s", tc.Function.Name))
		b.addMessage("tool", result)
	}

	// 再次调用 LLM 获取最终响应
	resp, err := b.llmClient.Chat(b.getMessages(), nil)
	if err != nil {
		return strings.Join(results, "\n\n"), nil
	}

	if len(resp.Choices) > 0 {
		finalResponse := resp.Choices[0].Message.Content
		b.addMessage("assistant", finalResponse)

		// 保存到内存
		b.memory.SaveMessage("assistant", finalResponse)

		return strings.Join(results, "\n\n") + "\n\n" + finalResponse, nil
	}

	return strings.Join(results, "\n\n"), nil
}

// addMessage 添加消息到历史
func (b *Brain) addMessage(role, content string) {
	b.history = append(b.history, llm.Message{
		Role:    role,
		Content: content,
	})

	// 限制历史长度
	if len(b.history) > b.maxTurns*2 {
		b.history = b.history[len(b.history)-b.maxTurns*2:]
	}
}

// getMessages 获取对话历史（添加系统提示）
func (b *Brain) getMessages() []llm.Message {
	systemPrompt := llm.Message{
		Role: "system",
		Content: `你是 Kele，一个智能的终端 AI 助手。你可以：
1. 回答问题和进行对话
2. 使用工具执行操作：
   - bash: 执行命令
   - read: 读取文件
   - write: 创建或修改文件

请用中文回答，保持简洁专业。当需要执行操作时，主动使用工具。`,
	}

	messages := []llm.Message{systemPrompt}
	messages = append(messages, b.history...)
	return messages
}

// GetHistory 获取历史记录
func (b *Brain) GetHistory() []llm.Message {
	return b.history
}

// ClearHistory 清空历史
func (b *Brain) ClearHistory() {
	b.history = []llm.Message{}
}

// StreamEvent 流式事件
type StreamEvent struct {
	Type    string // content, tool, error, done
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

// SaveMemory 保存记忆
func (b *Brain) SaveMemory(key, value string) error {
	return b.memory.UpdateMemory(key, value)
}

// SearchMemory 搜索记忆
func (b *Brain) SearchMemory(query string) ([]string, error) {
	return b.memory.Search(query, 5)
}

// SetModel 设置模型
func (b *Brain) SetModel(model string) {
	b.llmClient.SetModel(model)
}

// GetModel 获取当前模型
func (b *Brain) GetModel() string {
	return b.llmClient.GetModel()
}

// GetDefaultModel 获取默认模型
func (b *Brain) GetDefaultModel() string {
	return b.llmClient.GetDefaultModel()
}

// ResetModel 重置为默认模型
func (b *Brain) ResetModel() {
	b.llmClient.ResetModel()
}
