package taskboard

import (
	"fmt"
	"time"
)

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

const (
	StatusBacklog   TaskStatus = "backlog"
	StatusReady     TaskStatus = "ready"
	StatusRunning   TaskStatus = "running"
	StatusDone      TaskStatus = "done"
	StatusFailed    TaskStatus = "failed"
	StatusCancelled TaskStatus = "cancelled"
)

// ValidTransition checks if a task status transition is allowed.
func (s TaskStatus) ValidTransition(to TaskStatus) bool {
	switch s {
	case StatusBacklog:
		return to == StatusReady || to == StatusCancelled
	case StatusReady:
		return to == StatusRunning || to == StatusBacklog || to == StatusCancelled
	case StatusRunning:
		return to == StatusDone || to == StatusFailed || to == StatusCancelled
	case StatusFailed:
		return to == StatusReady || to == StatusCancelled // retry or cancel
	case StatusDone, StatusCancelled:
		return false
	}
	return false
}

// IsTerminal returns true if the status is a final state.
func (s TaskStatus) IsTerminal() bool {
	return s == StatusDone || s == StatusCancelled
}

// WorkspaceStatus represents the lifecycle state of a workspace.
type WorkspaceStatus string

const (
	WorkspaceActive   WorkspaceStatus = "active"
	WorkspacePaused   WorkspaceStatus = "paused"
	WorkspaceArchived WorkspaceStatus = "archived"
)

// Workspace groups related tasks with shared context and concurrency control.
type Workspace struct {
	ID             string
	Name           string
	Description    string
	Goal           string // user's original goal (vague statement)
	Status         WorkspaceStatus
	MaxConcurrent  int
	Context        string // system prompt injected into all task sessions (Planner-generated)
	WorkDir        string
	Summary        string // Synthesizer-generated completion report
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Task is a single unit of work within a workspace.
type Task struct {
	ID              string
	WorkspaceID     string
	Title           string
	Description     string
	Prompt          string
	Status          TaskStatus
	Priority        int // 0=critical, 1=high, 2=normal, 3=low
	AssignedSession string
	Result          string
	Error           string
	MaxRetries      int
	RetryCount      int
	Tags            []string
	DependsOn       []string // task IDs
	CreatedAt       time.Time
	StartedAt       time.Time
	CompletedAt     time.Time
}

// TaskLog records a single event during task execution.
type TaskLog struct {
	ID        int64
	TaskID    string
	EventType string // content, tool_call, tool_result, error, thinking
	Content   string
	ToolName  string
	Timestamp time.Time
}

// EventType constants for board events.
const (
	EventTaskCreated        = "task_created"
	EventTaskReady          = "task_ready"
	EventTaskStarted        = "task_started"
	EventTaskCompleted      = "task_completed"
	EventTaskFailed         = "task_failed"
	EventTaskCancelled      = "task_cancelled"
	EventWorkspaceCreated   = "workspace_created"
	EventWorkspacePaused    = "workspace_paused"
	EventWorkspaceResumed   = "workspace_resumed"
	EventWorkspaceCompleted = "workspace_completed"
)

// BoardEvent is a state change notification broadcast to subscribers.
type BoardEvent struct {
	Type        string
	WorkspaceID string
	TaskID      string
	Detail      string
	Timestamp   time.Time
}

// StatusCounts holds per-workspace task status counts.
type StatusCounts struct {
	Backlog   int
	Ready     int
	Running   int
	Done      int
	Failed    int
	Cancelled int
}

// Total returns the total number of tasks.
func (c StatusCounts) Total() int {
	return c.Backlog + c.Ready + c.Running + c.Done + c.Failed + c.Cancelled
}

// AllDone returns true if all non-cancelled tasks are done.
func (c StatusCounts) AllDone() bool {
	return c.Backlog == 0 && c.Ready == 0 && c.Running == 0 && c.Failed == 0 && c.Done > 0
}

// PlanResult is the structured output from the Planner AI agent.
type PlanResult struct {
	WorkspaceName    string        `json:"workspace_name"`
	WorkspaceContext string        `json:"workspace_context"`
	MaxConcurrent    int           `json:"max_concurrent,omitempty"`
	Tasks            []PlannedTask `json:"tasks"`
}

// PlannedTask is a single task within a plan.
type PlannedTask struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Prompt      string   `json:"prompt"`
	Priority    int      `json:"priority"`
	DependsOn   []int    `json:"depends_on"` // indices into Tasks slice
	Tags        []string `json:"tags,omitempty"`
}

// Validate checks the PlanResult for basic correctness.
func (p *PlanResult) Validate() error {
	if p.WorkspaceName == "" {
		return fmt.Errorf("workspace_name is required")
	}
	if len(p.Tasks) == 0 {
		return fmt.Errorf("at least one task is required")
	}
	for i, t := range p.Tasks {
		if t.Title == "" {
			return fmt.Errorf("task %d: title is required", i)
		}
		if t.Prompt == "" {
			return fmt.Errorf("task %d: prompt is required", i)
		}
		for _, dep := range t.DependsOn {
			if dep < 0 || dep >= len(p.Tasks) {
				return fmt.Errorf("task %d: invalid dependency index %d", i, dep)
			}
			if dep == i {
				return fmt.Errorf("task %d: cannot depend on itself", i)
			}
		}
	}
	return nil
}
