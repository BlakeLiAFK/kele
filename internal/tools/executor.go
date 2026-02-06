package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BlakeLiAFK/kele/internal/llm"
)

// Executor 工具执行器
type Executor struct {
	workDir string
}

// NewExecutor 创建执行器
func NewExecutor() *Executor {
	wd, _ := os.Getwd()
	return &Executor{
		workDir: wd,
	}
}

// GetTools 获取所有可用工具
func (e *Executor) GetTools() []llm.Tool {
	return []llm.Tool{
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "bash",
				Description: "执行 bash 命令。可以用来列出文件、查看目录结构、运行程序等。",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]interface{}{
							"type":        "string",
							"description": "要执行的 bash 命令",
						},
					},
					"required": []string{"command"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "read",
				Description: "读取文件内容。用于查看源代码、配置文件等。",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "文件路径（相对或绝对路径）",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "write",
				Description: "写入或创建文件。用于修改代码、创建新文件等。",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "文件路径（相对或绝对路径）",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "要写入的内容",
						},
					},
					"required": []string{"path", "content"},
				},
			},
		},
	}
}

// Execute 执行工具调用
func (e *Executor) Execute(toolCall llm.ToolCall) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("解析参数失败: %v", err)
	}

	switch toolCall.Function.Name {
	case "bash":
		return e.executeBash(args)
	case "read":
		return e.executeRead(args)
	case "write":
		return e.executeWrite(args)
	default:
		return "", fmt.Errorf("未知工具: %s", toolCall.Function.Name)
	}
}

// executeBash 执行 bash 命令
func (e *Executor) executeBash(args map[string]interface{}) (string, error) {
	command, ok := args["command"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 command 参数")
	}

	// 安全检查：禁止危险命令
	dangerous := []string{"rm -rf", "dd if=", "mkfs", "> /dev/"}
	for _, d := range dangerous {
		if strings.Contains(command, d) {
			return "", fmt.Errorf("禁止执行危险命令: %s", command)
		}
	}

	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = e.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}

	return string(output), nil
}

// executeRead 读取文件
func (e *Executor) executeRead(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 path 参数")
	}

	// 处理相对路径
	if !filepath.IsAbs(path) {
		path = filepath.Join(e.workDir, path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}

	return string(content), nil
}

// executeWrite 写入文件
func (e *Executor) executeWrite(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 path 参数")
	}

	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 content 参数")
	}

	// 处理相对路径
	if !filepath.IsAbs(path) {
		path = filepath.Join(e.workDir, path)
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %v", err)
	}

	// 写入文件
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("写入文件失败: %v", err)
	}

	return fmt.Sprintf("成功写入文件: %s (%d bytes)", path, len(content)), nil
}
