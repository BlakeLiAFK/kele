package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Client LLM 客户端
type Client struct {
	apiBase      string
	apiKey       string
	model        string
	defaultModel string
	smallModel   string
	client       *http.Client
}

// NewClient 创建新客户端
func NewClient() *Client {
	apiBase := os.Getenv("OPENAI_API_BASE")
	if apiBase == "" {
		apiBase = "https://api.openai.com/v1"
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		panic("OPENAI_API_KEY environment variable is required")
	}

	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o"
	}

	smallModel := os.Getenv("KELE_SMALL_MODEL")

	return &Client{
		apiBase:      apiBase,
		apiKey:       apiKey,
		model:        model,
		defaultModel: model,
		smallModel:   smallModel,
		client:       &http.Client{},
	}
}

// SetModel 设置模型
func (c *Client) SetModel(model string) {
	c.model = model
}

// GetModel 获取当前模型
func (c *Client) GetModel() string {
	return c.model
}

// GetDefaultModel 获取默认模型
func (c *Client) GetDefaultModel() string {
	return c.defaultModel
}

// ResetModel 重置为默认模型
func (c *Client) ResetModel() {
	c.model = c.defaultModel
}

// GetSmallModel 获取小模型名称（回落到主模型）
func (c *Client) GetSmallModel() string {
	if c.smallModel != "" {
		return c.smallModel
	}
	return c.model
}

// SetSmallModel 设置小模型
func (c *Client) SetSmallModel(model string) {
	c.smallModel = model
}

// Complete 非流式快速补全（使用小模型）
func (c *Client) Complete(messages []Message, maxTokens int) (string, error) {
	useModel := c.GetSmallModel()

	req := ChatRequest{
		Model:       useModel,
		Messages:    messages,
		Stream:      false,
		Temperature: 0.3,
		MaxTokens:   maxTokens,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequest("POST", c.apiBase+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error: %s - %s", resp.Status, string(bodyBytes))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) == 0 {
		return "", nil
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

// Chat 发送聊天请求（非流式）
func (c *Client) Chat(messages []Message, tools []Tool) (*ChatResponse, error) {
	req := ChatRequest{
		Model:       c.model,
		Messages:    messages,
		Stream:      false,
		Temperature: 0.7,
		MaxTokens:   4096,
		Tools:       tools,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", c.apiBase+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(bodyBytes))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, err
	}

	return &chatResp, nil
}

// ChatStream 流式聊天（支持工具调用累积）
func (c *Client) ChatStream(messages []Message, tools []Tool) <-chan StreamEvent {
	eventChan := make(chan StreamEvent, 100)

	go func() {
		defer close(eventChan)

		req := ChatRequest{
			Model:       c.model,
			Messages:    messages,
			Stream:      true,
			Temperature: 0.7,
			MaxTokens:   4096,
			Tools:       tools,
		}

		body, err := json.Marshal(req)
		if err != nil {
			eventChan <- StreamEvent{Type: "error", Error: err}
			return
		}

		httpReq, err := http.NewRequest("POST", c.apiBase+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			eventChan <- StreamEvent{Type: "error", Error: err}
			return
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.client.Do(httpReq)
		if err != nil {
			eventChan <- StreamEvent{Type: "error", Error: err}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			eventChan <- StreamEvent{Type: "error", Error: fmt.Errorf("API error: %s - %s", resp.Status, string(bodyBytes))}
			return
		}

		// 工具调用累积器
		var toolCalls []ToolCall
		toolCallArgs := make(map[int]*strings.Builder)
		terminated := false

		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					eventChan <- StreamEvent{Type: "error", Error: err}
				} else if !terminated {
					// EOF 兜底
					if len(toolCalls) > 0 {
						c.finalizeToolCalls(toolCalls, toolCallArgs)
						eventChan <- StreamEvent{Type: "tool_calls", ToolCalls: toolCalls}
					} else {
						eventChan <- StreamEvent{Type: "done"}
					}
				}
				return
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			if line == "data: [DONE]" {
				if !terminated {
					if len(toolCalls) > 0 {
						c.finalizeToolCalls(toolCalls, toolCallArgs)
						eventChan <- StreamEvent{Type: "tool_calls", ToolCalls: toolCalls}
					} else {
						eventChan <- StreamEvent{Type: "done"}
					}
				}
				return
			}

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			var chunk StreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta

			// 推理内容（DeepSeek 等模型）
			if delta.ReasoningContent != "" {
				eventChan <- StreamEvent{Type: "reasoning", ReasoningContent: delta.ReasoningContent}
			}

			// 文本内容
			if delta.Content != "" {
				eventChan <- StreamEvent{Type: "content", Content: delta.Content}
			}

			// 工具调用（流式累积）
			for _, tc := range delta.ToolCalls {
				idx := tc.Index
				for len(toolCalls) <= idx {
					toolCalls = append(toolCalls, ToolCall{})
				}
				if tc.ID != "" {
					toolCalls[idx].ID = tc.ID
					toolCalls[idx].Type = tc.Type
					toolCalls[idx].Function.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					if _, ok := toolCallArgs[idx]; !ok {
						toolCallArgs[idx] = &strings.Builder{}
					}
					toolCallArgs[idx].WriteString(tc.Function.Arguments)
				}
			}

			// 检查结束原因
			if chunk.Choices[0].FinishReason != nil {
				reason := *chunk.Choices[0].FinishReason
				terminated = true
				if reason == "tool_calls" {
					c.finalizeToolCalls(toolCalls, toolCallArgs)
					eventChan <- StreamEvent{Type: "tool_calls", ToolCalls: toolCalls}
				} else {
					eventChan <- StreamEvent{Type: "done"}
				}
				return
			}
		}
	}()

	return eventChan
}

// finalizeToolCalls 将累积的参数写入 ToolCall
func (c *Client) finalizeToolCalls(toolCalls []ToolCall, args map[int]*strings.Builder) {
	for idx, builder := range args {
		if idx < len(toolCalls) {
			toolCalls[idx].Function.Arguments = builder.String()
		}
	}
}
