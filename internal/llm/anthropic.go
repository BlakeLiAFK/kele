package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/BlakeLiAFK/kele/internal/config"
)

// AnthropicProvider Anthropic Claude 供应商
type AnthropicProvider struct {
	apiBase string
	apiKey  string
	client  *http.Client
}

// NewAnthropicProvider 创建 Anthropic 供应商
func NewAnthropicProvider(cfg *config.Config) *AnthropicProvider {
	return &AnthropicProvider{
		apiBase: cfg.LLM.AnthropicAPIBase,
		apiKey:  cfg.LLM.AnthropicAPIKey,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (p *AnthropicProvider) Name() string        { return "anthropic" }
func (p *AnthropicProvider) SupportsTools() bool  { return true }

// --- Anthropic API 请求/响应类型 ---

type anthropicRequest struct {
	Model     string              `json:"model"`
	MaxTokens int                 `json:"max_tokens"`
	System    string              `json:"system,omitempty"`
	Messages  []anthropicMessage  `json:"messages"`
	Tools     []anthropicTool     `json:"tools,omitempty"`
	Stream    bool                `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string 或 []anthropicContentBlock
}

type anthropicContentBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Input     interface{} `json:"input,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type anthropicResponse struct {
	ID      string                  `json:"id"`
	Type    string                  `json:"type"`
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
	Model   string                  `json:"model"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	StopReason string `json:"stop_reason"`
}

type anthropicStreamEvent struct {
	Type  string          `json:"type"`
	Index int             `json:"index,omitempty"`
	Delta json.RawMessage `json:"delta,omitempty"`
	ContentBlock *anthropicContentBlock `json:"content_block,omitempty"`
}

// Chat 非流式聊天
func (p *AnthropicProvider) Chat(ctx context.Context, messages []Message, tools []Tool, opts ChatOptions) (*ChatResponse, error) {
	system, anthropicMsgs := convertToAnthropic(messages)
	anthropicTools := convertToolsToAnthropic(tools)

	req := anthropicRequest{
		Model:     opts.Model,
		MaxTokens: opts.MaxTokens,
		System:    system,
		Messages:  anthropicMsgs,
		Tools:     anthropicTools,
		Stream:    false,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("网络错误: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, classifyAPIError(resp.StatusCode, string(bodyBytes))
	}

	var anthropicResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return convertFromAnthropic(&anthropicResp), nil
}

// ChatStream 流式聊天
func (p *AnthropicProvider) ChatStream(ctx context.Context, messages []Message, tools []Tool, opts ChatOptions) (<-chan StreamEvent, error) {
	system, anthropicMsgs := convertToAnthropic(messages)
	anthropicTools := convertToolsToAnthropic(tools)

	req := anthropicRequest{
		Model:     opts.Model,
		MaxTokens: opts.MaxTokens,
		System:    system,
		Messages:  anthropicMsgs,
		Tools:     anthropicTools,
		Stream:    true,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("网络错误: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, classifyAPIError(resp.StatusCode, string(bodyBytes))
	}

	eventChan := make(chan StreamEvent, 100)
	go p.readAnthropicStream(resp, eventChan)
	return eventChan, nil
}

func (p *AnthropicProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
}

func (p *AnthropicProvider) readAnthropicStream(resp *http.Response, eventChan chan<- StreamEvent) {
	defer close(eventChan)
	defer resp.Body.Close()

	var currentToolCalls []ToolCall
	var currentToolInput strings.Builder
	currentToolIdx := -1

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				eventChan <- StreamEvent{Type: "error", Error: err}
			}
			// 如果有待处理的工具调用
			if len(currentToolCalls) > 0 {
				if currentToolIdx >= 0 && currentToolIdx < len(currentToolCalls) {
					currentToolCalls[currentToolIdx].Function.Arguments = currentToolInput.String()
				}
				eventChan <- StreamEvent{Type: "tool_calls", ToolCalls: currentToolCalls}
			} else {
				eventChan <- StreamEvent{Type: "done"}
			}
			return
		}

		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_start":
			if event.ContentBlock != nil {
				switch event.ContentBlock.Type {
				case "tool_use":
					tc := ToolCall{
						ID:   event.ContentBlock.ID,
						Type: "function",
					}
					tc.Function.Name = event.ContentBlock.Name
					currentToolCalls = append(currentToolCalls, tc)
					currentToolIdx = len(currentToolCalls) - 1
					currentToolInput.Reset()
				case "thinking":
					// Thinking block started
				}
			}

		case "content_block_delta":
			if event.Delta != nil {
				var delta struct {
					Type         string `json:"type"`
					Text         string `json:"text"`
					PartialJSON  string `json:"partial_json"`
					Thinking     string `json:"thinking"`
				}
				json.Unmarshal(event.Delta, &delta)

				switch delta.Type {
				case "text_delta":
					if delta.Text != "" {
						eventChan <- StreamEvent{Type: "content", Content: delta.Text}
					}
				case "input_json_delta":
					if delta.PartialJSON != "" {
						currentToolInput.WriteString(delta.PartialJSON)
					}
				case "thinking_delta":
					if delta.Thinking != "" {
						eventChan <- StreamEvent{Type: "reasoning", ReasoningContent: delta.Thinking}
					}
				}
			}

		case "content_block_stop":
			// 如果当前块是工具调用，完成参数累积
			if currentToolIdx >= 0 && currentToolIdx < len(currentToolCalls) {
				currentToolCalls[currentToolIdx].Function.Arguments = currentToolInput.String()
				currentToolInput.Reset()
				currentToolIdx = -1
			}

		case "message_stop":
			if len(currentToolCalls) > 0 {
				eventChan <- StreamEvent{Type: "tool_calls", ToolCalls: currentToolCalls}
			} else {
				eventChan <- StreamEvent{Type: "done"}
			}
			return

		case "message_delta":
			// 消息级 delta（包含 stop_reason）
			if event.Delta != nil {
				var delta struct {
					StopReason string `json:"stop_reason"`
				}
				json.Unmarshal(event.Delta, &delta)
				if delta.StopReason == "tool_use" && len(currentToolCalls) > 0 {
					eventChan <- StreamEvent{Type: "tool_calls", ToolCalls: currentToolCalls}
					return
				}
			}

		case "error":
			var errData struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			json.Unmarshal(event.Delta, &errData)
			eventChan <- StreamEvent{Type: "error", Error: fmt.Errorf("Anthropic 错误: %s", errData.Error.Message)}
			return
		}
	}
}

// --- 格式转换 ---

// convertToAnthropic 将内部消息格式转换为 Anthropic 格式
func convertToAnthropic(messages []Message) (string, []anthropicMessage) {
	var system string
	var result []anthropicMessage

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			system = msg.Content
		case "user":
			result = append(result, anthropicMessage{
				Role:    "user",
				Content: msg.Content,
			})
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				// 带工具调用的助手消息
				var blocks []anthropicContentBlock
				if msg.Content != "" {
					blocks = append(blocks, anthropicContentBlock{
						Type: "text",
						Text: msg.Content,
					})
				}
				for _, tc := range msg.ToolCalls {
					var input interface{}
					json.Unmarshal([]byte(tc.Function.Arguments), &input)
					if input == nil {
						input = map[string]interface{}{}
					}
					blocks = append(blocks, anthropicContentBlock{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Input: input,
					})
				}
				result = append(result, anthropicMessage{
					Role:    "assistant",
					Content: blocks,
				})
			} else {
				result = append(result, anthropicMessage{
					Role:    "assistant",
					Content: msg.Content,
				})
			}
		case "tool":
			// 工具结果
			blocks := []anthropicContentBlock{
				{
					Type:      "tool_result",
					ToolUseID: msg.ToolCallID,
					Content:   msg.Content,
				},
			}
			result = append(result, anthropicMessage{
				Role:    "user",
				Content: blocks,
			})
		}
	}

	return system, result
}

// convertToolsToAnthropic 转换工具定义
func convertToolsToAnthropic(tools []Tool) []anthropicTool {
	if len(tools) == 0 {
		return nil
	}
	var result []anthropicTool
	for _, t := range tools {
		result = append(result, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}
	return result
}

// convertFromAnthropic 将 Anthropic 响应转换为内部格式
func convertFromAnthropic(resp *anthropicResponse) *ChatResponse {
	var content string
	var toolCalls []ToolCall

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			content += block.Text
		case "tool_use":
			inputBytes, _ := json.Marshal(block.Input)
			tc := ToolCall{
				ID:   block.ID,
				Type: "function",
			}
			tc.Function.Name = block.Name
			tc.Function.Arguments = string(inputBytes)
			toolCalls = append(toolCalls, tc)
		}
	}

	finishReason := resp.StopReason
	if finishReason == "end_turn" {
		finishReason = "stop"
	} else if finishReason == "tool_use" {
		finishReason = "tool_calls"
	}

	return &ChatResponse{
		ID:    resp.ID,
		Model: resp.Model,
		Choices: []ChatChoice{
			{
				Index: 0,
				Message: Message{
					Role:      "assistant",
					Content:   content,
					ToolCalls: toolCalls,
				},
				FinishReason: finishReason,
			},
		},
		Usage: ChatUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}
