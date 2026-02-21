package taskboard

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// TaskStore provides SQLite-backed persistence for workspaces, tasks, and logs.
type TaskStore struct {
	db *sql.DB
}

// NewTaskStore opens (or creates) the taskboard database and ensures tables exist.
func NewTaskStore(dbPath string) (*TaskStore, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s := &TaskStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *TaskStore) Close() error {
	return s.db.Close()
}

func (s *TaskStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS workspaces (
			id             TEXT PRIMARY KEY,
			name           TEXT NOT NULL,
			description    TEXT DEFAULT '',
			goal           TEXT DEFAULT '',
			status         TEXT DEFAULT 'active',
			max_concurrent INTEGER DEFAULT 3,
			context        TEXT DEFAULT '',
			work_dir       TEXT DEFAULT '',
			summary        TEXT DEFAULT '',
			created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at     DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS tasks (
			id               TEXT PRIMARY KEY,
			workspace_id     TEXT NOT NULL REFERENCES workspaces(id),
			title            TEXT NOT NULL,
			description      TEXT DEFAULT '',
			prompt           TEXT NOT NULL,
			status           TEXT DEFAULT 'backlog',
			priority         INTEGER DEFAULT 2,
			assigned_session TEXT DEFAULT '',
			result           TEXT DEFAULT '',
			error            TEXT DEFAULT '',
			max_retries      INTEGER DEFAULT 1,
			retry_count      INTEGER DEFAULT 0,
			tags             TEXT DEFAULT '[]',
			depends_on       TEXT DEFAULT '[]',
			created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
			started_at       DATETIME,
			completed_at     DATETIME
		);

		CREATE INDEX IF NOT EXISTS idx_tasks_workspace ON tasks(workspace_id);
		CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
		CREATE INDEX IF NOT EXISTS idx_tasks_priority ON tasks(priority);

		CREATE TABLE IF NOT EXISTS task_logs (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id    TEXT NOT NULL REFERENCES tasks(id),
			event_type TEXT NOT NULL,
			content    TEXT DEFAULT '',
			tool_name  TEXT DEFAULT '',
			timestamp  DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_task_logs_task ON task_logs(task_id);
	`)
	return err
}

// --- Workspace CRUD ---

func (s *TaskStore) CreateWorkspace(ws *Workspace) error {
	_, err := s.db.Exec(`
		INSERT INTO workspaces (id, name, description, goal, status, max_concurrent, context, work_dir, summary, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ws.ID, ws.Name, ws.Description, ws.Goal, string(ws.Status),
		ws.MaxConcurrent, ws.Context, ws.WorkDir, ws.Summary,
		ws.CreatedAt, ws.UpdatedAt)
	return err
}

func (s *TaskStore) GetWorkspace(id string) (*Workspace, error) {
	ws := &Workspace{}
	var status string
	err := s.db.QueryRow(`SELECT id, name, description, goal, status, max_concurrent, context, work_dir, summary, created_at, updated_at FROM workspaces WHERE id = ?`, id).
		Scan(&ws.ID, &ws.Name, &ws.Description, &ws.Goal, &status,
			&ws.MaxConcurrent, &ws.Context, &ws.WorkDir, &ws.Summary,
			&ws.CreatedAt, &ws.UpdatedAt)
	if err != nil {
		return nil, err
	}
	ws.Status = WorkspaceStatus(status)
	return ws, nil
}

func (s *TaskStore) UpdateWorkspace(ws *Workspace) error {
	ws.UpdatedAt = time.Now()
	_, err := s.db.Exec(`
		UPDATE workspaces SET name=?, description=?, goal=?, status=?, max_concurrent=?, context=?, work_dir=?, summary=?, updated_at=?
		WHERE id=?`,
		ws.Name, ws.Description, ws.Goal, string(ws.Status),
		ws.MaxConcurrent, ws.Context, ws.WorkDir, ws.Summary,
		ws.UpdatedAt, ws.ID)
	return err
}

func (s *TaskStore) DeleteWorkspace(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	tx.Exec(`DELETE FROM task_logs WHERE task_id IN (SELECT id FROM tasks WHERE workspace_id = ?)`, id)
	tx.Exec(`DELETE FROM tasks WHERE workspace_id = ?`, id)
	tx.Exec(`DELETE FROM workspaces WHERE id = ?`, id)
	return tx.Commit()
}

func (s *TaskStore) ListWorkspaces() ([]*Workspace, error) {
	rows, err := s.db.Query(`SELECT id, name, description, goal, status, max_concurrent, context, work_dir, summary, created_at, updated_at FROM workspaces ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*Workspace
	for rows.Next() {
		ws := &Workspace{}
		var status string
		if err := rows.Scan(&ws.ID, &ws.Name, &ws.Description, &ws.Goal, &status,
			&ws.MaxConcurrent, &ws.Context, &ws.WorkDir, &ws.Summary,
			&ws.CreatedAt, &ws.UpdatedAt); err != nil {
			return nil, err
		}
		ws.Status = WorkspaceStatus(status)
		result = append(result, ws)
	}
	return result, nil
}

// --- Task CRUD ---

func (s *TaskStore) CreateTask(t *Task) error {
	tags, _ := json.Marshal(t.Tags)
	deps, _ := json.Marshal(t.DependsOn)
	_, err := s.db.Exec(`
		INSERT INTO tasks (id, workspace_id, title, description, prompt, status, priority, assigned_session, result, error, max_retries, retry_count, tags, depends_on, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.WorkspaceID, t.Title, t.Description, t.Prompt,
		string(t.Status), t.Priority, t.AssignedSession,
		t.Result, t.Error, t.MaxRetries, t.RetryCount,
		string(tags), string(deps), t.CreatedAt)
	return err
}

func (s *TaskStore) GetTask(id string) (*Task, error) {
	t := &Task{}
	var status, tags, deps string
	var startedAt, completedAt sql.NullTime
	err := s.db.QueryRow(`
		SELECT id, workspace_id, title, description, prompt, status, priority,
		       assigned_session, result, error, max_retries, retry_count,
		       tags, depends_on, created_at, started_at, completed_at
		FROM tasks WHERE id = ?`, id).
		Scan(&t.ID, &t.WorkspaceID, &t.Title, &t.Description, &t.Prompt,
			&status, &t.Priority, &t.AssignedSession,
			&t.Result, &t.Error, &t.MaxRetries, &t.RetryCount,
			&tags, &deps, &t.CreatedAt, &startedAt, &completedAt)
	if err != nil {
		return nil, err
	}
	t.Status = TaskStatus(status)
	json.Unmarshal([]byte(tags), &t.Tags)
	json.Unmarshal([]byte(deps), &t.DependsOn)
	if startedAt.Valid {
		t.StartedAt = startedAt.Time
	}
	if completedAt.Valid {
		t.CompletedAt = completedAt.Time
	}
	if t.Tags == nil {
		t.Tags = []string{}
	}
	if t.DependsOn == nil {
		t.DependsOn = []string{}
	}
	return t, nil
}

func (s *TaskStore) UpdateTask(t *Task) error {
	tags, _ := json.Marshal(t.Tags)
	deps, _ := json.Marshal(t.DependsOn)
	var startedAt, completedAt *time.Time
	if !t.StartedAt.IsZero() {
		startedAt = &t.StartedAt
	}
	if !t.CompletedAt.IsZero() {
		completedAt = &t.CompletedAt
	}
	_, err := s.db.Exec(`
		UPDATE tasks SET title=?, description=?, prompt=?, status=?, priority=?,
		       assigned_session=?, result=?, error=?, max_retries=?, retry_count=?,
		       tags=?, depends_on=?, started_at=?, completed_at=?
		WHERE id=?`,
		t.Title, t.Description, t.Prompt, string(t.Status), t.Priority,
		t.AssignedSession, t.Result, t.Error, t.MaxRetries, t.RetryCount,
		string(tags), string(deps), startedAt, completedAt, t.ID)
	return err
}

func (s *TaskStore) DeleteTask(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	tx.Exec(`DELETE FROM task_logs WHERE task_id = ?`, id)
	tx.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	return tx.Commit()
}

func (s *TaskStore) ListTasks(workspaceID string, statusFilter string) ([]*Task, error) {
	query := `SELECT id, workspace_id, title, description, prompt, status, priority,
	                 assigned_session, result, error, max_retries, retry_count,
	                 tags, depends_on, created_at, started_at, completed_at
	          FROM tasks`
	var args []interface{}
	var conditions []string

	if workspaceID != "" {
		conditions = append(conditions, "workspace_id = ?")
		args = append(args, workspaceID)
	}
	if statusFilter != "" {
		statuses := strings.Split(statusFilter, ",")
		placeholders := make([]string, len(statuses))
		for i, st := range statuses {
			placeholders[i] = "?"
			args = append(args, strings.TrimSpace(st))
		}
		conditions = append(conditions, fmt.Sprintf("status IN (%s)", strings.Join(placeholders, ",")))
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY priority ASC, created_at ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*Task
	for rows.Next() {
		t := &Task{}
		var status, tags, deps string
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.WorkspaceID, &t.Title, &t.Description, &t.Prompt,
			&status, &t.Priority, &t.AssignedSession,
			&t.Result, &t.Error, &t.MaxRetries, &t.RetryCount,
			&tags, &deps, &t.CreatedAt, &startedAt, &completedAt); err != nil {
			return nil, err
		}
		t.Status = TaskStatus(status)
		json.Unmarshal([]byte(tags), &t.Tags)
		json.Unmarshal([]byte(deps), &t.DependsOn)
		if startedAt.Valid {
			t.StartedAt = startedAt.Time
		}
		if completedAt.Valid {
			t.CompletedAt = completedAt.Time
		}
		if t.Tags == nil {
			t.Tags = []string{}
		}
		if t.DependsOn == nil {
			t.DependsOn = []string{}
		}
		result = append(result, t)
	}
	return result, nil
}

// --- Queries ---

// GetReadyTasks returns up to limit tasks in ready state for a workspace, ordered by priority.
func (s *TaskStore) GetReadyTasks(workspaceID string, limit int) ([]*Task, error) {
	return s.listByStatus(workspaceID, StatusReady, limit)
}

func (s *TaskStore) listByStatus(workspaceID string, status TaskStatus, limit int) ([]*Task, error) {
	rows, err := s.db.Query(`
		SELECT id, workspace_id, title, description, prompt, status, priority,
		       assigned_session, result, error, max_retries, retry_count,
		       tags, depends_on, created_at, started_at, completed_at
		FROM tasks WHERE workspace_id = ? AND status = ?
		ORDER BY priority ASC, created_at ASC
		LIMIT ?`, workspaceID, string(status), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanTasks(rows)
}

// CountByStatus returns task counts grouped by status for a workspace.
func (s *TaskStore) CountByStatus(workspaceID string) (*StatusCounts, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM tasks WHERE workspace_id = ? GROUP BY status`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := &StatusCounts{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		switch TaskStatus(status) {
		case StatusBacklog:
			counts.Backlog = count
		case StatusReady:
			counts.Ready = count
		case StatusRunning:
			counts.Running = count
		case StatusDone:
			counts.Done = count
		case StatusFailed:
			counts.Failed = count
		case StatusCancelled:
			counts.Cancelled = count
		}
	}
	return counts, nil
}

// GetTasksByIDs returns tasks matching the given IDs.
func (s *TaskStore) GetTasksByIDs(ids []string) ([]*Task, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`
		SELECT id, workspace_id, title, description, prompt, status, priority,
		       assigned_session, result, error, max_retries, retry_count,
		       tags, depends_on, created_at, started_at, completed_at
		FROM tasks WHERE id IN (%s)`, strings.Join(placeholders, ","))
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanTasks(rows)
}

// GetDependents returns tasks that depend on the given task ID.
func (s *TaskStore) GetDependents(taskID string) ([]*Task, error) {
	// SQLite JSON: depends_on is stored as JSON array, search with LIKE
	rows, err := s.db.Query(`
		SELECT id, workspace_id, title, description, prompt, status, priority,
		       assigned_session, result, error, max_retries, retry_count,
		       tags, depends_on, created_at, started_at, completed_at
		FROM tasks WHERE depends_on LIKE ?`, fmt.Sprintf("%%%s%%", taskID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	allTasks, err := s.scanTasks(rows)
	if err != nil {
		return nil, err
	}
	// Filter to exact matches (LIKE might match substrings)
	var result []*Task
	for _, t := range allTasks {
		for _, dep := range t.DependsOn {
			if dep == taskID {
				result = append(result, t)
				break
			}
		}
	}
	return result, nil
}

// CreateFromPlan creates a workspace and all tasks from a PlanResult.
// Returns the workspace and tasks with their assigned IDs.
func (s *TaskStore) CreateFromPlan(plan *PlanResult, workspaceID string, goal string, workDir string) (*Workspace, []*Task, error) {
	now := time.Now()
	maxConcurrent := plan.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}

	ws := &Workspace{
		ID:            workspaceID,
		Name:          plan.WorkspaceName,
		Goal:          goal,
		Status:        WorkspaceActive,
		MaxConcurrent: maxConcurrent,
		Context:       plan.WorkspaceContext,
		WorkDir:       workDir,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	// Insert workspace
	_, err = tx.Exec(`
		INSERT INTO workspaces (id, name, description, goal, status, max_concurrent, context, work_dir, summary, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ws.ID, ws.Name, ws.Description, ws.Goal, string(ws.Status),
		ws.MaxConcurrent, ws.Context, ws.WorkDir, ws.Summary,
		ws.CreatedAt, ws.UpdatedAt)
	if err != nil {
		return nil, nil, fmt.Errorf("create workspace: %w", err)
	}

	// Generate task IDs first so we can map dependency indices to IDs
	taskIDs := make([]string, len(plan.Tasks))
	for i := range plan.Tasks {
		taskIDs[i] = fmt.Sprintf("%s-t%d", workspaceID, i+1)
	}

	var tasks []*Task
	for i, pt := range plan.Tasks {
		// Map dependency indices to task IDs
		var deps []string
		for _, depIdx := range pt.DependsOn {
			if depIdx >= 0 && depIdx < len(taskIDs) {
				deps = append(deps, taskIDs[depIdx])
			}
		}

		// Tasks with no dependencies start as ready
		status := StatusBacklog
		if len(deps) == 0 {
			status = StatusReady
		}

		t := &Task{
			ID:          taskIDs[i],
			WorkspaceID: workspaceID,
			Title:       pt.Title,
			Description: pt.Description,
			Prompt:      pt.Prompt,
			Status:      status,
			Priority:    pt.Priority,
			MaxRetries:  1,
			Tags:        pt.Tags,
			DependsOn:   deps,
			CreatedAt:   now,
		}
		if t.Tags == nil {
			t.Tags = []string{}
		}
		if t.DependsOn == nil {
			t.DependsOn = []string{}
		}

		tagsJSON, _ := json.Marshal(t.Tags)
		depsJSON, _ := json.Marshal(t.DependsOn)

		_, err = tx.Exec(`
			INSERT INTO tasks (id, workspace_id, title, description, prompt, status, priority, max_retries, tags, depends_on, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID, t.WorkspaceID, t.Title, t.Description, t.Prompt,
			string(t.Status), t.Priority, t.MaxRetries,
			string(tagsJSON), string(depsJSON), t.CreatedAt)
		if err != nil {
			return nil, nil, fmt.Errorf("create task %d: %w", i, err)
		}
		tasks = append(tasks, t)
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return ws, tasks, nil
}

// --- Task Logs ---

func (s *TaskStore) AppendTaskLog(taskID, eventType, content, toolName string) error {
	_, err := s.db.Exec(`INSERT INTO task_logs (task_id, event_type, content, tool_name) VALUES (?, ?, ?, ?)`,
		taskID, eventType, content, toolName)
	return err
}

func (s *TaskStore) GetTaskLog(taskID string, limit int) ([]*TaskLog, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`
		SELECT id, task_id, event_type, content, tool_name, timestamp
		FROM task_logs WHERE task_id = ?
		ORDER BY id ASC LIMIT ?`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*TaskLog
	for rows.Next() {
		l := &TaskLog{}
		if err := rows.Scan(&l.ID, &l.TaskID, &l.EventType, &l.Content, &l.ToolName, &l.Timestamp); err != nil {
			return nil, err
		}
		result = append(result, l)
	}
	return result, nil
}

// --- Recovery ---

// RecoverRunningTasks resets any tasks left in running state (from a daemon crash) back to ready.
func (s *TaskStore) RecoverRunningTasks() (int64, error) {
	res, err := s.db.Exec(`UPDATE tasks SET status = 'ready', assigned_session = '' WHERE status = 'running'`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- internal helpers ---

func (s *TaskStore) scanTasks(rows *sql.Rows) ([]*Task, error) {
	var result []*Task
	for rows.Next() {
		t := &Task{}
		var status, tags, deps string
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.WorkspaceID, &t.Title, &t.Description, &t.Prompt,
			&status, &t.Priority, &t.AssignedSession,
			&t.Result, &t.Error, &t.MaxRetries, &t.RetryCount,
			&tags, &deps, &t.CreatedAt, &startedAt, &completedAt); err != nil {
			return nil, err
		}
		t.Status = TaskStatus(status)
		json.Unmarshal([]byte(tags), &t.Tags)
		json.Unmarshal([]byte(deps), &t.DependsOn)
		if startedAt.Valid {
			t.StartedAt = startedAt.Time
		}
		if completedAt.Valid {
			t.CompletedAt = completedAt.Time
		}
		if t.Tags == nil {
			t.Tags = []string{}
		}
		if t.DependsOn == nil {
			t.DependsOn = []string{}
		}
		result = append(result, t)
	}
	return result, nil
}
