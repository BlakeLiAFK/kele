package taskboard

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// SessionEvent mirrors daemon.ChatEvent for cross-package usage.
type SessionEvent struct {
	Type       string // content, thinking, tool_call, tool_result, error, done
	Content    string
	ToolName   string
	ToolResult string
	Error      string
}

// TaskSession is the interface a session must satisfy for task execution.
type TaskSession interface {
	GetID() string
	InjectContext(ctx string)
	ChatStream(input string) (<-chan SessionEvent, error)
}

// TaskSessionManager creates and destroys sessions for task execution.
type TaskSessionManager interface {
	CreateTaskSession(name string) TaskSession
	DeleteTaskSession(id string)
}

// Scheduler periodically checks for ready tasks and dispatches them to agent sessions.
type Scheduler struct {
	board    *Board
	sessions TaskSessionManager

	triggerCh chan struct{}
	stopCh    chan struct{}
	doneCh    chan struct{}
}

// NewScheduler creates a scheduler linked to the given board.
func NewScheduler(board *Board, sessions TaskSessionManager) *Scheduler {
	return &Scheduler{
		board:     board,
		sessions:  sessions,
		triggerCh: make(chan struct{}, 1),
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
}

// Start begins the scheduling loop.
func (s *Scheduler) Start() {
	go s.scheduleLoop()
}

// Stop terminates the scheduling loop.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	<-s.doneCh
}

// Trigger wakes up the scheduler to check for ready tasks.
func (s *Scheduler) Trigger() {
	select {
	case s.triggerCh <- struct{}{}:
	default:
	}
}

func (s *Scheduler) scheduleLoop() {
	defer close(s.doneCh)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.triggerCh:
		case <-ticker.C:
		case <-s.stopCh:
			return
		}
		s.runScheduleCycle()
	}
}

func (s *Scheduler) runScheduleCycle() {
	workspaces, err := s.board.Store().ListWorkspaces()
	if err != nil {
		log.Printf("scheduler: list workspaces error: %v", err)
		return
	}

	for _, ws := range workspaces {
		if ws.Status != WorkspaceActive {
			continue
		}

		counts, err := s.board.Store().CountByStatus(ws.ID)
		if err != nil {
			continue
		}

		if counts.Running >= ws.MaxConcurrent {
			continue
		}

		slots := ws.MaxConcurrent - counts.Running
		readyTasks, err := s.board.Store().GetReadyTasks(ws.ID, slots)
		if err != nil {
			continue
		}

		for _, task := range readyTasks {
			s.executeTask(ws, task)
		}
	}
}

func (s *Scheduler) executeTask(ws *Workspace, task *Task) {
	// Build prompt with dependency results injected
	prompt := s.buildTaskPrompt(task)

	// Update task status to running
	task.Status = StatusRunning
	task.StartedAt = time.Now()
	if err := s.board.Store().UpdateTask(task); err != nil {
		log.Printf("scheduler: update task %s error: %v", task.ID, err)
		return
	}
	s.board.broadcast(BoardEvent{
		Type:        EventTaskStarted,
		WorkspaceID: ws.ID,
		TaskID:      task.ID,
		Detail:      task.Title,
		Timestamp:   task.StartedAt,
	})

	// Execute asynchronously
	go func() {
		result, err := s.runTaskSession(ws, task, prompt)
		now := time.Now()

		if err != nil {
			task.Status = StatusFailed
			task.Error = err.Error()
			task.CompletedAt = now
			task.RetryCount++
			if task.RetryCount < task.MaxRetries {
				task.Status = StatusReady
				task.Error = fmt.Sprintf("retry %d: %s", task.RetryCount, err.Error())
				task.CompletedAt = time.Time{}
			}
			s.board.broadcast(BoardEvent{
				Type:        EventTaskFailed,
				WorkspaceID: ws.ID,
				TaskID:      task.ID,
				Detail:      task.Error,
				Timestamp:   now,
			})
		} else {
			task.Status = StatusDone
			task.Result = result
			task.CompletedAt = now
			s.board.broadcast(BoardEvent{
				Type:        EventTaskCompleted,
				WorkspaceID: ws.ID,
				TaskID:      task.ID,
				Detail:      task.Title,
				Timestamp:   now,
			})
		}

		if err := s.board.Store().UpdateTask(task); err != nil {
			log.Printf("scheduler: update task %s after execution error: %v", task.ID, err)
		}

		// Resolve dependencies and check workspace completion
		s.board.OnTaskFinished(ws, task)
		s.Trigger()
	}()
}

// buildTaskPrompt constructs the final prompt for a task, injecting dependency results.
func (s *Scheduler) buildTaskPrompt(task *Task) string {
	var b strings.Builder

	if len(task.DependsOn) > 0 {
		deps, err := s.board.Store().GetTasksByIDs(task.DependsOn)
		if err == nil {
			hasDeps := false
			for _, dep := range deps {
				if dep.Status == StatusDone && dep.Result != "" {
					if !hasDeps {
						b.WriteString("## 前置任务结果\n\n")
						hasDeps = true
					}
					b.WriteString(fmt.Sprintf("### [%s] %s\n", dep.ID, dep.Title))
					b.WriteString(truncateResult(dep.Result, 2000))
					b.WriteString("\n\n")
				}
			}
			if hasDeps {
				b.WriteString("---\n\n")
			}
		}
	}

	b.WriteString(task.Prompt)
	return b.String()
}

// runTaskSession creates a temporary session, runs the task prompt through ChatStream,
// collects the output, and returns the result.
func (s *Scheduler) runTaskSession(ws *Workspace, task *Task, prompt string) (string, error) {
	// Create temporary session
	sess := s.sessions.CreateTaskSession(fmt.Sprintf("task:%s", task.ID))
	defer s.sessions.DeleteTaskSession(sess.GetID())

	// Inject workspace context into the session's system prompt
	if ws.Context != "" {
		sess.InjectContext(ws.Context)
	}

	// Run ChatStream
	eventChan, err := sess.ChatStream(prompt)
	if err != nil {
		return "", fmt.Errorf("chat stream: %w", err)
	}

	var result strings.Builder
	for ev := range eventChan {
		// Log events for task log
		switch ev.Type {
		case "content":
			result.WriteString(ev.Content)
			s.board.Store().AppendTaskLog(task.ID, "content", ev.Content, "")
		case "thinking":
			s.board.Store().AppendTaskLog(task.ID, "thinking", ev.Content, "")
		case "tool_call":
			s.board.Store().AppendTaskLog(task.ID, "tool_call", "", ev.ToolName)
		case "tool_result":
			s.board.Store().AppendTaskLog(task.ID, "tool_result", ev.ToolResult, ev.ToolName)
		case "error":
			if ev.Error != "" {
				return "", fmt.Errorf("llm error: %s", ev.Error)
			}
		}
	}

	return result.String(), nil
}

// truncateResult trims a string to maxLen, preserving head.
func truncateResult(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + fmt.Sprintf("\n... [截断, 原始 %d 字节]", len(s))
}
