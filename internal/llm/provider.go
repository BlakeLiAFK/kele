package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/BlakeLiAFK/kele/internal/config"
)

// Provider LLM 供应商接口
type Provider interface {
	Name() string
	Chat(ctx context.Context, messages []Message, tools []Tool, opts ChatOptions) (*ChatResponse, error)
	ChatStream(ctx context.Context, messages []Message, tools []Tool, opts ChatOptions) (<-chan StreamEvent, error)
	SupportsTools() bool
}

// ChatOptions 聊天参数
type ChatOptions struct {
	Model       string
	Temperature float64
	MaxTokens   int
}

// ProviderManager 多供应商管理器
type ProviderManager struct {
	providers map[string]Provider
	mu        sync.RWMutex

	// 当前状态
	activeProvider     Provider
	activeProviderName string // 显式锁定的供应商名称
	explicitProvider   bool   // 是否由 /provider use 锁定
	model              string
	defaultModel       string
	smallModel         string
	smallProvider      Provider

	cfg *config.Config
}

// NewProviderManager 创建供应商管理器
func NewProviderManager(cfg *config.Config) *ProviderManager {
	pm := &ProviderManager{
		providers:    make(map[string]Provider),
		model:        cfg.LLM.OpenAIModel,
		defaultModel: cfg.LLM.OpenAIModel,
		smallModel:   cfg.LLM.SmallModel,
		cfg:          cfg,
	}

	// 注册可用的供应商
	if cfg.HasOpenAI() {
		openai := NewOpenAIProvider(cfg)
		pm.providers["openai"] = openai
		pm.activeProvider = openai
	}

	if cfg.HasAnthropic() {
		anthropic := NewAnthropicProvider(cfg)
		pm.providers["anthropic"] = anthropic
		// 如果没有 OpenAI，默认用 Anthropic
		if pm.activeProvider == nil {
			pm.activeProvider = anthropic
			if pm.model == "gpt-4o" {
				pm.model = "claude-sonnet-4-5-20250929"
				pm.defaultModel = pm.model
			}
		}
	}

	// Ollama 始终注册（本地服务，可能可用）
	ollama := NewOllamaProvider(cfg)
	pm.providers["ollama"] = ollama
	if pm.activeProvider == nil {
		pm.activeProvider = ollama
		if pm.model == "gpt-4o" {
			pm.model = "llama3:8b"
			pm.defaultModel = pm.model
		}
	}

	// 设置 small provider
	pm.smallProvider = pm.resolveProvider(pm.smallModel)

	// 加载自定义供应商
	pm.loadCustomProviders()

	// 恢复上次活跃供应商
	if name, err := config.GetValue("llm.active_provider"); err == nil && name != "" {
		if p, ok := pm.providers[name]; ok {
			pm.activeProvider = p
			pm.activeProviderName = name
			pm.explicitProvider = true
			// 恢复对应的默认模型
			if model, err := config.GetValue("llm.active_model"); err == nil && model != "" {
				pm.model = model
			}
		}
	}

	return pm
}

// loadCustomProviders 从 DB 加载自定义供应商并注册
func (pm *ProviderManager) loadCustomProviders() {
	profiles, err := config.ListProviderProfiles()
	if err != nil {
		return
	}
	for _, p := range profiles {
		var provider Provider
		switch p.Type {
		case "anthropic":
			provider = NewAnthropicProviderDirect(p.Name, p.APIBase, p.APIKey)
		default:
			// openai 及其他兼容类型
			provider = NewOpenAIProviderDirect(p.Name, p.APIBase, p.APIKey)
		}
		pm.providers[p.Name] = provider
	}
}

// Chat 非流式聊天（带自动重试）
func (pm *ProviderManager) Chat(messages []Message, tools []Tool) (*ChatResponse, error) {
	pm.mu.RLock()
	provider := pm.activeProvider
	model := pm.model
	pm.mu.RUnlock()

	if provider == nil {
		return nil, fmt.Errorf("未配置任何 LLM 供应商，请设置 OPENAI_API_KEY 或 ANTHROPIC_API_KEY")
	}

	opts := ChatOptions{
		Model:       model,
		Temperature: pm.cfg.LLM.Temperature,
		MaxTokens:   pm.cfg.LLM.MaxTokens,
	}

	// 自动重试：最多 3 次，指数退避
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		resp, err := provider.Chat(context.Background(), messages, tools, opts)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isRetryableError(err) {
			return nil, err
		}
		// 指数退避：1s, 2s, 4s
		backoff := time.Duration(1<<uint(attempt)) * time.Second
		time.Sleep(backoff)
	}
	return nil, fmt.Errorf("重试 3 次后仍失败: %w", lastErr)
}

// ChatStream 流式聊天（带自动重试）
func (pm *ProviderManager) ChatStream(messages []Message, tools []Tool) <-chan StreamEvent {
	pm.mu.RLock()
	provider := pm.activeProvider
	model := pm.model
	pm.mu.RUnlock()

	if provider == nil {
		ch := make(chan StreamEvent, 1)
		ch <- StreamEvent{Type: "error", Error: fmt.Errorf("未配置任何 LLM 供应商")}
		close(ch)
		return ch
	}

	opts := ChatOptions{
		Model:       model,
		Temperature: pm.cfg.LLM.Temperature,
		MaxTokens:   pm.cfg.LLM.MaxTokens,
	}

	// 自动重试
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		ch, err := provider.ChatStream(context.Background(), messages, tools, opts)
		if err == nil {
			return ch
		}
		lastErr = err
		if !isRetryableError(err) {
			errCh := make(chan StreamEvent, 1)
			errCh <- StreamEvent{Type: "error", Error: err}
			close(errCh)
			return errCh
		}
		backoff := time.Duration(1<<uint(attempt)) * time.Second
		time.Sleep(backoff)
	}

	errCh := make(chan StreamEvent, 1)
	errCh <- StreamEvent{Type: "error", Error: fmt.Errorf("重试 3 次后仍失败: %w", lastErr)}
	close(errCh)
	return errCh
}

// Complete 快速补全（使用小模型）
func (pm *ProviderManager) Complete(messages []Message, maxTokens int) (string, error) {
	pm.mu.RLock()
	provider := pm.smallProvider
	model := pm.GetSmallModel()
	pm.mu.RUnlock()

	if provider == nil {
		provider = pm.activeProvider
		model = pm.model
	}

	if provider == nil {
		return "", fmt.Errorf("无可用供应商")
	}

	resp, err := provider.Chat(context.Background(), messages, nil, ChatOptions{
		Model:       model,
		Temperature: 0.3,
		MaxTokens:   maxTokens,
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", nil
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

// SetModel 设置模型（锁定供应商时只换模型，否则自动切换供应商）
func (pm *ProviderManager) SetModel(model string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.model = model
	if !pm.explicitProvider {
		pm.activeProvider = pm.resolveProvider(model)
	}
}

// GetModel 获取当前模型
func (pm *ProviderManager) GetModel() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.model
}

// GetDefaultModel 获取默认模型
func (pm *ProviderManager) GetDefaultModel() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.defaultModel
}

// ResetModel 重置为默认模型，同时解除供应商锁定
func (pm *ProviderManager) ResetModel() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.model = pm.defaultModel
	pm.explicitProvider = false
	pm.activeProviderName = ""
	// 先回落到内置供应商再 resolve，避免 resolveProvider 回落到自定义供应商
	for _, name := range []string{"openai", "anthropic", "ollama"} {
		if p, ok := pm.providers[name]; ok {
			pm.activeProvider = p
			break
		}
	}
	pm.activeProvider = pm.resolveProvider(pm.model)
	// 清除持久化
	config.DeleteValue("llm.active_provider")
	config.DeleteValue("llm.active_model")
}

// GetSmallModel 获取小模型名称
func (pm *ProviderManager) GetSmallModel() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if pm.smallModel != "" {
		return pm.smallModel
	}
	return pm.model
}

// SetSmallModel 设置小模型
func (pm *ProviderManager) SetSmallModel(model string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.smallModel = model
	pm.smallProvider = pm.resolveProvider(model)
}

// GetActiveProviderName 获取当前供应商名称
func (pm *ProviderManager) GetActiveProviderName() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if pm.activeProviderName != "" {
		return pm.activeProviderName
	}
	if pm.activeProvider != nil {
		return pm.activeProvider.Name()
	}
	return "none"
}

// UseProvider 切换并锁定到指定供应商
func (pm *ProviderManager) UseProvider(name, model string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	p, ok := pm.providers[name]
	if !ok {
		return fmt.Errorf("供应商不存在: %s", name)
	}

	pm.activeProvider = p
	pm.activeProviderName = name
	pm.explicitProvider = true

	if model != "" {
		pm.model = model
	} else {
		// 尝试从 DB 获取该供应商的默认模型
		if profile, err := config.GetProvider(name); err == nil && profile.DefaultModel != "" {
			pm.model = profile.DefaultModel
		}
	}

	// 持久化
	config.SetValue("llm.active_provider", name)
	config.SetValue("llm.active_model", pm.model)
	return nil
}

// RegisterProvider 注册新供应商到管理器
func (pm *ProviderManager) RegisterProvider(name string, p Provider) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.providers[name] = p
}

// RemoveProvider 从管理器移除供应商
func (pm *ProviderManager) RemoveProvider(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, ok := pm.providers[name]; !ok {
		return fmt.Errorf("供应商不存在: %s", name)
	}
	// 不可移除正在使用的供应商
	if pm.activeProviderName == name {
		return fmt.Errorf("不能移除当前活跃供应商: %s", name)
	}
	delete(pm.providers, name)
	return nil
}

// IsExplicitProvider 是否处于供应商锁定模式
func (pm *ProviderManager) IsExplicitProvider() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.explicitProvider
}

// ActiveSupportsTools 当前活跃供应商是否支持工具调用
func (pm *ProviderManager) ActiveSupportsTools() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if pm.activeProvider != nil {
		return pm.activeProvider.SupportsTools()
	}
	return false
}

// ListProviders 列出所有已注册供应商
func (pm *ProviderManager) ListProviders() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	var names []string
	for name := range pm.providers {
		names = append(names, name)
	}
	return names
}

// resolveProvider 根据模型名推断供应商（内部调用，不加锁）
func (pm *ProviderManager) resolveProvider(model string) Provider {
	if model == "" {
		return pm.activeProvider
	}
	lower := strings.ToLower(model)

	// Claude 模型 → Anthropic
	if strings.HasPrefix(lower, "claude") {
		if p, ok := pm.providers["anthropic"]; ok {
			return p
		}
	}

	// 包含 : 的模型名 → Ollama（如 llama3:8b）
	if strings.Contains(model, ":") {
		if p, ok := pm.providers["ollama"]; ok {
			return p
		}
	}

	// GPT / o1 / o3 → OpenAI
	if strings.HasPrefix(lower, "gpt") || strings.HasPrefix(lower, "o1") || strings.HasPrefix(lower, "o3") {
		if p, ok := pm.providers["openai"]; ok {
			return p
		}
	}

	// DeepSeek → OpenAI（兼容接口）
	if strings.HasPrefix(lower, "deepseek") {
		if p, ok := pm.providers["openai"]; ok {
			return p
		}
	}

	// 默认使用当前活跃供应商
	if pm.activeProvider != nil {
		return pm.activeProvider
	}

	// 兜底：返回任意可用供应商
	for _, p := range pm.providers {
		return p
	}
	return nil
}

// isRetryableError 判断错误是否可重试
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// 网络错误
	if strings.Contains(msg, "网络错误") || strings.Contains(msg, "connection") ||
		strings.Contains(msg, "timeout") || strings.Contains(msg, "EOF") {
		return true
	}
	// 429 限流
	if strings.Contains(msg, "频率超限") || strings.Contains(msg, "429") {
		return true
	}
	// 5xx 服务器错误
	if strings.Contains(msg, "服务暂时不可用") || strings.Contains(msg, "500") ||
		strings.Contains(msg, "502") || strings.Contains(msg, "503") {
		return true
	}
	return false
}
