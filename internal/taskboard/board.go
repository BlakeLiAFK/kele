package taskboard

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// Board is the central orchestrator for workspaces and tasks.
// It owns the store, manages the event bus, and coordinates with the scheduler.
type Board struct {
	store     *TaskStore
	scheduler *Scheduler
	eventBus  *eventBus

	mu sync.RWMutex
}

// NewBoard creates a new Board with the given store.
// Call SetScheduler after creating the Scheduler to link them.
func NewBoard(store *TaskStore) *Board {
	return &Board{
		store:    store,
		eventBus: newEventBus(),
	}
}

// SetScheduler links the board to its scheduler (breaks circular dependency).
func (b *Board) SetScheduler(s *Scheduler) {
	b.scheduler = s
}

// Store returns the underlying TaskStore.
func (b *Board) Store() *TaskStore {
	return b.store
}

// --- Workspace operations ---

func (b *Board) CreateWorkspace(ws *Workspace) error {
	if ws.ID == "" {
		ws.ID = fmt.Sprintf("ws-%d", time.Now().UnixNano())
	}
	if ws.Status == "" {
		ws.Status = WorkspaceActive
	}
	if ws.MaxConcurrent <= 0 {
		ws.MaxConcurrent = 3
	}
	now := time.Now()
	ws.CreatedAt = now
	ws.UpdatedAt = now
	if err := b.store.CreateWorkspace(ws); err != nil {
		return err
	}
	b.broadcast(BoardEvent{
		Type:        EventWorkspaceCreated,
		WorkspaceID: ws.ID,
		Detail:      ws.Name,
		Timestamp:   now,
	})
	return nil
}

func (b *Board) GetWorkspace(id string) (*Workspace, error) {
	return b.store.GetWorkspace(id)
}

func (b *Board) ListWorkspaces() ([]*Workspace, error) {
	return b.store.ListWorkspaces()
}

func (b *Board) PauseWorkspace(id string) error {
	ws, err := b.store.GetWorkspace(id)
	if err != nil {
		return err
	}
	ws.Status = WorkspacePaused
	if err := b.store.UpdateWorkspace(ws); err != nil {
		return err
	}
	b.broadcast(BoardEvent{
		Type:        EventWorkspacePaused,
		WorkspaceID: id,
		Detail:      ws.Name,
		Timestamp:   time.Now(),
	})
	return nil
}

func (b *Board) ResumeWorkspace(id string) error {
	ws, err := b.store.GetWorkspace(id)
	if err != nil {
		return err
	}
	ws.Status = WorkspaceActive
	if err := b.store.UpdateWorkspace(ws); err != nil {
		return err
	}
	b.broadcast(BoardEvent{
		Type:        EventWorkspaceResumed,
		WorkspaceID: id,
		Detail:      ws.Name,
		Timestamp:   time.Now(),
	})
	if b.scheduler != nil {
		b.scheduler.Trigger()
	}
	return nil
}

func (b *Board) DeleteWorkspace(id string) error {
	return b.store.DeleteWorkspace(id)
}

func (b *Board) UpdateWorkspace(ws *Workspace) error {
	return b.store.UpdateWorkspace(ws)
}

// --- Task operations ---

func (b *Board) CreateTask(t *Task) error {
	if t.ID == "" {
		t.ID = fmt.Sprintf("t-%d", time.Now().UnixNano())
	}
	if t.Status == "" {
		t.Status = StatusBacklog
	}
	if t.MaxRetries <= 0 {
		t.MaxRetries = 1
	}
	if t.Tags == nil {
		t.Tags = []string{}
	}
	if t.DependsOn == nil {
		t.DependsOn = []string{}
	}
	t.CreatedAt = time.Now()
	if err := b.store.CreateTask(t); err != nil {
		return err
	}
	b.broadcast(BoardEvent{
		Type:        EventTaskCreated,
		WorkspaceID: t.WorkspaceID,
		TaskID:      t.ID,
		Detail:      t.Title,
		Timestamp:   t.CreatedAt,
	})
	// If task is ready, trigger scheduler
	if t.Status == StatusReady && b.scheduler != nil {
		b.scheduler.Trigger()
	}
	return nil
}

func (b *Board) GetTask(id string) (*Task, error) {
	return b.store.GetTask(id)
}

func (b *Board) UpdateTask(t *Task) error {
	return b.store.UpdateTask(t)
}

func (b *Board) DeleteTask(id string) error {
	return b.store.DeleteTask(id)
}

func (b *Board) ListTasks(workspaceID, statusFilter string) ([]*Task, error) {
	return b.store.ListTasks(workspaceID, statusFilter)
}

// StartTask manually starts a single task (bypasses scheduler).
func (b *Board) StartTask(id string) (*Task, error) {
	t, err := b.store.GetTask(id)
	if err != nil {
		return nil, err
	}
	if t.Status != StatusReady && t.Status != StatusBacklog {
		return nil, fmt.Errorf("task %s is in %s state, cannot start", id, t.Status)
	}
	t.Status = StatusReady
	if err := b.store.UpdateTask(t); err != nil {
		return nil, err
	}
	if b.scheduler != nil {
		b.scheduler.Trigger()
	}
	return t, nil
}

// CancelTask cancels a task.
func (b *Board) CancelTask(id string) (*Task, error) {
	t, err := b.store.GetTask(id)
	if err != nil {
		return nil, err
	}
	if t.Status.IsTerminal() {
		return nil, fmt.Errorf("task %s is already in terminal state %s", id, t.Status)
	}
	t.Status = StatusCancelled
	t.CompletedAt = time.Now()
	if err := b.store.UpdateTask(t); err != nil {
		return nil, err
	}
	b.broadcast(BoardEvent{
		Type:        EventTaskCancelled,
		WorkspaceID: t.WorkspaceID,
		TaskID:      t.ID,
		Detail:      t.Title,
		Timestamp:   t.CompletedAt,
	})
	return t, nil
}

// RetryTask resets a failed task back to ready.
func (b *Board) RetryTask(id string) (*Task, error) {
	t, err := b.store.GetTask(id)
	if err != nil {
		return nil, err
	}
	if t.Status != StatusFailed {
		return nil, fmt.Errorf("task %s is in %s state, can only retry failed tasks", id, t.Status)
	}
	t.Status = StatusReady
	t.Error = ""
	t.AssignedSession = ""
	t.StartedAt = time.Time{}
	t.CompletedAt = time.Time{}
	if err := b.store.UpdateTask(t); err != nil {
		return nil, err
	}
	if b.scheduler != nil {
		b.scheduler.Trigger()
	}
	return t, nil
}

// --- Board overview ---

// GetOverview returns an aggregated view of all workspaces.
func (b *Board) GetOverview() (*BoardOverview, error) {
	workspaces, err := b.store.ListWorkspaces()
	if err != nil {
		return nil, err
	}
	overview := &BoardOverview{}
	for _, ws := range workspaces {
		counts, err := b.store.CountByStatus(ws.ID)
		if err != nil {
			continue
		}
		wo := WorkspaceOverview{
			ID:            ws.ID,
			Name:          ws.Name,
			Status:        ws.Status,
			Backlog:       counts.Backlog,
			Ready:         counts.Ready,
			Running:       counts.Running,
			Done:          counts.Done,
			Failed:        counts.Failed,
			MaxConcurrent: ws.MaxConcurrent,
		}
		overview.Workspaces = append(overview.Workspaces, wo)
		overview.TotalTasks += counts.Total()
		overview.RunningTasks += counts.Running
		overview.PendingTasks += counts.Backlog + counts.Ready
		overview.CompletedTasks += counts.Done
	}
	return overview, nil
}

// BoardOverview is an aggregated view of all workspaces.
type BoardOverview struct {
	Workspaces     []WorkspaceOverview
	TotalTasks     int
	RunningTasks   int
	PendingTasks   int
	CompletedTasks int
}

// WorkspaceOverview is a summary of a single workspace.
type WorkspaceOverview struct {
	ID            string
	Name          string
	Status        WorkspaceStatus
	Backlog       int
	Ready         int
	Running       int
	Done          int
	Failed        int
	MaxConcurrent int
}

// --- Dependency resolution + completion detection ---

// OnTaskFinished is called when a task reaches a terminal state.
// It promotes dependent tasks and checks if the workspace is complete.
func (b *Board) OnTaskFinished(ws *Workspace, task *Task) {
	if task.Status == StatusDone {
		b.resolveDependencies(task)
	}
	// Check if workspace is fully complete
	counts, err := b.store.CountByStatus(ws.ID)
	if err != nil {
		log.Printf("board: count by status error: %v", err)
		return
	}
	if counts.AllDone() {
		b.broadcast(BoardEvent{
			Type:        EventWorkspaceCompleted,
			WorkspaceID: ws.ID,
			Detail:      fmt.Sprintf("%d tasks completed", counts.Done),
			Timestamp:   time.Now(),
		})
	}
}

// resolveDependencies checks tasks that depend on the completed task
// and promotes them to ready if all their dependencies are satisfied.
func (b *Board) resolveDependencies(completedTask *Task) {
	dependents, err := b.store.GetDependents(completedTask.ID)
	if err != nil {
		log.Printf("board: get dependents error: %v", err)
		return
	}
	for _, dep := range dependents {
		if dep.Status != StatusBacklog {
			continue
		}
		// Check if ALL dependencies of this task are now done
		allDone := true
		if len(dep.DependsOn) > 0 {
			depTasks, err := b.store.GetTasksByIDs(dep.DependsOn)
			if err != nil {
				continue
			}
			for _, dt := range depTasks {
				if dt.Status != StatusDone {
					allDone = false
					break
				}
			}
		}
		if allDone {
			dep.Status = StatusReady
			if err := b.store.UpdateTask(dep); err != nil {
				log.Printf("board: promote task %s to ready error: %v", dep.ID, err)
				continue
			}
			b.broadcast(BoardEvent{
				Type:        EventTaskReady,
				WorkspaceID: dep.WorkspaceID,
				TaskID:      dep.ID,
				Detail:      dep.Title,
				Timestamp:   time.Now(),
			})
		}
	}
}

// --- Event bus ---

// Subscribe returns a channel that receives board events.
// The caller must call Unsubscribe with the returned ID when done.
func (b *Board) Subscribe() (int, <-chan BoardEvent) {
	return b.eventBus.subscribe()
}

// Unsubscribe removes a subscriber.
func (b *Board) Unsubscribe(id int) {
	b.eventBus.unsubscribe(id)
}

func (b *Board) broadcast(event BoardEvent) {
	b.eventBus.broadcast(event)
}

// eventBus fans out events to all subscribers.
type eventBus struct {
	subs    map[int]chan BoardEvent
	counter int
	mu      sync.RWMutex
}

func newEventBus() *eventBus {
	return &eventBus{
		subs: make(map[int]chan BoardEvent),
	}
}

func (eb *eventBus) subscribe() (int, <-chan BoardEvent) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.counter++
	ch := make(chan BoardEvent, 64)
	eb.subs[eb.counter] = ch
	return eb.counter, ch
}

func (eb *eventBus) unsubscribe(id int) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	if ch, ok := eb.subs[id]; ok {
		close(ch)
		delete(eb.subs, id)
	}
}

func (eb *eventBus) broadcast(event BoardEvent) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	for _, ch := range eb.subs {
		select {
		case ch <- event:
		default:
			// Drop event if subscriber is slow
		}
	}
}
