package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// PythonTool Python 执行工具
type PythonTool struct {
	workDir       string
	maxOutputSize int
	timeout       time.Duration
}

// NewPythonTool 创建 Python 工具
func NewPythonTool(workDir string, maxOutputSize int) *PythonTool {
	return &PythonTool{
		workDir:       workDir,
		maxOutputSize: maxOutputSize,
		timeout:       30 * time.Second,
	}
}

func (t *PythonTool) SetWorkDir(dir string) { t.workDir = dir }
func (t *PythonTool) Name() string           { return "python" }

func (t *PythonTool) Description() string {
	return "执行 Python 代码片段。适用于数据分析、数学计算、文本处理等。超时限制 30 秒。"
}

func (t *PythonTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"code": map[string]interface{}{
				"type":        "string",
				"description": "要执行的 Python 代码",
			},
		},
		"required": []string{"code"},
	}
}

func (t *PythonTool) Execute(args map[string]interface{}) (string, error) {
	code, _ := args["code"].(string)
	if code == "" {
		return "", fmt.Errorf("缺少 code 参数")
	}

	// 检测 python3 是否可用
	pythonPath, err := findPython()
	if err != nil {
		return "", err
	}

	// 写入临时文件
	tmpFile, err := os.CreateTemp("", "kele_python_*.py")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(code); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("写入代码失败: %v", err)
	}
	tmpFile.Close()

	// 执行
	ctx, cancel := context.WithTimeout(context.Background(), t.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, pythonPath, tmpFile.Name())
	cmd.Dir = t.workDir

	output, err := cmd.CombinedOutput()
	result := string(output)

	if t.maxOutputSize > 0 && len(result) > t.maxOutputSize {
		result = result[:t.maxOutputSize] + fmt.Sprintf("\n\n... [输出被截断，超过 %d 字节]", t.maxOutputSize)
	}

	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("Python 执行超时 (%v)", t.timeout)
	}

	if err != nil {
		return result, nil // 返回 stderr 输出但不作为系统错误
	}

	return result, nil
}

// findPython 查找可用的 Python 解释器
func findPython() (string, error) {
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
		// 也检查常见路径
		for _, dir := range []string{"/usr/bin", "/usr/local/bin"} {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("未找到 Python 解释器。请安装 python3")
}
