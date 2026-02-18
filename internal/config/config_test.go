package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// 清理可能干扰的环境变量
	os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("ANTHROPIC_API_KEY")

	cfg := Load()

	if cfg.LLM.OpenAIAPIBase != "https://api.openai.com/v1" {
		t.Errorf("默认 API Base 应为 OpenAI, 实际 %s", cfg.LLM.OpenAIAPIBase)
	}
	if cfg.LLM.OpenAIModel != "gpt-4o" {
		t.Errorf("默认模型应为 gpt-4o, 实际 %s", cfg.LLM.OpenAIModel)
	}
	if cfg.LLM.Temperature != 0.7 {
		t.Errorf("默认温度应为 0.7, 实际 %f", cfg.LLM.Temperature)
	}
	if cfg.LLM.MaxTokens != 4096 {
		t.Errorf("默认最大 tokens 应为 4096, 实际 %d", cfg.LLM.MaxTokens)
	}
	if cfg.Tools.BashTimeout != 60 {
		t.Errorf("默认 bash 超时应为 60, 实际 %d", cfg.Tools.BashTimeout)
	}
	if cfg.TUI.MaxSessions != 9 {
		t.Errorf("默认最大会话数应为 9, 实际 %d", cfg.TUI.MaxSessions)
	}
	if cfg.Memory.DBPath != ".kele/memory.db" {
		t.Errorf("默认数据库路径应为 .kele/memory.db, 实际 %s", cfg.Memory.DBPath)
	}
}

func TestLoadFromEnv(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "test-key-123")
	os.Setenv("OPENAI_MODEL", "gpt-3.5-turbo")
	os.Setenv("KELE_MAX_TOKENS", "8192")
	defer func() {
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("OPENAI_MODEL")
		os.Unsetenv("KELE_MAX_TOKENS")
	}()

	cfg := Load()

	if cfg.LLM.OpenAIAPIKey != "test-key-123" {
		t.Errorf("API Key 应为 test-key-123, 实际 %s", cfg.LLM.OpenAIAPIKey)
	}
	if cfg.LLM.OpenAIModel != "gpt-3.5-turbo" {
		t.Errorf("模型应为 gpt-3.5-turbo, 实际 %s", cfg.LLM.OpenAIModel)
	}
	if cfg.LLM.MaxTokens != 8192 {
		t.Errorf("MaxTokens 应为 8192, 实际 %d", cfg.LLM.MaxTokens)
	}
}

func TestIsDangerous(t *testing.T) {
	cfg := Load()

	tests := []struct {
		cmd      string
		expected bool
	}{
		{"ls -la", false},
		{"rm -rf /", true},
		{"rm -rf ~", true},
		{"dd if=/dev/zero", true},
		{"mkfs.ext4 /dev/sda", true},
		{"> /dev/sda", true},
		{"echo hello", false},
		{"git status", false},
	}

	for _, tt := range tests {
		result := cfg.IsDangerous(tt.cmd)
		if result != tt.expected {
			t.Errorf("IsDangerous(%q) = %v, 期望 %v", tt.cmd, result, tt.expected)
		}
	}
}

func TestHasOpenAI(t *testing.T) {
	os.Unsetenv("OPENAI_API_KEY")
	cfg := Load()
	if cfg.HasOpenAI() {
		t.Error("未设置 OPENAI_API_KEY 时 HasOpenAI 应为 false")
	}

	os.Setenv("OPENAI_API_KEY", "sk-test")
	defer os.Unsetenv("OPENAI_API_KEY")
	cfg = Load()
	if !cfg.HasOpenAI() {
		t.Error("设置 OPENAI_API_KEY 后 HasOpenAI 应为 true")
	}
}

func TestHasAnthropic(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")
	cfg := Load()
	if cfg.HasAnthropic() {
		t.Error("未设置 ANTHROPIC_API_KEY 时 HasAnthropic 应为 false")
	}

	os.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	defer os.Unsetenv("ANTHROPIC_API_KEY")
	cfg = Load()
	if !cfg.HasAnthropic() {
		t.Error("设置 ANTHROPIC_API_KEY 后 HasAnthropic 应为 true")
	}
}

func TestApplyFlags(t *testing.T) {
	cfg := Load()
	cfg.ApplyFlags("custom-model", true, "/tmp/test.yaml")

	if cfg.LLM.OpenAIModel != "custom-model" {
		t.Errorf("ApplyFlags 应覆盖模型为 custom-model, 实际 %s", cfg.LLM.OpenAIModel)
	}
	if !cfg.Debug {
		t.Error("ApplyFlags 应设置 Debug 为 true")
	}
	if cfg.ConfigPath != "/tmp/test.yaml" {
		t.Errorf("ApplyFlags 应设置 ConfigPath 为 /tmp/test.yaml, 实际 %s", cfg.ConfigPath)
	}
}

func TestVersion(t *testing.T) {
	if Version != "0.2.0" {
		t.Errorf("版本号应为 0.2.0, 实际 %s", Version)
	}
}
