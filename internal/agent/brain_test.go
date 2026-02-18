package agent

import (
	"strings"
	"testing"

	"github.com/BlakeLiAFK/kele/internal/config"
	"github.com/BlakeLiAFK/kele/internal/llm"
)

func testBrain() *Brain {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			OpenAIAPIBase: "https://api.openai.com/v1",
			OpenAIModel:   "gpt-4o",
			SmallModel:    "gpt-4o-mini",
			Temperature:   0.7,
			MaxTokens:     4096,
			MaxToolRounds: 10,
			MaxTurns:      20,
			OllamaHost:    "http://localhost:11434",
		},
		Tools: config.ToolsConfig{
			DangerousCommands: config.DefaultDangerousCommands,
			BashTimeout:       60,
			MaxOutputSize:     51200,
			MaxWriteSize:      1048576,
		},
		Memory: config.MemoryConfig{
			DBPath:     "/tmp/kele-test-nonexistent/memory.db",
			MemoryFile: "/tmp/kele-test-nonexistent/MEMORY.md",
			SessionDir: "/tmp/kele-test-nonexistent/sessions",
		},
		TUI: config.TUIConfig{
			MaxSessions:   9,
			MaxInputChars: 5000,
		},
	}
	return NewBrain(nil, cfg)
}

func TestAddMessageAndHistory(t *testing.T) {
	b := testBrain()

	b.addMessage("user", "hello")
	b.addMessage("assistant", "hi there")

	history := b.GetHistory()
	if len(history) != 2 {
		t.Fatalf("应有 2 条历史, 实际 %d", len(history))
	}
	if history[0].Role != "user" || history[0].Content != "hello" {
		t.Errorf("第一条消息不匹配: %+v", history[0])
	}
}

func TestTrimHistory(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			MaxTurns:      3,
			MaxToolRounds: 10,
			OllamaHost:    "http://localhost:11434",
		},
		Tools: config.ToolsConfig{
			DangerousCommands: config.DefaultDangerousCommands,
			BashTimeout:       60,
			MaxOutputSize:     51200,
		},
		Memory: config.MemoryConfig{
			DBPath:     "/tmp/kele-test-nonexistent/memory.db",
			MemoryFile: "/tmp/kele-test-nonexistent/MEMORY.md",
			SessionDir: "/tmp/kele-test-nonexistent/sessions",
		},
	}
	b := NewBrain(nil, cfg)

	// MaxTurns=3 → 最多保留 6 条消息
	for i := 0; i < 10; i++ {
		b.addMessage("user", "msg")
		b.addMessage("assistant", "reply")
	}

	if len(b.GetHistory()) > 6 {
		t.Errorf("trimHistory 应限制在 6 条, 实际 %d", len(b.GetHistory()))
	}
}

func TestClearHistory(t *testing.T) {
	b := testBrain()

	b.addMessage("user", "hello")
	b.ClearHistory()

	if len(b.GetHistory()) != 0 {
		t.Errorf("ClearHistory 后应为空, 实际 %d", len(b.GetHistory()))
	}
}

func TestCompressToolOutput(t *testing.T) {
	b := testBrain()

	// 短输出不压缩
	short := "hello world"
	if b.compressToolOutput(short) != short {
		t.Error("短输出不应被压缩")
	}

	// 超过 2KB 的输出应被压缩
	long := strings.Repeat("x", 3000)
	compressed := b.compressToolOutput(long)
	if len(compressed) >= len(long) {
		t.Error("长输出应被压缩")
	}
	if !strings.Contains(compressed, "省略") {
		t.Error("压缩后应包含省略标记")
	}
}

func TestEstimateTokens(t *testing.T) {
	b := testBrain()

	b.addMessage("user", "hello world")
	tokens := b.EstimateTokens()
	if tokens <= 0 {
		t.Error("EstimateTokens 应返回正数")
	}
}

func TestGetMessages(t *testing.T) {
	b := testBrain()

	b.addMessage("user", "test")
	msgs := b.getMessages()

	// 第一条应为 system prompt
	if msgs[0].Role != "system" {
		t.Errorf("第一条应为 system, 实际 %s", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content, "Kele") {
		t.Error("system prompt 应包含 Kele")
	}

	// 第二条应为用户消息
	if len(msgs) < 2 || msgs[1].Role != "user" {
		t.Error("第二条应为用户消息")
	}
}

func TestGetProviderInfo(t *testing.T) {
	b := testBrain()
	info := b.GetProviderInfo()

	if _, ok := info["provider"]; !ok {
		t.Error("GetProviderInfo 应包含 provider 字段")
	}
	if _, ok := info["model"]; !ok {
		t.Error("GetProviderInfo 应包含 model 字段")
	}
}

func TestModelOperations(t *testing.T) {
	b := testBrain()

	originalModel := b.GetModel()
	b.SetModel("test-model")
	if b.GetModel() != "test-model" {
		t.Errorf("SetModel 后应为 test-model, 实际 %s", b.GetModel())
	}

	b.ResetModel()
	if b.GetModel() != originalModel {
		t.Errorf("ResetModel 后应为 %s, 实际 %s", originalModel, b.GetModel())
	}
}

func TestAppendRawMessage(t *testing.T) {
	b := testBrain()

	tc := llm.ToolCall{ID: "call_1"}
	tc.Function.Name = "bash"
	tc.Function.Arguments = `{"command":"ls"}`
	msg := llm.Message{
		Role:      "assistant",
		Content:   "test content",
		ToolCalls: []llm.ToolCall{tc},
	}
	b.appendRawMessage(msg)

	history := b.GetHistory()
	if len(history) != 1 {
		t.Fatalf("应有 1 条历史, 实际 %d", len(history))
	}
	if len(history[0].ToolCalls) != 1 {
		t.Error("应保留 ToolCalls")
	}
}
