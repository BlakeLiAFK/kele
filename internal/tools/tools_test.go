package tools

import (
	"strings"
	"testing"

	"github.com/BlakeLiAFK/kele/internal/config"
)

// --- Registry 测试 ---

func TestRegistryBasic(t *testing.T) {
	r := NewRegistry()

	// 注册一个工具
	r.Register(&mockTool{name: "test_tool"})

	if !r.Has("test_tool") {
		t.Error("注册后 Has 应返回 true")
	}
	if r.Has("nonexistent") {
		t.Error("未注册工具 Has 应返回 false")
	}

	list := r.List()
	if len(list) != 1 || list[0] != "test_tool" {
		t.Errorf("List 应返回 [test_tool], 实际 %v", list)
	}
}

func TestRegistryExecute(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "echo", result: "hello"})

	result, err := r.Execute("echo", map[string]interface{}{})
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if result != "hello" {
		t.Errorf("结果应为 hello, 实际 %s", result)
	}
}

func TestRegistryExecuteUnknown(t *testing.T) {
	r := NewRegistry()
	_, err := r.Execute("unknown", map[string]interface{}{})
	if err == nil {
		t.Error("执行未知工具应返回错误")
	}
}

func TestRegistryOrder(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "c_tool"})
	r.Register(&mockTool{name: "a_tool"})
	r.Register(&mockTool{name: "b_tool"})

	list := r.List()
	if len(list) != 3 {
		t.Fatalf("应有 3 个工具, 实际 %d", len(list))
	}
	if list[0] != "c_tool" || list[1] != "a_tool" || list[2] != "b_tool" {
		t.Errorf("应保持注册顺序, 实际 %v", list)
	}
}

func TestRegistryGetTools(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "bash", desc: "run commands"})

	tools := r.GetTools()
	if len(tools) != 1 {
		t.Fatalf("应有 1 个工具, 实际 %d", len(tools))
	}
	if tools[0].Function.Name != "bash" {
		t.Errorf("工具名应为 bash, 实际 %s", tools[0].Function.Name)
	}
	if tools[0].Function.Description != "run commands" {
		t.Errorf("描述应为 run commands, 实际 %s", tools[0].Function.Description)
	}
	if tools[0].Type != "function" {
		t.Errorf("类型应为 function, 实际 %s", tools[0].Type)
	}
}

// --- BashTool 安全性测试 ---

func TestBashDangerousCommand(t *testing.T) {
	cfg := config.Load()
	bash := &BashTool{
		workDir: "/tmp",
		cfg:     cfg,
		maxOutputSize: 1024,
		timeout: 5,
	}

	_, err := bash.Execute(map[string]interface{}{
		"command": "rm -rf /",
	})
	if err == nil {
		t.Error("危险命令应被拒绝")
	}
	if !strings.Contains(err.Error(), "禁止") {
		t.Errorf("错误消息应包含'禁止', 实际 %s", err.Error())
	}
}

func TestBashSafeCommand(t *testing.T) {
	cfg := config.Load()
	bash := &BashTool{
		workDir: "/tmp",
		cfg:     cfg,
		maxOutputSize: 1024,
		timeout: 5e9,
	}

	result, err := bash.Execute(map[string]interface{}{
		"command": "echo hello",
	})
	if err != nil {
		t.Fatalf("安全命令不应报错: %v", err)
	}
	if !strings.Contains(result, "hello") {
		t.Errorf("结果应包含 hello, 实际 %s", result)
	}
}

func TestBashMissingCommand(t *testing.T) {
	cfg := config.Load()
	bash := &BashTool{workDir: "/tmp", cfg: cfg}
	_, err := bash.Execute(map[string]interface{}{})
	if err == nil {
		t.Error("缺少 command 参数应报错")
	}
}

// --- GitTool 测试 ---

func TestGitSafeCommands(t *testing.T) {
	git := NewGitTool("/tmp", 1024)

	// status 应该被允许（即使不在 git 仓库中会报错，但不是安全拒绝）
	_, err := git.Execute(map[string]interface{}{
		"command": "status",
	})
	// 可能因不在 git 仓库中失败，但不应是"不支持"错误
	if err != nil && strings.Contains(err.Error(), "不支持") {
		t.Error("git status 应该被允许")
	}
}

func TestGitDangerousPatterns(t *testing.T) {
	git := NewGitTool("/tmp", 1024)

	dangerousCmds := []string{
		"push --force",
		"reset --hard",
		"clean -f",
		"branch -D main",
	}

	for _, cmd := range dangerousCmds {
		_, err := git.Execute(map[string]interface{}{
			"command": cmd,
		})
		if err == nil {
			t.Errorf("git %s 应被拒绝", cmd)
		}
	}
}

func TestGitUnsupportedCommand(t *testing.T) {
	git := NewGitTool("/tmp", 1024)

	_, err := git.Execute(map[string]interface{}{
		"command": "rebase",
	})
	if err == nil {
		t.Error("不支持的 git 命令应报错")
	}
}

// --- HTTPTool 测试 ---

func TestHTTPURLSafety(t *testing.T) {
	http := NewHTTPTool(1024)

	// 内网地址应被拒绝
	unsafeURLs := []string{
		"http://127.0.0.1/secret",
		"http://localhost/admin",
		"http://192.168.1.1/config",
		"http://10.0.0.1/internal",
		"http://service.local/api",
	}

	for _, url := range unsafeURLs {
		_, err := http.Execute(map[string]interface{}{
			"url":    url,
			"method": "GET",
		})
		if err == nil {
			t.Errorf("内网 URL %s 应被拒绝", url)
		}
	}
}

// --- PythonTool 测试 ---

func TestPythonExecution(t *testing.T) {
	py := NewPythonTool("/tmp", 1024)

	result, err := py.Execute(map[string]interface{}{
		"code": "print('hello from python')",
	})
	if err != nil {
		t.Skipf("Python 不可用: %v", err)
	}
	if !strings.Contains(result, "hello from python") {
		t.Errorf("结果应包含 'hello from python', 实际 %s", result)
	}
}

func TestPythonMissingCode(t *testing.T) {
	py := NewPythonTool("/tmp", 1024)
	_, err := py.Execute(map[string]interface{}{})
	if err == nil {
		t.Error("缺少 code 参数应报错")
	}
}

// --- mock 工具 ---

type mockTool struct {
	name   string
	desc   string
	result string
}

func (m *mockTool) Name() string                                        { return m.name }
func (m *mockTool) Description() string                                 { return m.desc }
func (m *mockTool) Parameters() map[string]interface{}                  { return map[string]interface{}{} }
func (m *mockTool) Execute(args map[string]interface{}) (string, error) { return m.result, nil }
