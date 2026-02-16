package llm

import (
	"os"
	"testing"

	"github.com/BlakeLiAFK/kele/internal/config"
)

func TestProviderManagerCreation(t *testing.T) {
	// 无 API Key 的情况
	os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("ANTHROPIC_API_KEY")
	cfg := config.Load()

	pm := NewProviderManager(cfg)

	// 应至少有 Ollama
	providers := pm.ListProviders()
	found := false
	for _, p := range providers {
		if p == "ollama" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Ollama 应始终被注册")
	}

	// 应有活跃供应商
	if pm.GetActiveProviderName() == "none" {
		t.Error("应有活跃供应商（至少 Ollama）")
	}
}

func TestProviderManagerWithOpenAI(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "sk-test")
	defer os.Unsetenv("OPENAI_API_KEY")
	cfg := config.Load()

	pm := NewProviderManager(cfg)

	if pm.GetActiveProviderName() != "openai" {
		t.Errorf("有 OpenAI Key 时默认供应商应为 openai, 实际 %s", pm.GetActiveProviderName())
	}
}

func TestProviderManagerModelRouting(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "sk-test")
	os.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	defer func() {
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("ANTHROPIC_API_KEY")
	}()

	cfg := config.Load()
	pm := NewProviderManager(cfg)

	// GPT 模型 → OpenAI
	pm.SetModel("gpt-4o")
	if pm.GetActiveProviderName() != "openai" {
		t.Errorf("gpt-4o 应路由到 openai, 实际 %s", pm.GetActiveProviderName())
	}

	// Claude 模型 → Anthropic
	pm.SetModel("claude-3-5-sonnet-20241022")
	if pm.GetActiveProviderName() != "anthropic" {
		t.Errorf("claude 模型应路由到 anthropic, 实际 %s", pm.GetActiveProviderName())
	}

	// Ollama 模型 → Ollama
	pm.SetModel("llama3:8b")
	if pm.GetActiveProviderName() != "ollama" {
		t.Errorf("llama3:8b 应路由到 ollama, 实际 %s", pm.GetActiveProviderName())
	}

	// DeepSeek → OpenAI
	pm.SetModel("deepseek-chat")
	if pm.GetActiveProviderName() != "openai" {
		t.Errorf("deepseek-chat 应路由到 openai, 实际 %s", pm.GetActiveProviderName())
	}

	// o1/o3 → OpenAI
	pm.SetModel("o1-preview")
	if pm.GetActiveProviderName() != "openai" {
		t.Errorf("o1-preview 应路由到 openai, 实际 %s", pm.GetActiveProviderName())
	}
}

func TestProviderManagerModelState(t *testing.T) {
	cfg := config.Load()
	pm := NewProviderManager(cfg)

	defaultModel := pm.GetDefaultModel()
	if defaultModel == "" {
		t.Error("默认模型不应为空")
	}

	// SetModel
	pm.SetModel("test-model")
	if pm.GetModel() != "test-model" {
		t.Errorf("SetModel 后应为 test-model, 实际 %s", pm.GetModel())
	}

	// ResetModel
	pm.ResetModel()
	if pm.GetModel() != defaultModel {
		t.Errorf("ResetModel 后应恢复为 %s, 实际 %s", defaultModel, pm.GetModel())
	}

	// SmallModel
	pm.SetSmallModel("small-test")
	if pm.GetSmallModel() != "small-test" {
		t.Errorf("SmallModel 应为 small-test, 实际 %s", pm.GetSmallModel())
	}
}

func TestProviderManagerNoSmallModel(t *testing.T) {
	os.Unsetenv("KELE_SMALL_MODEL")
	cfg := config.Load()
	pm := NewProviderManager(cfg)

	// 未设置小模型时应回落到主模型
	small := pm.GetSmallModel()
	main := pm.GetModel()
	if small != main {
		t.Errorf("未设置小模型时应回落到主模型 %s, 实际 %s", main, small)
	}
}

// TestAnthropicMessageConversion 测试 Anthropic 消息格式转换
func TestAnthropicMessageConversion(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "You are a helpful assistant"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
		{Role: "user", Content: "How are you?"},
	}

	system, anthropicMsgs := convertToAnthropic(messages)

	if system != "You are a helpful assistant" {
		t.Errorf("System 应被提取, 实际 %s", system)
	}

	if len(anthropicMsgs) != 3 {
		t.Fatalf("应有 3 条消息（排除 system）, 实际 %d", len(anthropicMsgs))
	}

	if anthropicMsgs[0].Role != "user" {
		t.Errorf("第一条应为 user, 实际 %s", anthropicMsgs[0].Role)
	}
}

func TestAnthropicToolConversion(t *testing.T) {
	tools := []Tool{
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "bash",
				Description: "Execute bash commands",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
		},
	}

	anthropicTools := convertToolsToAnthropic(tools)
	if len(anthropicTools) != 1 {
		t.Fatalf("应有 1 个工具, 实际 %d", len(anthropicTools))
	}
	if anthropicTools[0].Name != "bash" {
		t.Errorf("工具名应为 bash, 实际 %s", anthropicTools[0].Name)
	}
}

func TestAnthropicResponseConversion(t *testing.T) {
	resp := &anthropicResponse{
		ID:    "msg_123",
		Model: "claude-3-5-sonnet",
		Content: []anthropicContentBlock{
			{Type: "text", Text: "Hello!"},
		},
		StopReason: "end_turn",
	}
	resp.Usage.InputTokens = 10
	resp.Usage.OutputTokens = 5

	chatResp := convertFromAnthropic(resp)
	if chatResp.ID != "msg_123" {
		t.Errorf("ID 应为 msg_123, 实际 %s", chatResp.ID)
	}
	if len(chatResp.Choices) != 1 {
		t.Fatalf("应有 1 个 choice, 实际 %d", len(chatResp.Choices))
	}
	if chatResp.Choices[0].Message.Content != "Hello!" {
		t.Errorf("内容应为 Hello!, 实际 %s", chatResp.Choices[0].Message.Content)
	}
	if chatResp.Choices[0].FinishReason != "stop" {
		t.Errorf("end_turn 应转为 stop, 实际 %s", chatResp.Choices[0].FinishReason)
	}
	if chatResp.Usage.TotalTokens != 15 {
		t.Errorf("总 token 应为 15, 实际 %d", chatResp.Usage.TotalTokens)
	}
}

func TestAnthropicToolUseConversion(t *testing.T) {
	resp := &anthropicResponse{
		ID:    "msg_456",
		Model: "claude-3-5-sonnet",
		Content: []anthropicContentBlock{
			{Type: "text", Text: "Let me check that."},
			{Type: "tool_use", ID: "toolu_123", Name: "bash", Input: map[string]interface{}{"command": "ls -la"}},
		},
		StopReason: "tool_use",
	}

	chatResp := convertFromAnthropic(resp)
	if len(chatResp.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("应有 1 个工具调用, 实际 %d", len(chatResp.Choices[0].Message.ToolCalls))
	}
	tc := chatResp.Choices[0].Message.ToolCalls[0]
	if tc.Function.Name != "bash" {
		t.Errorf("工具名应为 bash, 实际 %s", tc.Function.Name)
	}
	if chatResp.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("tool_use 应转为 tool_calls, 实际 %s", chatResp.Choices[0].FinishReason)
	}
}

func TestClassifyAPIError(t *testing.T) {
	tests := []struct {
		code     int
		contains string
	}{
		{401, "认证失败"},
		{403, "权限不足"},
		{404, "模型不存在"},
		{429, "频率超限"},
		{500, "不可用"},
		{502, "不可用"},
		{418, "API 错误"},
	}

	for _, tt := range tests {
		err := classifyAPIError(tt.code, "test body")
		if err == nil {
			t.Errorf("HTTP %d 应返回错误", tt.code)
			continue
		}
		if !containsSubstring(err.Error(), tt.contains) {
			t.Errorf("HTTP %d 错误应包含 %q, 实际 %s", tt.code, tt.contains, err.Error())
		}
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsRune(s, sub))
}

func containsRune(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
