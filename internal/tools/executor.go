package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BlakeLiAFK/kele/internal/config"
	"github.com/BlakeLiAFK/kele/internal/cron"
	"github.com/BlakeLiAFK/kele/internal/llm"
)

// Executor 工具执行器
type Executor struct {
	workDir   string
	scheduler *cron.Scheduler
	registry  *Registry
	cfg       *config.Config
	audit     *AuditLogger
}

// NewExecutor 创建执行器
func NewExecutor(scheduler *cron.Scheduler, cfg *config.Config) *Executor {
	wd, _ := os.Getwd()

	e := &Executor{
		workDir:   wd,
		scheduler: scheduler,
		registry:  NewRegistry(),
		cfg:       cfg,
		audit:     NewAuditLogger(cfg.Memory.AuditLog),
	}

	// 注册内置工具
	e.registry.Register(&BashTool{
		workDir:       wd,
		cfg:           cfg,
		maxOutputSize: cfg.Tools.MaxOutputSize,
		timeout:       time.Duration(cfg.Tools.BashTimeout) * time.Second,
	})
	e.registry.Register(&ReadTool{workDir: wd})
	e.registry.Register(&WriteTool{workDir: wd, maxWriteSize: cfg.Tools.MaxWriteSize})
	e.registry.Register(NewHTTPTool(cfg.Tools.MaxOutputSize))
	e.registry.Register(NewGitTool(wd, cfg.Tools.MaxOutputSize))
	e.registry.Register(NewPythonTool(wd, cfg.Tools.MaxOutputSize))

	return e
}

// GetTools 获取所有可用工具
func (e *Executor) GetTools() []llm.Tool {
	tools := e.registry.GetTools()

	if e.scheduler != nil {
		tools = append(tools, e.cronTools()...)
	}

	return tools
}

// Execute 执行工具调用（带审计日志）
func (e *Executor) Execute(toolCall llm.ToolCall) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("解析参数失败: %v", err)
	}

	name := toolCall.Function.Name
	start := time.Now()

	var result string
	var execErr error

	if e.registry.Has(name) {
		result, execErr = e.registry.Execute(name, args)
	} else {
		switch name {
		case "cron_create":
			result, execErr = e.executeCronCreate(args)
		case "cron_list":
			result, execErr = e.executeCronList()
		case "cron_get":
			result, execErr = e.executeCronGet(args)
		case "cron_update":
			result, execErr = e.executeCronUpdate(args)
		case "cron_delete":
			result, execErr = e.executeCronDelete(args)
		default:
			execErr = fmt.Errorf("未知工具: %s", name)
		}
	}

	// 审计日志
	e.audit.Log(name, args, result, execErr, time.Since(start))

	return result, execErr
}

// ListTools 列出所有工具名
func (e *Executor) ListTools() []string {
	tools := e.registry.List()
	if e.scheduler != nil {
		tools = append(tools, "cron_create", "cron_list", "cron_get", "cron_update", "cron_delete")
	}
	return tools
}

// --- 内置工具实现 ---

// BashTool Bash 命令工具
type BashTool struct {
	workDir       string
	cfg           *config.Config
	maxOutputSize int
	timeout       time.Duration
}

func (t *BashTool) Name() string { return "bash" }
func (t *BashTool) Description() string {
	return "执行 bash 命令。可以用来列出文件、查看目录结构、运行程序等。"
}
func (t *BashTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "要执行的 bash 命令",
			},
		},
		"required": []string{"command"},
	}
}

func (t *BashTool) Execute(args map[string]interface{}) (string, error) {
	command, ok := args["command"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 command 参数")
	}

	if t.cfg.IsDangerous(command) {
		return "", fmt.Errorf("禁止执行危险命令: %s", command)
	}

	ctx, cancel := context.WithTimeout(context.Background(), t.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = t.workDir
	output, err := cmd.CombinedOutput()
	result := string(output)

	if t.maxOutputSize > 0 && len(result) > t.maxOutputSize {
		result = result[:t.maxOutputSize] + fmt.Sprintf("\n\n... [输出被截断，超过 %d 字节]", t.maxOutputSize)
	}

	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("命令执行超时 (%v)", t.timeout)
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

// ReadTool 文件读取工具
type ReadTool struct{ workDir string }

func (t *ReadTool) Name() string        { return "read" }
func (t *ReadTool) Description() string { return "读取文件内容。用于查看源代码、配置文件等。" }
func (t *ReadTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{"type": "string", "description": "文件路径（相对或绝对路径）"},
		},
		"required": []string{"path"},
	}
}
func (t *ReadTool) Execute(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 path 参数")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.workDir, path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}
	return string(content), nil
}

// WriteTool 文件写入工具
type WriteTool struct {
	workDir      string
	maxWriteSize int
}

func (t *WriteTool) Name() string        { return "write" }
func (t *WriteTool) Description() string { return "写入或创建文件。用于修改代码、创建新文件等。" }
func (t *WriteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path":    map[string]interface{}{"type": "string", "description": "文件路径（相对或绝对路径）"},
			"content": map[string]interface{}{"type": "string", "description": "要写入的内容"},
		},
		"required": []string{"path", "content"},
	}
}
func (t *WriteTool) Execute(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 path 参数")
	}
	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("缺少 content 参数")
	}
	if t.maxWriteSize > 0 && len(content) > t.maxWriteSize {
		return "", fmt.Errorf("文件内容超过大小限制 (%d 字节)", t.maxWriteSize)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.workDir, path)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("写入文件失败: %v", err)
	}
	return fmt.Sprintf("成功写入文件: %s (%d bytes)", path, len(content)), nil
}

// --- 定时任务工具 ---

func (e *Executor) cronTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "cron_create", Description: "创建定时任务。使用标准 5 字段 cron 表达式。", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"name": map[string]interface{}{"type": "string", "description": "任务名称"}, "schedule": map[string]interface{}{"type": "string", "description": "cron 表达式"}, "command": map[string]interface{}{"type": "string", "description": "bash 命令"}}, "required": []string{"name", "schedule", "command"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "cron_list", Description: "列出所有定时任务。", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "cron_get", Description: "查看定时任务详情。", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"id": map[string]interface{}{"type": "string", "description": "任务 ID"}}, "required": []string{"id"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "cron_update", Description: "更新定时任务。", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"id": map[string]interface{}{"type": "string", "description": "任务 ID"}, "name": map[string]interface{}{"type": "string"}, "schedule": map[string]interface{}{"type": "string"}, "command": map[string]interface{}{"type": "string"}, "enabled": map[string]interface{}{"type": "boolean"}}, "required": []string{"id"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "cron_delete", Description: "删除定时任务。", Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"id": map[string]interface{}{"type": "string", "description": "任务 ID"}}, "required": []string{"id"}}}},
	}
}

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
	return fmt.Sprintf("定时任务已创建\nID: %s\n名称: %s\n表达式: %s\n命令: %s\n下次执行: %s", job.ID, job.Name, job.Schedule, job.Command, nextStr), nil
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
		sb.WriteString(fmt.Sprintf("  %s  %-12s  %-16s  [%s]  下次: %s\n", j.ID, j.Name, j.Schedule, status, nextStr))
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
	sb.WriteString(fmt.Sprintf("任务详情\n\nID: %s\n名称: %s\n表达式: %s\n命令: %s\n状态: %s\n", job.ID, job.Name, job.Schedule, job.Command, status))
	if job.LastRun != nil {
		sb.WriteString(fmt.Sprintf("上次执行: %s\n", job.LastRun.Format("2006-01-02 15:04:05")))
	}
	if job.NextRun != nil {
		sb.WriteString(fmt.Sprintf("下次执行: %s\n", job.NextRun.Format("2006-01-02 15:04:05")))
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
			sb.WriteString(fmt.Sprintf("  %s  %dms%s  %s\n", l.RunAt.Format("01-02 15:04"), l.DurationMs, errMark, strings.TrimSpace(output)))
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
	for _, key := range []string{"name", "schedule", "command", "enabled"} {
		if v, ok := args[key]; ok {
			updates[key] = v
		}
	}
	job, err := e.scheduler.UpdateJob(id, updates)
	if err != nil {
		return "", err
	}
	status := "启用"
	if !job.Enabled {
		status = "暂停"
	}
	return fmt.Sprintf("任务已更新\nID: %s\n名称: %s\n状态: %s", job.ID, job.Name, status), nil
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
