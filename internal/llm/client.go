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

	return &Client{
		apiBase:      apiBase,
		apiKey:       apiKey,
		model:        model,
		defaultModel: model,
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

// Chat 发送聊天请求
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

// ChatStream 流式聊天
func (c *Client) ChatStream(messages []Message, tools []Tool) (<-chan string, <-chan error) {
	contentChan := make(chan string, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(contentChan)
		defer close(errChan)

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
			errChan <- err
			return
		}

		httpReq, err := http.NewRequest("POST", c.apiBase+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			errChan <- err
			return
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.client.Do(httpReq)
		if err != nil {
			errChan <- err
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			errChan <- fmt.Errorf("API error: %s - %s", resp.Status, string(bodyBytes))
			return
		}

		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					errChan <- err
				}
				return
			}

			line = strings.TrimSpace(line)
			if line == "" || line == "data: [DONE]" {
				continue
			}

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			var chunk StreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) > 0 {
				content := chunk.Choices[0].Delta.Content
				if content != "" {
					contentChan <- content
				}
			}
		}
	}()

	return contentChan, errChan
}
