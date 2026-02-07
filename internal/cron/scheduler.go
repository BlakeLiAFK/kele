package cron

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Job 定时任务
type Job struct {
	ID         string
	Name       string
	Schedule   string
	Command    string
	Enabled    bool
	LastRun    *time.Time
	NextRun    *time.Time
	LastResult string
	LastError  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// LogEntry 执行日志
type LogEntry struct {
	ID         int
	JobID      string
	RunAt      time.Time
	Output     string
	Error      string
	DurationMs int64
}

// Scheduler 定时任务调度器
type Scheduler struct {
	db      *sql.DB
	workDir string
	done    chan struct{}
	running bool
	mu      sync.Mutex
}

// NewScheduler 创建调度器
func NewScheduler(dbPath, workDir string) *Scheduler {
	os.MkdirAll(".kele", 0755)

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		panic(fmt.Sprintf("打开 cron 数据库失败: %v", err))
	}

	s := &Scheduler{
		db:      db,
		workDir: workDir,
	}
	s.initSchema()
	return s
}

// initSchema 初始化数据表
func (s *Scheduler) initSchema() {
	schema := `
	CREATE TABLE IF NOT EXISTS cron_jobs (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		schedule TEXT NOT NULL,
		command TEXT NOT NULL,
		enabled INTEGER DEFAULT 1,
		last_run DATETIME,
		next_run DATETIME,
		last_result TEXT DEFAULT '',
		last_error TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS cron_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id TEXT NOT NULL,
		run_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		output TEXT DEFAULT '',
		error TEXT DEFAULT '',
		duration_ms INTEGER DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_cron_logs_job ON cron_logs(job_id, run_at DESC);
	`
	if _, err := s.db.Exec(schema); err != nil {
		panic(fmt.Sprintf("初始化 cron 表失败: %v", err))
	}
}

// Start 启动调度器
func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	s.running = true
	s.done = make(chan struct{})
	go s.run()
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	close(s.done)
}

// Close 关闭数据库连接
func (s *Scheduler) Close() {
	s.Stop()
	if s.db != nil {
		s.db.Close()
	}
}

// run 调度主循环
func (s *Scheduler) run() {
	// 对齐到下一分钟边界
	now := time.Now()
	next := now.Truncate(time.Minute).Add(time.Minute)
	select {
	case <-time.After(time.Until(next)):
	case <-s.done:
		return
	}

	s.tick()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

// tick 每分钟检查并执行匹配的任务
func (s *Scheduler) tick() {
	now := time.Now()
	jobs, err := s.listEnabled()
	if err != nil {
		return
	}
	for _, job := range jobs {
		expr, err := Parse(job.Schedule)
		if err != nil {
			continue
		}
		if !expr.Matches(now) {
			continue
		}
		// 本分钟已执行过则跳过
		if job.LastRun != nil && now.Truncate(time.Minute).Equal(job.LastRun.Truncate(time.Minute)) {
			continue
		}
		go s.executeJob(job, now)
	}
}

// executeJob 执行单个任务
func (s *Scheduler) executeJob(job Job, runAt time.Time) {
	// 安全检查
	if isDangerous(job.Command) {
		s.logExecution(job.ID, runAt, "", "禁止执行危险命令", 0)
		return
	}

	start := time.Now()

	// 5 分钟超时
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", job.Command)
	cmd.Dir = s.workDir
	output, err := cmd.CombinedOutput()
	duration := time.Since(start).Milliseconds()

	errStr := ""
	if err != nil {
		errStr = err.Error()
	}

	// 计算下次执行时间
	var nextRun *time.Time
	if expr, e := Parse(job.Schedule); e == nil {
		t := expr.NextAfter(time.Now())
		if !t.IsZero() {
			nextRun = &t
		}
	}

	// 更新 job 状态
	s.db.Exec(`UPDATE cron_jobs SET last_run=?, last_result=?, last_error=?,
		next_run=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		runAt, string(output), errStr, nextRun, job.ID)

	// 记录日志
	s.logExecution(job.ID, runAt, string(output), errStr, duration)
}

// logExecution 记录执行日志
func (s *Scheduler) logExecution(jobID string, runAt time.Time, output, errStr string, durationMs int64) {
	s.db.Exec(`INSERT INTO cron_logs (job_id, run_at, output, error, duration_ms)
		VALUES (?, ?, ?, ?, ?)`, jobID, runAt, output, errStr, durationMs)

	// 保留最近 50 条日志
	s.db.Exec(`DELETE FROM cron_logs WHERE job_id=? AND id NOT IN
		(SELECT id FROM cron_logs WHERE job_id=? ORDER BY run_at DESC LIMIT 50)`,
		jobID, jobID)
}

// listEnabled 获取所有启用的任务
func (s *Scheduler) listEnabled() ([]Job, error) {
	return s.queryJobs("SELECT id, name, schedule, command, enabled, last_run, next_run, last_result, last_error, created_at, updated_at FROM cron_jobs WHERE enabled=1")
}

// --- CRUD ---

// CreateJob 创建定时任务
func (s *Scheduler) CreateJob(name, schedule, command string) (*Job, error) {
	expr, err := Parse(schedule)
	if err != nil {
		return nil, fmt.Errorf("无效的 cron 表达式: %v", err)
	}

	id := generateID()
	now := time.Now()
	next := expr.NextAfter(now)

	_, err = s.db.Exec(`INSERT INTO cron_jobs (id, name, schedule, command, enabled, next_run, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, ?, ?, ?)`,
		id, name, schedule, command, next, now, now)
	if err != nil {
		return nil, fmt.Errorf("创建任务失败: %v", err)
	}

	return &Job{
		ID:        id,
		Name:      name,
		Schedule:  schedule,
		Command:   command,
		Enabled:   true,
		NextRun:   &next,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// ListJobs 列出所有任务
func (s *Scheduler) ListJobs() ([]Job, error) {
	return s.queryJobs("SELECT id, name, schedule, command, enabled, last_run, next_run, last_result, last_error, created_at, updated_at FROM cron_jobs ORDER BY created_at DESC")
}

// GetJob 获取任务详情 + 最近日志
func (s *Scheduler) GetJob(id string) (*Job, []LogEntry, error) {
	jobs, err := s.queryJobs("SELECT id, name, schedule, command, enabled, last_run, next_run, last_result, last_error, created_at, updated_at FROM cron_jobs WHERE id=?", id)
	if err != nil {
		return nil, nil, err
	}
	if len(jobs) == 0 {
		return nil, nil, fmt.Errorf("任务不存在: %s", id)
	}

	// 查询最近 10 条日志
	rows, err := s.db.Query("SELECT id, job_id, run_at, output, error, duration_ms FROM cron_logs WHERE job_id=? ORDER BY run_at DESC LIMIT 10", id)
	if err != nil {
		return &jobs[0], nil, nil
	}
	defer rows.Close()

	var logs []LogEntry
	for rows.Next() {
		var l LogEntry
		var runAt string
		if err := rows.Scan(&l.ID, &l.JobID, &runAt, &l.Output, &l.Error, &l.DurationMs); err != nil {
			continue
		}
		l.RunAt, _ = time.Parse("2006-01-02 15:04:05", runAt)
		logs = append(logs, l)
	}

	return &jobs[0], logs, nil
}

// UpdateJob 更新任务属性
func (s *Scheduler) UpdateJob(id string, updates map[string]interface{}) (*Job, error) {
	// 检查任务是否存在
	jobs, err := s.queryJobs("SELECT id, name, schedule, command, enabled, last_run, next_run, last_result, last_error, created_at, updated_at FROM cron_jobs WHERE id=?", id)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, fmt.Errorf("任务不存在: %s", id)
	}

	var setClauses []string
	var args []interface{}

	if v, ok := updates["name"]; ok {
		setClauses = append(setClauses, "name=?")
		args = append(args, v)
	}
	if v, ok := updates["schedule"]; ok {
		schedule := v.(string)
		if _, err := Parse(schedule); err != nil {
			return nil, fmt.Errorf("无效的 cron 表达式: %v", err)
		}
		setClauses = append(setClauses, "schedule=?")
		args = append(args, schedule)
		// 重算下次执行时间
		if expr, e := Parse(schedule); e == nil {
			next := expr.NextAfter(time.Now())
			setClauses = append(setClauses, "next_run=?")
			args = append(args, next)
		}
	}
	if v, ok := updates["command"]; ok {
		setClauses = append(setClauses, "command=?")
		args = append(args, v)
	}
	if v, ok := updates["enabled"]; ok {
		setClauses = append(setClauses, "enabled=?")
		if v.(bool) {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}

	if len(setClauses) == 0 {
		return &jobs[0], nil
	}

	setClauses = append(setClauses, "updated_at=CURRENT_TIMESTAMP")
	query := fmt.Sprintf("UPDATE cron_jobs SET %s WHERE id=?", strings.Join(setClauses, ", "))
	args = append(args, id)

	if _, err := s.db.Exec(query, args...); err != nil {
		return nil, fmt.Errorf("更新失败: %v", err)
	}

	// 返回更新后的任务
	updated, _ := s.queryJobs("SELECT id, name, schedule, command, enabled, last_run, next_run, last_result, last_error, created_at, updated_at FROM cron_jobs WHERE id=?", id)
	if len(updated) > 0 {
		return &updated[0], nil
	}
	return &jobs[0], nil
}

// DeleteJob 删除任务
func (s *Scheduler) DeleteJob(id string) error {
	result, err := s.db.Exec("DELETE FROM cron_jobs WHERE id=?", id)
	if err != nil {
		return fmt.Errorf("删除失败: %v", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("任务不存在: %s", id)
	}
	// 清理日志
	s.db.Exec("DELETE FROM cron_logs WHERE job_id=?", id)
	return nil
}

// --- 辅助方法 ---

// queryJobs 查询任务列表
func (s *Scheduler) queryJobs(query string, args ...interface{}) ([]Job, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		var enabled int
		var lastRun, nextRun, createdAt, updatedAt sql.NullString

		if err := rows.Scan(&j.ID, &j.Name, &j.Schedule, &j.Command, &enabled,
			&lastRun, &nextRun, &j.LastResult, &j.LastError, &createdAt, &updatedAt); err != nil {
			continue
		}

		j.Enabled = enabled == 1
		if lastRun.Valid {
			t, _ := time.Parse("2006-01-02 15:04:05", lastRun.String)
			j.LastRun = &t
		}
		if nextRun.Valid {
			t, _ := time.Parse("2006-01-02 15:04:05", nextRun.String)
			j.NextRun = &t
		}
		if createdAt.Valid {
			j.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt.String)
		}
		if updatedAt.Valid {
			j.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt.String)
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// generateID 生成任务 ID
func generateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("cj_%x", b)
}

// isDangerous 检查命令是否危险
func isDangerous(command string) bool {
	dangerous := []string{"rm -rf /", "dd if=", "mkfs", "> /dev/", ":(){ :|:& };:"}
	for _, d := range dangerous {
		if strings.Contains(command, d) {
			return true
		}
	}
	return false
}
