package tools

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GitTool Git 操作工具
type GitTool struct {
	workDir       string
	maxOutputSize int
}

// NewGitTool 创建 Git 工具
func NewGitTool(workDir string, maxOutputSize int) *GitTool {
	return &GitTool{workDir: workDir, maxOutputSize: maxOutputSize}
}

func (t *GitTool) Name() string { return "git" }

func (t *GitTool) Description() string {
	return "执行 Git 操作。支持 status/diff/log/add/commit/branch/checkout 子命令。禁止 push --force、reset --hard 等危险操作。"
}

func (t *GitTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"subcommand": map[string]interface{}{
				"type":        "string",
				"description": "Git 子命令: status, diff, log, add, commit, branch, checkout, show, stash",
			},
			"args": map[string]interface{}{
				"type":        "string",
				"description": "子命令的参数（可选），如 commit 时传 '-m \"message\"'",
			},
		},
		"required": []string{"subcommand"},
	}
}

// 允许的 Git 子命令
var allowedGitCommands = map[string]bool{
	"status":   true,
	"diff":     true,
	"log":      true,
	"add":      true,
	"commit":   true,
	"branch":   true,
	"checkout": true,
	"show":     true,
	"stash":    true,
	"blame":    true,
	"tag":      true,
	"remote":   true,
}

// 危险的 Git 参数组合
var dangerousGitPatterns = []string{
	"push --force",
	"push -f",
	"reset --hard",
	"clean -f",
	"clean -fd",
	"branch -D",
	"checkout .",
	"restore .",
}

func (t *GitTool) Execute(args map[string]interface{}) (string, error) {
	subcommand, _ := args["subcommand"].(string)
	if subcommand == "" {
		return "", fmt.Errorf("缺少 subcommand 参数")
	}

	// 检查是否为允许的子命令
	if !allowedGitCommands[subcommand] {
		return "", fmt.Errorf("不支持的 Git 子命令: %s (支持: status/diff/log/add/commit/branch/checkout/show/stash)", subcommand)
	}

	// 构建完整命令
	cmdArgs := []string{subcommand}
	if extraArgs, ok := args["args"].(string); ok && extraArgs != "" {
		// 安全检查
		fullCmd := subcommand + " " + extraArgs
		for _, pattern := range dangerousGitPatterns {
			if strings.Contains(fullCmd, pattern) {
				return "", fmt.Errorf("禁止执行危险 Git 操作: git %s", fullCmd)
			}
		}
		cmdArgs = append(cmdArgs, strings.Fields(extraArgs)...)
	}

	// 检测是否在 Git 仓库中
	checkCmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	checkCmd.Dir = t.workDir
	if err := checkCmd.Run(); err != nil {
		return "", fmt.Errorf("当前目录不是 Git 仓库")
	}

	cmd := exec.Command("git", cmdArgs...)
	cmd.Dir = t.workDir

	// 设置超时
	done := make(chan error, 1)
	var output []byte
	go func() {
		var err error
		output, err = cmd.CombinedOutput()
		done <- err
	}()

	select {
	case err := <-done:
		result := string(output)
		if t.maxOutputSize > 0 && len(result) > t.maxOutputSize {
			result = result[:t.maxOutputSize] + fmt.Sprintf("\n\n... [输出被截断，超过 %d 字节]", t.maxOutputSize)
		}
		// diff 输出添加语法高亮标记
		if subcommand == "diff" || subcommand == "show" {
			result = highlightDiff(result)
		}
		if err != nil {
			return result, nil // Git 命令有时以非零退出码返回有效信息（如 diff）
		}
		return result, nil
	case <-time.After(30 * time.Second):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return "", fmt.Errorf("Git 命令执行超时")
	}
}

// highlightDiff 为 diff 输出添加 ANSI 颜色
func highlightDiff(input string) string {
	lines := strings.Split(input, "\n")
	var sb strings.Builder
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- "):
			sb.WriteString("\033[1m" + line + "\033[0m\n") // 粗体
		case strings.HasPrefix(line, "+"):
			sb.WriteString("\033[32m" + line + "\033[0m\n") // 绿色
		case strings.HasPrefix(line, "-"):
			sb.WriteString("\033[31m" + line + "\033[0m\n") // 红色
		case strings.HasPrefix(line, "@@"):
			sb.WriteString("\033[36m" + line + "\033[0m\n") // 青色
		default:
			sb.WriteString(line + "\n")
		}
	}
	// 去掉末尾多余换行
	result := sb.String()
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}
	return result
}
