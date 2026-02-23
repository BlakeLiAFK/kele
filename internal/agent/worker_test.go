package agent

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/BlakeLiAFK/kele/internal/tools"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"中文测试字符串", 4, "中文测试..."},
	}
	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestCompressOutput(t *testing.T) {
	// 短文本不压缩
	short := "hello"
	if got := compressOutput(short, 1024); got != short {
		t.Errorf("短文本不应被压缩")
	}

	// 超过 maxSize 截断
	long := strings.Repeat("x", 10000)
	got := compressOutput(long, 5000)
	if len(got) > 6000 {
		t.Errorf("截断后长度 %d 超预期", len(got))
	}
	if !strings.Contains(got, "输出被截断") {
		t.Error("应包含截断提示")
	}

	// 超过压缩阈值的中间段省略
	medium := strings.Repeat("a", 3000)
	got = compressOutput(medium, 51200)
	if !strings.Contains(got, "省略") {
		t.Error("应包含省略提示")
	}
}

func TestWorkerInfo(t *testing.T) {
	w := &Worker{
		id:     "w1",
		task:   "test task",
		status: StatusPending,
	}

	info := w.info()
	if info.ID != "w1" {
		t.Errorf("ID = %s, want w1", info.ID)
	}
	if info.Status != "pending" {
		t.Errorf("Status = %s, want pending", info.Status)
	}
	if info.Task != "test task" {
		t.Errorf("Task = %s, want test task", info.Task)
	}
}

func TestWorkerAppendLog(t *testing.T) {
	w := &Worker{
		id:     "w1",
		task:   "test",
		status: StatusRunning,
	}

	w.appendLog("info", "log message 1")
	w.appendLog("tool_call", "bash")

	if len(w.log) != 2 {
		t.Fatalf("log count = %d, want 2", len(w.log))
	}
	if w.log[0].Type != "info" {
		t.Errorf("log[0].Type = %s, want info", w.log[0].Type)
	}
	if w.log[1].Content != "bash" {
		t.Errorf("log[1].Content = %s, want bash", w.log[1].Content)
	}
}

func TestWorkerFinish(t *testing.T) {
	w := &Worker{
		id:        "w1",
		task:      "test",
		status:    StatusRunning,
		startTime: time.Now(),
		done:      make(chan struct{}),
	}

	w.finish(StatusCompleted, "result text", "")

	if w.status != StatusCompleted {
		t.Errorf("status = %s, want completed", w.status)
	}
	if w.result != "result text" {
		t.Errorf("result = %s, want 'result text'", w.result)
	}
	if w.endTime.IsZero() {
		t.Error("endTime should not be zero")
	}

	// finish 应该写入 done 日志
	found := false
	for _, l := range w.log {
		if l.Type == "done" {
			found = true
			break
		}
	}
	if !found {
		t.Error("should have a 'done' log entry")
	}
}

func TestWorkerFinishFailed(t *testing.T) {
	w := &Worker{
		id:        "w1",
		task:      "test",
		status:    StatusRunning,
		startTime: time.Now(),
		done:      make(chan struct{}),
	}

	w.finish(StatusFailed, "", "something went wrong")

	if w.status != StatusFailed {
		t.Errorf("status = %s, want failed", w.status)
	}
	if w.errMsg != "something went wrong" {
		t.Errorf("errMsg = %s, want 'something went wrong'", w.errMsg)
	}

	info := w.info()
	if info.Error != "something went wrong" {
		t.Errorf("info.Error = %s", info.Error)
	}
}

func TestWorkerPoolListAllEmpty(t *testing.T) {
	pool := &WorkerPool{
		workers: make(map[string]*Worker),
	}
	all := pool.ListAll()
	if len(all) != 0 {
		t.Errorf("empty pool should return 0 agents, got %d", len(all))
	}
}

func TestWorkerPoolStatusNotFound(t *testing.T) {
	pool := &WorkerPool{
		workers: make(map[string]*Worker),
	}
	_, err := pool.Status("nonexistent")
	if err == nil {
		t.Error("should return error for nonexistent agent")
	}
}

func TestWorkerPoolResultNotFound(t *testing.T) {
	pool := &WorkerPool{
		workers: make(map[string]*Worker),
	}
	_, err := pool.Result("nonexistent", time.Second)
	if err == nil {
		t.Error("should return error for nonexistent agent")
	}
}

func TestWorkerPoolRecentLogsNotFound(t *testing.T) {
	pool := &WorkerPool{
		workers: make(map[string]*Worker),
	}
	_, err := pool.RecentLogs("nonexistent", 5)
	if err == nil {
		t.Error("should return error for nonexistent agent")
	}
}

func TestWorkerPoolRecentLogs(t *testing.T) {
	w := &Worker{
		id:     "w1",
		task:   "test",
		status: StatusCompleted,
		log: []tools.AgentLogEntry{
			{Time: time.Now(), Type: "info", Content: "started"},
			{Time: time.Now(), Type: "tool_call", Content: "bash"},
			{Time: time.Now(), Type: "done", Content: "finished"},
		},
	}

	pool := &WorkerPool{
		workers: map[string]*Worker{"w1": w},
	}

	// 取最近 2 条
	logs, err := pool.RecentLogs("w1", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("want 2 logs, got %d", len(logs))
	}
	if logs[0].Type != "tool_call" {
		t.Errorf("logs[0].Type = %s, want tool_call", logs[0].Type)
	}
	if logs[1].Type != "done" {
		t.Errorf("logs[1].Type = %s, want done", logs[1].Type)
	}

	// 取全部（n=0）
	all, err := pool.RecentLogs("w1", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("want 3 logs, got %d", len(all))
	}
}

func TestWorkerPoolResultBlocking(t *testing.T) {
	w := &Worker{
		id:     "w1",
		task:   "test",
		status: StatusRunning,
		done:   make(chan struct{}),
	}

	pool := &WorkerPool{
		workers: map[string]*Worker{"w1": w},
	}

	// 模拟子 agent 完成
	go func() {
		time.Sleep(50 * time.Millisecond)
		w.mu.Lock()
		w.status = StatusCompleted
		w.result = "done!"
		w.mu.Unlock()
		close(w.done)
	}()

	result, err := pool.Result("w1", 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "done!" {
		t.Errorf("result = %s, want 'done!'", result)
	}
}

func TestWorkerPoolResultTimeout(t *testing.T) {
	w := &Worker{
		id:     "w1",
		task:   "test",
		status: StatusRunning,
		done:   make(chan struct{}),
	}

	pool := &WorkerPool{
		workers: map[string]*Worker{"w1": w},
	}

	_, err := pool.Result("w1", 50*time.Millisecond)
	if err == nil {
		t.Error("should timeout")
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Errorf("error should mention timeout, got: %s", err.Error())
	}
}

func TestWorkerPoolResultFailed(t *testing.T) {
	w := &Worker{
		id:     "w1",
		task:   "test",
		status: StatusFailed,
		errMsg: "LLM error",
		done:   make(chan struct{}),
	}
	close(w.done)

	pool := &WorkerPool{
		workers: map[string]*Worker{"w1": w},
	}

	_, err := pool.Result("w1", time.Second)
	if err == nil {
		t.Error("should return error for failed agent")
	}
	if !strings.Contains(err.Error(), "LLM error") {
		t.Errorf("error should contain original error, got: %s", err.Error())
	}
}

func TestFilterTools(t *testing.T) {
	w := &Worker{}

	names := []string{"bash", "read", "write", "spawn_agent", "agent_status", "agent_result", "http"}
	filtered := w.filterToolNames(names)

	expected := map[string]bool{"bash": true, "read": true, "write": true, "http": true}
	if len(filtered) != len(expected) {
		t.Fatalf("filtered count = %d, want %d", len(filtered), len(expected))
	}
	for _, name := range filtered {
		if !expected[name] {
			t.Errorf("unexpected tool in filtered list: %s", name)
		}
	}
}

func TestWorkerPoolConcurrencyLimit(t *testing.T) {
	pool := &WorkerPool{
		workers:       make(map[string]*Worker),
		maxConcurrent: 2,
		running:       2,
	}

	_, err := pool.Spawn("task")
	if err == nil {
		t.Error("should reject when at concurrency limit")
	}
	if !strings.Contains(err.Error(), "上限") {
		t.Errorf("error should mention limit, got: %s", err.Error())
	}
}

func TestWorkerPoolShutdownRejectsNewSpawn(t *testing.T) {
	pool := &WorkerPool{
		workers:       make(map[string]*Worker),
		maxConcurrent: 5,
		stopped:       true,
	}

	_, err := pool.Spawn("task")
	if err == nil {
		t.Error("should reject spawn after shutdown")
	}
	if !strings.Contains(err.Error(), "已关闭") {
		t.Errorf("error should mention closed, got: %s", err.Error())
	}
}

func TestWorkerRunPanicRecovery(t *testing.T) {
	w := &Worker{
		id:        "w1",
		task:      "panic task",
		status:    StatusPending,
		done:      make(chan struct{}),
		startTime: time.Now(),
	}

	// 模拟 panic: provider 为 nil 会在 run() 中 panic
	// 直接测试 panic recovery 逻辑
	go func() {
		defer close(w.done)
		defer func() {
			if r := recover(); r != nil {
				w.finish(StatusFailed, "", fmt.Sprintf("panic: %v", r))
			}
		}()
		w.mu.Lock()
		w.status = StatusRunning
		w.mu.Unlock()
		panic("test panic")
	}()

	<-w.done

	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.status != StatusFailed {
		t.Errorf("status = %s, want failed", w.status)
	}
	if !strings.Contains(w.errMsg, "panic") {
		t.Errorf("errMsg should contain 'panic', got: %s", w.errMsg)
	}
}
