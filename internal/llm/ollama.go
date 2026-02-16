package llm

import (
	"context"
	"fmt"
	"time"
	"net/http"

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
