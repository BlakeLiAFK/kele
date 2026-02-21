package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/BlakeLiAFK/kele/internal/config"
)

// OllamaProvider Ollama 本地模型供应商（使用 OpenAI 兼容接口）
type OllamaProvider struct {
	host   string
	client *http.Client
	openai *OpenAIProvider // 内部复用 OpenAI 兼容逻辑
}

// NewOllamaProvider 创建 Ollama 供应商
func NewOllamaProvider(cfg *config.Config) *OllamaProvider {
	host := cfg.LLM.OllamaHost
	return &OllamaProvider{
		host: host,
		client: &http.Client{
			Timeout: 10 * time.Minute, // Ollama 本地推理可能较慢
		},
		openai: &OpenAIProvider{
			apiBase: host + "/v1",
			apiKey:  "ollama", // Ollama 不需要真实 key
			client: &http.Client{
				Timeout: 10 * time.Minute,
			},
		},
	}
}

func (p *OllamaProvider) Name() string { return "ollama" }

func (p *OllamaProvider) SupportsTools() bool { return true }

// Chat 非流式聊天（通过 OpenAI 兼容接口）
func (p *OllamaProvider) Chat(ctx context.Context, messages []Message, tools []Tool, opts ChatOptions) (*ChatResponse, error) {
	resp, err := p.openai.Chat(ctx, messages, tools, opts)
	if err != nil {
		return nil, fmt.Errorf("Ollama 错误: %w (确认 Ollama 已运行: %s)", err, p.host)
	}
	return resp, nil
}

// ChatStream 流式聊天
func (p *OllamaProvider) ChatStream(ctx context.Context, messages []Message, tools []Tool, opts ChatOptions) (<-chan StreamEvent, error) {
	ch, err := p.openai.ChatStream(ctx, messages, tools, opts)
	if err != nil {
		return nil, fmt.Errorf("Ollama 错误: %w (确认 Ollama 已运行: %s)", err, p.host)
	}
	return ch, nil
}

// ListModels 查询 Ollama 本地已安装的模型
func (p *OllamaProvider) ListModels() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", p.host+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接 Ollama (%s): %w", p.host, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	var names []string
	for _, m := range result.Models {
		names = append(names, m.Name)
	}
	return names, nil
}
