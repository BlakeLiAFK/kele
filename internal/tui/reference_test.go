package tui

import (
	"os"
	"testing"
)

func TestParseReferences(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantClean string
		wantCount int
	}{
		{
			name:      "无引用",
			input:     "你好世界",
			wantClean: "你好世界",
			wantCount: 0,
		},
		{
			name:      "单个文件引用",
			input:     "分析 @go.mod 的依赖",
			wantClean: "分析  的依赖",
			wantCount: 1,
		},
		{
			name:      "多个引用",
			input:     "对比 @go.mod 和 @Makefile",
			wantClean: "对比  和",
			wantCount: 2,
		},
		{
			name:      "目录引用",
			input:     "查看 @internal/ 结构",
			wantClean: "查看  结构",
			wantCount: 1,
		},
		{
			name:      "glob 引用",
			input:     "分析 @internal/tui/*.go",
			wantClean: "分析",
			wantCount: 1,
		},
		{
			name:      "重复引用",
			input:     "@go.mod @go.mod",
			wantClean: "",
			wantCount: 1, // 去重
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clean, refs := parseReferences(tt.input)
			if clean != tt.wantClean {
				t.Errorf("cleanText = %q, want %q", clean, tt.wantClean)
			}
			if len(refs) != tt.wantCount {
				t.Errorf("refs count = %d, want %d", len(refs), tt.wantCount)
			}
		})
	}
}

func TestResolveReference(t *testing.T) {
	// 测试读取真实文件
	ref := resolveReference("go.mod")
	if ref.Type != "file" {
		t.Errorf("expected type=file, got %s", ref.Type)
	}
	// go.mod 在项目根目录，但测试在 internal/tui/ 下运行
	// 所以可能找不到。这里只测试不 panic

	// 测试不存在的文件
	ref = resolveReference("nonexistent_file_12345.txt")
	if ref.Error == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestBuildContextMessage(t *testing.T) {
	refs := []Reference{
		{
			Path:    "test.go",
			Type:    "file",
			Content: "package main\n\nfunc main() {}",
		},
	}

	msg := buildContextMessage("分析这段代码", refs)
	if msg == "" {
		t.Error("expected non-empty message")
	}
	if len(msg) < 10 {
		t.Errorf("message too short: %s", msg)
	}
}

func TestFormatRefSummary(t *testing.T) {
	refs := []Reference{
		{Path: "main.go", Type: "file", Content: "code here"},
		{Path: "src/", Type: "dir", Content: "dir listing"},
	}
	summary := formatRefSummary(refs)
	if summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestCompleteFilePath(t *testing.T) {
	// 创建临时文件用于测试
	tmpDir := t.TempDir()
	os.WriteFile(tmpDir+"/test.go", []byte("test"), 0644)
	os.WriteFile(tmpDir+"/test.md", []byte("test"), 0644)
	os.Mkdir(tmpDir+"/subdir", 0755)

	// 切换到临时目录测试
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldDir)

	// 测试空前缀
	_, candidates := completeFilePath("")
	if len(candidates) == 0 {
		t.Error("expected candidates for empty prefix")
	}

	// 测试有前缀
	completed, candidates := completeFilePath("tes")
	if len(candidates) == 0 {
		t.Error("expected candidates for 'tes' prefix")
	}
	_ = completed
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		size int64
		want string
	}{
		{100, "100B"},
		{1024, "1.0KB"},
		{1024 * 1024, "1.0MB"},
		{2048, "2.0KB"},
	}

	for _, tt := range tests {
		got := formatSize(tt.size)
		if got != tt.want {
			t.Errorf("formatSize(%d) = %s, want %s", tt.size, got, tt.want)
		}
	}
}
