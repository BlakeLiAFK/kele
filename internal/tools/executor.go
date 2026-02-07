package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BlakeLiAFK/kele/internal/cron"
	"github.com/BlakeLiAFK/kele/internal/llm"
)

// Executor 工具执行器
type Executor struct {
	workDir   string
	scheduler *cron.Scheduler
}

// NewExecutor 创建执行器
func NewExecutor(scheduler *cron.Scheduler) *Executor {
	wd, _ := os.Getwd()
	return &Executor{
		workDir:   wd,
		scheduler: scheduler,
	}
}

// GetTools 获取所有可用工具
func (e *Executor) GetTools() []llm.Tool {
	tools := []llm.Tool{
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

	// 定时任务工具
	if e.scheduler != nil {
		tools = append(tools, e.cronTools()...)
	}

	return tools
}

// cronTools 返回定时任务相关工具定义
func (e *Executor) cronTools() []llm.Tool {
	return []llm.Tool{
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "cron_create",
				Description: "创建定时任务。使用标准 5 字段 cron 表达式（分 时 日 月 周）。支持 @hourly @daily @weekly @monthly 快捷方式。",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "任务名称（简短描述）",
						},
						"schedule": map[string]interface{}{
							"type":        "string",
							"description": "cron 表达式，如 '*/5 * * * *' 每5分钟, '0 9 * * 1-5' 工作日9点, '@daily' 每天0点",
						},
						"command": map[string]interface{}{
							"type":        "string",
							"description": "要执行的 bash 命令",
						},
					},
					"required": []string{"name", "schedule", "command"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "cron_list",
				Description: "列出所有定时任务及其状态。",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "cron_get",
				Description: "查看定时任务详情和最近执行日志。",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "任务 ID",
						},
					},
					"required": []string{"id"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "cron_update",
				Description: "更新定时任务属性。只需传入要修改的字段。可用于暂停/恢复任务。",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "任务 ID",
						},
						"name": map[string]interface{}{
							"type":        "string",
							"description": "新的任务名称",
						},
						"schedule": map[string]interface{}{
							"type":        "string",
							"description": "新的 cron 表达式",
						},
						"command": map[string]interface{}{
							"type":        "string",
							"description": "新的 bash 命令",
						},
						"enabled": map[string]interface{}{
							"type":        "boolean",
							"description": "是否启用（false 暂停，true 恢复）",
						},
					},
					"required": []string{"id"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "cron_delete",
				Description: "删除定时任务及其执行日志。",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "任务 ID",
						},
					},
					"required": []string{"id"},
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
	case "cron_create":
		return e.executeCronCreate(args)
	case "cron_list":
		return e.executeCronList()
	case "cron_get":
		return e.executeCronGet(args)
	case "cron_update":
		return e.executeCronUpdate(args)
	case "cron_delete":
		return e.executeCronDelete(args)
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

// --- 定时任务工具实现 ---

func (e *Executor) executeCronCreate(args map[string]interface{}) (string, error) {
	if e.scheduler == nil {
		return "", fmt.Errorf("定时任务调度器未初始化")
	}

	name, _ := args["name"].(string)
	schedule, _ := args["schedule"].(string)
	command, _ := args["command"].(string)

	if name == "" || schedule == "" || command == "" {
		return "", fmt.Errorf("缺少必填参数: name, schedule, command")
	}

	job, err := e.scheduler.CreateJob(name, schedule, command)
	if err != nil {
		return "", err
	}

	nextStr := "N/A"
	if job.NextRun != nil {
		nextStr = job.NextRun.Format("2006-01-02 15:04")
	}

	return fmt.Sprintf("定时任务已创建\nID: %s\n名称: %s\n表达式: %s\n命令: %s\n下次执行: %s",
		job.ID, job.Name, job.Schedule, job.Command, nextStr), nil
}

func (e *Executor) executeCronList() (string, error) {
	if e.scheduler == nil {
		return "", fmt.Errorf("定时任务调度器未初始化")
	}

	jobs, err := e.scheduler.ListJobs()
	if err != nil {
		return "", err
	}

	if len(jobs) == 0 {
		return "暂无定时任务", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("定时任务列表 (%d 个)\n\n", len(jobs)))
	for _, j := range jobs {
		status := "启用"
		if !j.Enabled {
			status = "暂停"
		}
		nextStr := "-"
		if j.NextRun != nil {
			nextStr = j.NextRun.Format("01-02 15:04")
		}
		sb.WriteString(fmt.Sprintf("  %s  %-12s  %-16s  [%s]  下次: %s\n",
			j.ID, j.Name, j.Schedule, status, nextStr))
	}
	return sb.String(), nil
}

func (e *Executor) executeCronGet(args map[string]interface{}) (string, error) {
	if e.scheduler == nil {
		return "", fmt.Errorf("定时任务调度器未初始化")
	}

	id, _ := args["id"].(string)
	if id == "" {
		return "", fmt.Errorf("缺少 id 参数")
	}

	job, logs, err := e.scheduler.GetJob(id)
	if err != nil {
		return "", err
	}

	status := "启用"
	if !job.Enabled {
		status = "暂停"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("任务详情\n\nID: %s\n名称: %s\n表达式: %s\n命令: %s\n状态: %s\n",
		job.ID, job.Name, job.Schedule, job.Command, status))

	if job.LastRun != nil {
		sb.WriteString(fmt.Sprintf("上次执行: %s\n", job.LastRun.Format("2006-01-02 15:04:05")))
	}
	if job.NextRun != nil {
		sb.WriteString(fmt.Sprintf("下次执行: %s\n", job.NextRun.Format("2006-01-02 15:04:05")))
	}
	if job.LastError != "" {
		sb.WriteString(fmt.Sprintf("上次错误: %s\n", job.LastError))
	}

	if len(logs) > 0 {
		sb.WriteString(fmt.Sprintf("\n最近执行日志 (%d 条)\n", len(logs)))
		for _, l := range logs {
			errMark := ""
			if l.Error != "" {
				errMark = " [ERR]"
			}
			output := l.Output
			if len(output) > 100 {
				output = output[:100] + "..."
			}
			sb.WriteString(fmt.Sprintf("  %s  %dms%s  %s\n",
				l.RunAt.Format("01-02 15:04"), l.DurationMs, errMark, strings.TrimSpace(output)))
		}
	}

	return sb.String(), nil
}

func (e *Executor) executeCronUpdate(args map[string]interface{}) (string, error) {
	if e.scheduler == nil {
		return "", fmt.Errorf("定时任务调度器未初始化")
	}

	id, _ := args["id"].(string)
	if id == "" {
		return "", fmt.Errorf("缺少 id 参数")
	}

	updates := make(map[string]interface{})
	if v, ok := args["name"]; ok {
		updates["name"] = v
	}
	if v, ok := args["schedule"]; ok {
		updates["schedule"] = v
	}
	if v, ok := args["command"]; ok {
		updates["command"] = v
	}
	if v, ok := args["enabled"]; ok {
		updates["enabled"] = v
	}

	job, err := e.scheduler.UpdateJob(id, updates)
	if err != nil {
		return "", err
	}

	status := "启用"
	if !job.Enabled {
		status = "暂停"
	}

	return fmt.Sprintf("任务已更新\nID: %s\n名称: %s\n表达式: %s\n命令: %s\n状态: %s",
		job.ID, job.Name, job.Schedule, job.Command, status), nil
}

func (e *Executor) executeCronDelete(args map[string]interface{}) (string, error) {
	if e.scheduler == nil {
		return "", fmt.Errorf("定时任务调度器未初始化")
	}

	id, _ := args["id"].(string)
	if id == "" {
		return "", fmt.Errorf("缺少 id 参数")
	}

	if err := e.scheduler.DeleteJob(id); err != nil {
		return "", err
	}

	return fmt.Sprintf("任务已删除: %s", id), nil
}
