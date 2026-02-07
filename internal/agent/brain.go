package agent

import (
	"fmt"
	"strings"

	"github.com/BlakeLiAFK/kele/internal/cron"
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
func NewBrain(scheduler *cron.Scheduler) *Brain {
	return &Brain{
		llmClient: llm.NewClient(),
		executor:  tools.NewExecutor(scheduler),
		memory:    memory.NewStore(),
		history:   []llm.Message{},
		maxTurns:  20,
	}
}

// Chat 处理对话（非流式，带自动工具调用循环）
func (b *Brain) Chat(userInput string) (string, error) {
	b.addMessage("user", userInput)

	const maxRounds = 10
	var allResults []string

	for round := 0; round < maxRounds; round++ {
		resp, err := b.llmClient.Chat(b.getMessages(), b.executor.GetTools())
		if err != nil {
			return strings.Join(allResults, "\n\n"), err
		}

		if len(resp.Choices) == 0 {
			return strings.Join(allResults, "\n\n"), fmt.Errorf("API returned empty response")
		}

		choice := resp.Choices[0]

		// 有工具调用 -> 执行工具并继续循环
		if len(choice.Message.ToolCalls) > 0 {
			b.appendRawMessage(choice.Message)

			for _, tc := range choice.Message.ToolCalls {
				result, err := b.executor.Execute(tc)
				if err != nil {
					result = fmt.Sprintf("Error: %v", err)
				}
				allResults = append(allResults, fmt.Sprintf("tool %s:\n%s", tc.Function.Name, result))
				b.appendRawMessage(llm.Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
				})
			}
			continue
		}

		// 纯文本响应
		content := choice.Message.Content
		b.addMessage("assistant", content)
		b.memory.SaveMessage("user", userInput)
		b.memory.SaveMessage("assistant", content)
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

		const maxToolRounds = 10
		var finalContent string

		for round := 0; round < maxToolRounds; round++ {
			llmEvents := b.llmClient.ChatStream(b.getMessages(), b.executor.GetTools())

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
					// 纯文本流结束
					if roundContent != "" {
						b.addMessage("assistant", roundContent)
						finalContent = roundContent
					}
					b.memory.SaveMessage("user", userInput)
					if finalContent != "" {
						b.memory.SaveMessage("assistant", finalContent)
					}
					eventChan <- StreamEvent{Type: "done"}
					return
				}
			}

			// 处理工具调用
			if gotToolCalls {
				// 添加带 tool_calls 的助手消息到历史
				assistantMsg := llm.Message{
					Role:      "assistant",
					Content:   roundContent,
					ToolCalls: pendingToolCalls,
				}
				b.appendRawMessage(assistantMsg)

				// 逐个执行工具
				for _, tc := range pendingToolCalls {
					eventChan <- StreamEvent{
						Type: "tool_start",
						Tool: &ToolExecution{Name: tc.Function.Name},
					}

					result, err := b.executor.Execute(tc)
					if err != nil {
						result = fmt.Sprintf("Error: %v", err)
					}

					// 添加工具结果到历史
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
				// 继续下一轮 LLM 调用
				continue
			}

			// 流意外结束（未收到 done 也未收到 tool_calls）
			if roundContent != "" {
				b.addMessage("assistant", roundContent)
				finalContent = roundContent
			}
			b.memory.SaveMessage("user", userInput)
			if finalContent != "" {
				b.memory.SaveMessage("assistant", finalContent)
			}
			eventChan <- StreamEvent{Type: "done"}
			return
		}

		// 达到最大工具轮数
		b.addMessage("assistant", "[reached max tool rounds]")
		eventChan <- StreamEvent{Type: "done"}
	}()

	return eventChan, nil
}

// addMessage 添加简单消息到历史
func (b *Brain) addMessage(role, content string) {
	b.history = append(b.history, llm.Message{
		Role:    role,
		Content: content,
	})
	b.trimHistory()
}

// appendRawMessage 添加完整消息到历史（含 ToolCalls/ToolCallID）
func (b *Brain) appendRawMessage(msg llm.Message) {
	b.history = append(b.history, msg)
	b.trimHistory()
}

// trimHistory 限制历史长度
func (b *Brain) trimHistory() {
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
3. 管理定时任务（cron）：
   - cron_create: 创建定时任务（标准 cron 表达式）
   - cron_list: 列出所有定时任务
   - cron_get: 查看任务详情和执行日志
   - cron_update: 更新任务（含暂停/恢复）
   - cron_delete: 删除任务

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

// StreamEvent 流式事件（Agent 层）
type StreamEvent struct {
	Type    string // content, tool_start, tool_result, error, done
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

// GetSmallModel 获取小模型名称
func (b *Brain) GetSmallModel() string {
	return b.llmClient.GetSmallModel()
}

// SetSmallModel 设置小模型
func (b *Brain) SetSmallModel(model string) {
	b.llmClient.SetSmallModel(model)
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

	return b.llmClient.Complete(messages, 60)
}
