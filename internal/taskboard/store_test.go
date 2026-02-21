package taskboard

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempDB(t *testing.T) (*TaskStore, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "taskboard-test-*")
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "test.db")
	store, err := NewTaskStore(dbPath)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatal(err)
	}
	return store, func() {
		store.Close()
		os.RemoveAll(dir)
	}
}

func TestWorkspaceCRUD(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	now := time.Now()
	ws := &Workspace{
		ID:            "ws-1",
		Name:          "test-workspace",
		Description:   "test desc",
		Goal:          "test goal",
		Status:        WorkspaceActive,
		MaxConcurrent: 3,
		Context:       "test context",
		WorkDir:       "/tmp",
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// Create
	if err := store.CreateWorkspace(ws); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	// Get
	got, err := store.GetWorkspace("ws-1")
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if got.Name != "test-workspace" {
		t.Errorf("Name = %s, want test-workspace", got.Name)
	}
	if got.Goal != "test goal" {
		t.Errorf("Goal = %s, want test goal", got.Goal)
	}
	if got.Status != WorkspaceActive {
		t.Errorf("Status = %s, want active", got.Status)
	}

	// Update
	got.Name = "updated"
	got.Summary = "done report"
	if err := store.UpdateWorkspace(got); err != nil {
		t.Fatalf("UpdateWorkspace: %v", err)
	}
	got2, _ := store.GetWorkspace("ws-1")
	if got2.Name != "updated" {
		t.Errorf("Updated Name = %s, want updated", got2.Name)
	}
	if got2.Summary != "done report" {
		t.Errorf("Summary = %s, want done report", got2.Summary)
	}

	// List
	list, err := store.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("ListWorkspaces len = %d, want 1", len(list))
	}

	// Delete
	if err := store.DeleteWorkspace("ws-1"); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
	list, _ = store.ListWorkspaces()
	if len(list) != 0 {
		t.Errorf("After delete, len = %d, want 0", len(list))
	}
}

func TestTaskCRUD(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	now := time.Now()
	store.CreateWorkspace(&Workspace{
		ID: "ws-1", Name: "test", Status: WorkspaceActive,
		MaxConcurrent: 3, CreatedAt: now, UpdatedAt: now,
	})

	task := &Task{
		ID:          "t-1",
		WorkspaceID: "ws-1",
		Title:       "task one",
		Prompt:      "do something",
		Status:      StatusBacklog,
		Priority:    1,
		Tags:        []string{"backend"},
		DependsOn:   []string{},
		MaxRetries:  2,
		CreatedAt:   now,
	}

	// Create
	if err := store.CreateTask(task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Get
	got, err := store.GetTask("t-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Title != "task one" {
		t.Errorf("Title = %s, want task one", got.Title)
	}
	if got.Priority != 1 {
		t.Errorf("Priority = %d, want 1", got.Priority)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "backend" {
		t.Errorf("Tags = %v, want [backend]", got.Tags)
	}

	// Update
	got.Status = StatusReady
	got.Result = "done!"
	if err := store.UpdateTask(got); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	got2, _ := store.GetTask("t-1")
	if got2.Status != StatusReady {
		t.Errorf("Status = %s, want ready", got2.Status)
	}

	// List
	tasks, err := store.ListTasks("ws-1", "")
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("ListTasks len = %d, want 1", len(tasks))
	}

	// List with filter
	tasks, _ = store.ListTasks("ws-1", "running")
	if len(tasks) != 0 {
		t.Errorf("ListTasks running len = %d, want 0", len(tasks))
	}

	// Delete
	if err := store.DeleteTask("t-1"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	tasks, _ = store.ListTasks("ws-1", "")
	if len(tasks) != 0 {
		t.Errorf("After delete, len = %d, want 0", len(tasks))
	}
}

func TestCountByStatus(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	now := time.Now()
	store.CreateWorkspace(&Workspace{
		ID: "ws-1", Name: "test", Status: WorkspaceActive,
		MaxConcurrent: 3, CreatedAt: now, UpdatedAt: now,
	})

	for i, status := range []TaskStatus{StatusBacklog, StatusReady, StatusReady, StatusRunning, StatusDone, StatusDone, StatusDone} {
		store.CreateTask(&Task{
			ID: fmt.Sprintf("t-%d", i), WorkspaceID: "ws-1",
			Title: fmt.Sprintf("task %d", i), Prompt: "p",
			Status: status, Tags: []string{}, DependsOn: []string{}, CreatedAt: now,
		})
	}

	counts, err := store.CountByStatus("ws-1")
	if err != nil {
		t.Fatalf("CountByStatus: %v", err)
	}
	if counts.Backlog != 1 {
		t.Errorf("Backlog = %d, want 1", counts.Backlog)
	}
	if counts.Ready != 2 {
		t.Errorf("Ready = %d, want 2", counts.Ready)
	}
	if counts.Running != 1 {
		t.Errorf("Running = %d, want 1", counts.Running)
	}
	if counts.Done != 3 {
		t.Errorf("Done = %d, want 3", counts.Done)
	}
	if counts.Total() != 7 {
		t.Errorf("Total = %d, want 7", counts.Total())
	}
}

func TestGetDependents(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	now := time.Now()
	store.CreateWorkspace(&Workspace{
		ID: "ws-1", Name: "test", Status: WorkspaceActive,
		MaxConcurrent: 3, CreatedAt: now, UpdatedAt: now,
	})

	store.CreateTask(&Task{ID: "t-1", WorkspaceID: "ws-1", Title: "base", Prompt: "p",
		Status: StatusDone, Tags: []string{}, DependsOn: []string{}, CreatedAt: now})
	store.CreateTask(&Task{ID: "t-2", WorkspaceID: "ws-1", Title: "depends on t-1", Prompt: "p",
		Status: StatusBacklog, Tags: []string{}, DependsOn: []string{"t-1"}, CreatedAt: now})
	store.CreateTask(&Task{ID: "t-3", WorkspaceID: "ws-1", Title: "also depends on t-1", Prompt: "p",
		Status: StatusBacklog, Tags: []string{}, DependsOn: []string{"t-1"}, CreatedAt: now})
	store.CreateTask(&Task{ID: "t-4", WorkspaceID: "ws-1", Title: "no deps", Prompt: "p",
		Status: StatusReady, Tags: []string{}, DependsOn: []string{}, CreatedAt: now})

	deps, err := store.GetDependents("t-1")
	if err != nil {
		t.Fatalf("GetDependents: %v", err)
	}
	if len(deps) != 2 {
		t.Errorf("GetDependents len = %d, want 2", len(deps))
	}
}

func TestCreateFromPlan(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	plan := &PlanResult{
		WorkspaceName:    "test-plan",
		WorkspaceContext: "context here",
		MaxConcurrent:    2,
		Tasks: []PlannedTask{
			{Title: "types", Prompt: "create types", Priority: 0, DependsOn: []int{}},
			{Title: "store", Prompt: "create store", Priority: 1, DependsOn: []int{0}},
			{Title: "board", Prompt: "create board", Priority: 1, DependsOn: []int{0}},
			{Title: "cli", Prompt: "create cli", Priority: 2, DependsOn: []int{1, 2}},
		},
	}

	ws, tasks, err := store.CreateFromPlan(plan, "ws-plan-1", "build taskboard", "/tmp")
	if err != nil {
		t.Fatalf("CreateFromPlan: %v", err)
	}

	if ws.Name != "test-plan" {
		t.Errorf("ws.Name = %s, want test-plan", ws.Name)
	}
	if ws.Goal != "build taskboard" {
		t.Errorf("ws.Goal = %s, want build taskboard", ws.Goal)
	}
	if ws.MaxConcurrent != 2 {
		t.Errorf("ws.MaxConcurrent = %d, want 2", ws.MaxConcurrent)
	}

	if len(tasks) != 4 {
		t.Fatalf("tasks len = %d, want 4", len(tasks))
	}

	// Task 0 (no deps) should be ready
	if tasks[0].Status != StatusReady {
		t.Errorf("task[0] status = %s, want ready", tasks[0].Status)
	}

	// Task 1 (depends on 0) should be backlog
	if tasks[1].Status != StatusBacklog {
		t.Errorf("task[1] status = %s, want backlog", tasks[1].Status)
	}
	if len(tasks[1].DependsOn) != 1 || tasks[1].DependsOn[0] != tasks[0].ID {
		t.Errorf("task[1] depends_on = %v, want [%s]", tasks[1].DependsOn, tasks[0].ID)
	}

	// Task 3 (depends on 1 and 2) should be backlog
	if tasks[3].Status != StatusBacklog {
		t.Errorf("task[3] status = %s, want backlog", tasks[3].Status)
	}
	if len(tasks[3].DependsOn) != 2 {
		t.Errorf("task[3] depends_on len = %d, want 2", len(tasks[3].DependsOn))
	}
}

func TestTaskLog(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	now := time.Now()
	store.CreateWorkspace(&Workspace{
		ID: "ws-1", Name: "test", Status: WorkspaceActive,
		MaxConcurrent: 3, CreatedAt: now, UpdatedAt: now,
	})
	store.CreateTask(&Task{ID: "t-1", WorkspaceID: "ws-1", Title: "test", Prompt: "p",
		Status: StatusRunning, Tags: []string{}, DependsOn: []string{}, CreatedAt: now})

	store.AppendTaskLog("t-1", "content", "hello world", "")
	store.AppendTaskLog("t-1", "tool_call", "", "bash")
	store.AppendTaskLog("t-1", "tool_result", "output", "bash")

	logs, err := store.GetTaskLog("t-1", 10)
	if err != nil {
		t.Fatalf("GetTaskLog: %v", err)
	}
	if len(logs) != 3 {
		t.Errorf("GetTaskLog len = %d, want 3", len(logs))
	}
	if logs[0].EventType != "content" {
		t.Errorf("logs[0].EventType = %s, want content", logs[0].EventType)
	}
	if logs[1].ToolName != "bash" {
		t.Errorf("logs[1].ToolName = %s, want bash", logs[1].ToolName)
	}
}

func TestRecoverRunningTasks(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	now := time.Now()
	store.CreateWorkspace(&Workspace{
		ID: "ws-1", Name: "test", Status: WorkspaceActive,
		MaxConcurrent: 3, CreatedAt: now, UpdatedAt: now,
	})
	store.CreateTask(&Task{ID: "t-1", WorkspaceID: "ws-1", Title: "running task", Prompt: "p",
		Status: StatusRunning, Tags: []string{}, DependsOn: []string{}, CreatedAt: now})
	store.CreateTask(&Task{ID: "t-2", WorkspaceID: "ws-1", Title: "done task", Prompt: "p",
		Status: StatusDone, Tags: []string{}, DependsOn: []string{}, CreatedAt: now})

	recovered, err := store.RecoverRunningTasks()
	if err != nil {
		t.Fatalf("RecoverRunningTasks: %v", err)
	}
	if recovered != 1 {
		t.Errorf("recovered = %d, want 1", recovered)
	}

	t1, _ := store.GetTask("t-1")
	if t1.Status != StatusReady {
		t.Errorf("t-1 status = %s, want ready", t1.Status)
	}
	t2, _ := store.GetTask("t-2")
	if t2.Status != StatusDone {
		t.Errorf("t-2 status = %s, want done (unchanged)", t2.Status)
	}
}
