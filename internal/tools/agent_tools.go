package tools

import (
	"fmt"
	"strings"
	"time"
)

// --- spawn_agent 工具 ---

// SpawnAgentTool 启动子 agent 执行任务
type SpawnAgentTool struct {
	spawner AgentSpawner
}

func NewSpawnAgentTool(spawner AgentSpawner) *SpawnAgentTool {
	return &SpawnAgentTool{spawner: spawner}
}

func (t *SpawnAgentTool) Name() string { return "spawn_agent" }
func (t *SpawnAgentTool) Description() string {
	return "启动一个子 agent 在后台独立执行任务。子 agent 拥有相同的工具集，会自动执行工具调用直到完成。适合将大任务拆分成多个子任务并行执行。"
}
func (t *SpawnAgentTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "子 agent 要执行的任务描述，要具体明确，包含所有必要上下文",
			},
		},
		"required": []string{"task"},
	}
}

func (t *SpawnAgentTool) Execute(args map[string]interface{}) (string, error) {
	task, ok := args["task"].(string)
	if !ok || task == "" {
		return "", fmt.Errorf("缺少 task 参数")
	}

	id, err := t.spawner.Spawn(task)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("子 agent 已启动\nID: %s\n任务: %s\n\n使用 agent_status 查看进度，agent_result 获取结果", id, task), nil
}

// --- agent_status 工具 ---

// AgentStatusTool 查看子 agent 状态
type AgentStatusTool struct {
	spawner AgentSpawner
}

func NewAgentStatusTool(spawner AgentSpawner) *AgentStatusTool {
	return &AgentStatusTool{spawner: spawner}
}

func (t *AgentStatusTool) Name() string { return "agent_status" }
func (t *AgentStatusTool) Description() string {
	return "查看子 agent 的执行状态和最近日志。不传 id 则列出所有子 agent。"
}
func (t *AgentStatusTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "子 agent ID（可选，不传则列出全部）",
			},
		},
	}
}

func (t *AgentStatusTool) Execute(args map[string]interface{}) (string, error) {
	id, _ := args["id"].(string)

	if id == "" {
		all := t.spawner.ListAll()
		if len(all) == 0 {
			return "暂无子 agent", nil
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "子 Agent 列表 (%d 个)\n\n", len(all))
		for _, info := range all {
			elapsed := ""
			if !info.StartTime.IsZero() {
				if info.EndTime.IsZero() {
					elapsed = time.Since(info.StartTime).Round(time.Second).String()
				} else {
					elapsed = info.EndTime.Sub(info.StartTime).Round(time.Millisecond).String()
				}
			}
			fmt.Fprintf(&sb, "  %s  [%s]  %s  %s\n", info.ID, info.Status, elapsed, truncateStr(info.Task, 60))
		}
		return sb.String(), nil
	}

	info, err := t.spawner.Status(id)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Agent %s\n", info.ID)
	fmt.Fprintf(&sb, "状态: %s\n", info.Status)
	fmt.Fprintf(&sb, "任务: %s\n", info.Task)
	if !info.StartTime.IsZero() {
		fmt.Fprintf(&sb, "开始: %s\n", info.StartTime.Format("15:04:05"))
	}
	if !info.EndTime.IsZero() {
		fmt.Fprintf(&sb, "结束: %s\n", info.EndTime.Format("15:04:05"))
		fmt.Fprintf(&sb, "耗时: %v\n", info.EndTime.Sub(info.StartTime).Round(time.Millisecond))
	}
	if info.Error != "" {
		fmt.Fprintf(&sb, "错误: %s\n", info.Error)
	}

	logs, _ := t.spawner.RecentLogs(id, 10)
	if len(logs) > 0 {
		sb.WriteString("\n最近日志:\n")
		for _, l := range logs {
			fmt.Fprintf(&sb, "  [%s] %s: %s\n", l.Time.Format("15:04:05"), l.Type, truncateStr(l.Content, 100))
		}
	}

	return sb.String(), nil
}

// --- agent_result 工具 ---

// AgentResultTool 等待并获取子 agent 结果
type AgentResultTool struct {
	spawner AgentSpawner
}

func NewAgentResultTool(spawner AgentSpawner) *AgentResultTool {
	return &AgentResultTool{spawner: spawner}
}

func (t *AgentResultTool) Name() string { return "agent_result" }
func (t *AgentResultTool) Description() string {
	return "等待子 agent 完成并获取结果。会阻塞直到 agent 完成或超时（5 分钟）。"
}
func (t *AgentResultTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "子 agent ID",
			},
		},
		"required": []string{"id"},
	}
}

func (t *AgentResultTool) Execute(args map[string]interface{}) (string, error) {
	id, ok := args["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("缺少 id 参数")
	}

	result, err := t.spawner.Result(id, 5*time.Minute)
	if err != nil {
		return "", err
	}

	if result == "" {
		return fmt.Sprintf("Agent %s 已完成，但无输出内容", id), nil
	}
	return fmt.Sprintf("Agent %s 结果:\n\n%s", id, result), nil
}

func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
