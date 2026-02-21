package taskboard

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// PlanEvent is a streaming event emitted by the Planner during goal decomposition.
type PlanEvent struct {
	Type     string // thinking, reading, plan_ready, error
	Content  string // progress info for thinking/reading/error
	PlanJSON string // populated when type=plan_ready
}

// Planner decomposes a vague user goal into structured tasks using an AI agent.
type Planner struct {
	sessions TaskSessionManager
}

// NewPlanner creates a new Planner.
func NewPlanner(sessions TaskSessionManager) *Planner {
	return &Planner{sessions: sessions}
}

const plannerPrompt = `你是一个任务规划专家。用户给出了一个目标，你需要：

1. 仔细分析用户的目标，理解需要做什么
2. 将用户目标分解为具体的、可独立执行的子任务
3. 每个子任务应该是一个 AI agent 能独立完成的工作单元（通常是一个文件/模块级别的改动）
4. 明确任务间的依赖关系（哪些必须先完成才能开始后续任务）
5. 按优先级排序（0=critical, 1=high, 2=normal, 3=low）
6. 每个任务的 prompt 要足够具体和详细，包含文件路径、实现要求、设计约束等

用户目标: %s

请直接输出 JSON 格式的任务计划（不要用 markdown 代码块包裹），格式如下:
{
  "workspace_name": "简短的工作区名称（英文，用短横线分隔）",
  "workspace_context": "给所有执行 agent 的共享上下文（项目背景、技术栈、代码风格等）",
  "max_concurrent": 2,
  "tasks": [
    {
      "title": "简短标题",
      "description": "人读的描述",
      "prompt": "给执行 agent 的完整、详细的 prompt",
      "priority": 2,
      "depends_on": [],
      "tags": ["backend"]
    }
  ]
}`

// Plan runs the AI planner to decompose a goal into tasks.
// Returns a channel of PlanEvents for streaming progress.
func (p *Planner) Plan(goal string) (<-chan PlanEvent, error) {
	if goal == "" {
		return nil, fmt.Errorf("goal is required")
	}

	eventCh := make(chan PlanEvent, 32)

	go func() {
		defer close(eventCh)

		eventCh <- PlanEvent{Type: "thinking", Content: "正在分析目标并生成任务计划..."}

		// Create a temporary session for planning
		sess := p.sessions.CreateTaskSession("planner")
		defer p.sessions.DeleteTaskSession(sess.GetID())

		prompt := fmt.Sprintf(plannerPrompt, goal)
		chatEvents, err := sess.ChatStream(prompt)
		if err != nil {
			eventCh <- PlanEvent{Type: "error", Content: fmt.Sprintf("chat error: %v", err)}
			return
		}

		var fullContent strings.Builder
		for ev := range chatEvents {
			switch ev.Type {
			case "content":
				fullContent.WriteString(ev.Content)
			case "thinking":
				eventCh <- PlanEvent{Type: "thinking", Content: ev.Content}
			case "tool_call":
				eventCh <- PlanEvent{Type: "reading", Content: fmt.Sprintf("调用工具: %s", ev.ToolName)}
			case "error":
				eventCh <- PlanEvent{Type: "error", Content: ev.Error}
				return
			}
		}

		content := fullContent.String()
		// Try to extract JSON from the content
		planJSON := extractJSON(content)
		if planJSON == "" {
			eventCh <- PlanEvent{Type: "error", Content: "AI 未返回有效的 JSON 计划"}
			return
		}

		// Validate
		var plan PlanResult
		if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
			eventCh <- PlanEvent{Type: "error", Content: fmt.Sprintf("JSON 解析失败: %v", err)}
			return
		}
		if err := plan.Validate(); err != nil {
			eventCh <- PlanEvent{Type: "error", Content: fmt.Sprintf("计划验证失败: %v", err)}
			return
		}

		eventCh <- PlanEvent{Type: "plan_ready", PlanJSON: planJSON}
	}()

	return eventCh, nil
}

// ParsePlan parses a JSON string into a PlanResult.
func ParsePlan(jsonStr string) (*PlanResult, error) {
	var plan PlanResult
	if err := json.Unmarshal([]byte(jsonStr), &plan); err != nil {
		return nil, fmt.Errorf("parse plan: %w", err)
	}
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	return &plan, nil
}

// ApproveAndCreate creates a workspace and tasks from a plan.
func (p *Planner) ApproveAndCreate(board *Board, plan *PlanResult, goal, workDir string, autoStart bool) (*Workspace, []*Task, error) {
	wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
	ws, tasks, err := board.Store().CreateFromPlan(plan, wsID, goal, workDir)
	if err != nil {
		return nil, nil, err
	}

	// Broadcast creation events
	board.broadcast(BoardEvent{
		Type:        EventWorkspaceCreated,
		WorkspaceID: ws.ID,
		Detail:      fmt.Sprintf("%s (%d tasks)", ws.Name, len(tasks)),
		Timestamp:   time.Now(),
	})
	for _, t := range tasks {
		evType := EventTaskCreated
		if t.Status == StatusReady {
			evType = EventTaskReady
		}
		board.broadcast(BoardEvent{
			Type:        evType,
			WorkspaceID: ws.ID,
			TaskID:      t.ID,
			Detail:      t.Title,
			Timestamp:   time.Now(),
		})
	}

	if autoStart && board.scheduler != nil {
		board.scheduler.Trigger()
	}

	return ws, tasks, nil
}

// Synthesize generates a summary report for a completed workspace.
func (p *Planner) Synthesize(board *Board, ws *Workspace) (string, error) {
	tasks, err := board.Store().ListTasks(ws.ID, string(StatusDone))
	if err != nil {
		return "", fmt.Errorf("list done tasks: %w", err)
	}
	if len(tasks) == 0 {
		return "没有已完成的任务。", nil
	}

	// Build synthesis prompt
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("以下是工作区 \"%s\" 中所有已完成任务的结果。\n", ws.Name))
	prompt.WriteString("请生成一份简洁的完成报告，包括：\n")
	prompt.WriteString("1. 完成了什么\n")
	prompt.WriteString("2. 修改/创建了哪些文件\n")
	prompt.WriteString("3. 是否有需要注意的问题\n\n")

	for _, t := range tasks {
		prompt.WriteString(fmt.Sprintf("### %s\n", t.Title))
		result := t.Result
		if len(result) > 1500 {
			result = result[:1500] + "..."
		}
		prompt.WriteString(result)
		prompt.WriteString("\n\n")
	}

	// Create temporary session for synthesis
	sess := p.sessions.CreateTaskSession("synthesizer")
	defer p.sessions.DeleteTaskSession(sess.GetID())

	eventChan, err := sess.ChatStream(prompt.String())
	if err != nil {
		return "", fmt.Errorf("synthesis chat: %w", err)
	}

	var result strings.Builder
	for ev := range eventChan {
		if ev.Type == "content" {
			result.WriteString(ev.Content)
		}
	}

	summary := result.String()
	ws.Summary = summary
	if err := board.Store().UpdateWorkspace(ws); err != nil {
		log.Printf("planner: update workspace summary error: %v", err)
	}

	return summary, nil
}

// extractJSON attempts to find a JSON object in a string (possibly surrounded by text/markdown).
func extractJSON(s string) string {
	// Try the whole string first
	s = strings.TrimSpace(s)
	if json.Valid([]byte(s)) {
		return s
	}

	// Try to find JSON between ```json and ``` markers
	if idx := strings.Index(s, "```json"); idx >= 0 {
		start := idx + 7
		if end := strings.Index(s[start:], "```"); end >= 0 {
			candidate := strings.TrimSpace(s[start : start+end])
			if json.Valid([]byte(candidate)) {
				return candidate
			}
		}
	}
	// Try ``` markers without json label
	if idx := strings.Index(s, "```"); idx >= 0 {
		start := idx + 3
		// Skip optional newline after ```
		if start < len(s) && s[start] == '\n' {
			start++
		}
		if end := strings.Index(s[start:], "```"); end >= 0 {
			candidate := strings.TrimSpace(s[start : start+end])
			if json.Valid([]byte(candidate)) {
				return candidate
			}
		}
	}

	// Try to find { ... } bounds
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	// Find matching closing brace from end
	end := strings.LastIndex(s, "}")
	if end <= start {
		return ""
	}
	candidate := s[start : end+1]
	if json.Valid([]byte(candidate)) {
		return candidate
	}

	return ""
}
