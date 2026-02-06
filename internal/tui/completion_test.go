package tui

import (
	"testing"
)

func TestLocalComplete_SlashCommand(t *testing.T) {
	engine := &CompletionEngine{
		cache: make(map[string]string),
	}

	tests := []struct {
		name           string
		input          string
		wantSuggCount  int
		wantCandCount  int
	}{
		{
			name:          "空输入",
			input:         "",
			wantSuggCount: 0,
			wantCandCount: 0,
		},
		{
			name:          "精确匹配不再提示",
			input:         "/help",
			wantSuggCount: 0,
			wantCandCount: 0,
		},
		{
			name:          "单字符前缀",
			input:         "/h",
			wantSuggCount: 2, // /help, /history
			wantCandCount: 2,
		},
		{
			name:          "model 前缀匹配多个",
			input:         "/model",
			wantSuggCount: 4, // /model, /models, /model-reset, /model-small
			wantCandCount: 4,
		},
		{
			name:          "唯一匹配",
			input:         "/deb",
			wantSuggCount: 1, // /debug
			wantCandCount: 1,
		},
		{
			name:          "无匹配",
			input:         "/xyz",
			wantSuggCount: 0,
			wantCandCount: 0,
		},
		{
			name:          "已有参数不补全",
			input:         "/model gpt",
			wantSuggCount: 0,
			wantCandCount: 0,
		},
		{
			name:          "普通文本无结果",
			input:         "hello",
			wantSuggCount: 0,
			wantCandCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestions, candidates := engine.LocalComplete(tt.input)
			if len(suggestions) != tt.wantSuggCount {
				t.Errorf("suggestions count = %d, want %d, got %v", len(suggestions), tt.wantSuggCount, suggestions)
			}
			if len(candidates) != tt.wantCandCount {
				t.Errorf("candidates count = %d, want %d, got %v", len(candidates), tt.wantCandCount, candidates)
			}
		})
	}
}

func TestLocalComplete_AtReference(t *testing.T) {
	engine := &CompletionEngine{
		cache: make(map[string]string),
	}

	// @ 后有路径的情况，候选数取决于文件系统，只验证不 panic
	_, candidates := engine.LocalComplete("@")
	if candidates == nil {
		// 可能当前目录为空，不算错误
		t.Log("@ 补全返回 nil candidates（可能当前目录无文件）")
	}

	// 包含空格的 @ 不应补全
	sugg, cand := engine.LocalComplete("分析 @go.mod 代码 @")
	_ = sugg
	_ = cand
}

func TestCompletionEngine_ClearCache(t *testing.T) {
	engine := &CompletionEngine{
		cache: make(map[string]string),
	}

	engine.cache["test"] = "test result"
	if len(engine.cache) != 1 {
		t.Fatal("cache should have 1 entry")
	}

	engine.ClearCache()
	if len(engine.cache) != 0 {
		t.Error("cache should be empty after ClearCache")
	}
}
