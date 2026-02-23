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

// OpenAIProvider OpenAI 兼容供应商
type OpenAIProvider struct {
	name    string // 自定义名称，空则返回 "openai"
	apiBase string
	apiKey  string
	client  *http.Client
}

// NewOpenAIProvider 创建 OpenAI 供应商
func NewOpenAIProvider(cfg *config.Config) *OpenAIProvider {
	return &OpenAIProvider{
		apiBase: cfg.LLM.OpenAIAPIBase,
		apiKey:  cfg.LLM.OpenAIAPIKey,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// NewOpenAIProviderDirect 直接创建指定名称的 OpenAI 兼容供应商
func NewOpenAIProviderDirect(name, apiBase, apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		name:    name,
		apiBase: apiBase,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

func (p *OpenAIProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	return "openai"
}

func (p *OpenAIProvider) SupportsTools() bool { return true }

// Chat 非流式聊天
func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message, tools []Tool, opts ChatOptions) (*ChatResponse, error) {
	req := ChatRequest{
		Model:       opts.Model,
		Messages:    messages,
		Stream:      false,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		Tools:       tools,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("网络错误: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, classifyAPIError(resp.StatusCode, string(bodyBytes))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &chatResp, nil
}

// ChatStream 流式聊天
func (p *OpenAIProvider) ChatStream(ctx context.Context, messages []Message, tools []Tool, opts ChatOptions) (<-chan StreamEvent, error) {
	req := ChatRequest{
		Model:       opts.Model,
		Messages:    messages,
		Stream:      true,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		Tools:       tools,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

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
	go p.readStream(resp, eventChan)
	return eventChan, nil
}

func (p *OpenAIProvider) readStream(resp *http.Response, eventChan chan<- StreamEvent) {
	defer close(eventChan)
	defer resp.Body.Close()

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
				if len(toolCalls) > 0 {
					finalizeToolCalls(toolCalls, toolCallArgs)
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
					finalizeToolCalls(toolCalls, toolCallArgs)
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

		if delta.ReasoningContent != "" {
			eventChan <- StreamEvent{Type: "reasoning", ReasoningContent: delta.ReasoningContent}
		}

		if delta.Content != "" {
			eventChan <- StreamEvent{Type: "content", Content: delta.Content}
		}

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

		if chunk.Choices[0].FinishReason != nil {
			reason := *chunk.Choices[0].FinishReason
			terminated = true
			if reason == "tool_calls" {
				finalizeToolCalls(toolCalls, toolCallArgs)
				eventChan <- StreamEvent{Type: "tool_calls", ToolCalls: toolCalls}
			} else {
				eventChan <- StreamEvent{Type: "done"}
			}
			return
		}
	}
}

// finalizeToolCalls 将累积的参数写入 ToolCall
func finalizeToolCalls(toolCalls []ToolCall, args map[int]*strings.Builder) {
	for idx, builder := range args {
		if idx < len(toolCalls) {
			toolCalls[idx].Function.Arguments = builder.String()
		}
	}
}

// classifyAPIError 分类 API 错误
func classifyAPIError(statusCode int, body string) error {
	switch statusCode {
	case 401:
		return fmt.Errorf("认证失败: API Key 无效或已过期。请检查环境变量设置")
	case 403:
		return fmt.Errorf("权限不足: 无权访问该模型或 API。%s", truncateError(body))
	case 404:
		return fmt.Errorf("模型不存在: 请检查模型名称是否正确。%s", truncateError(body))
	case 429:
		return fmt.Errorf("请求频率超限: 请稍后重试。%s", truncateError(body))
	case 500, 502, 503:
		return fmt.Errorf("服务暂时不可用 (HTTP %d): 请稍后重试", statusCode)
	default:
		return fmt.Errorf("API 错误 (HTTP %d): %s", statusCode, truncateError(body))
	}
}

func truncateError(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
