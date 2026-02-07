package cron

import (
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// --- 解析器测试 ---

func TestParseBasic(t *testing.T) {
	cases := []struct {
		expr    string
		wantErr bool
	}{
		{"* * * * *", false},
		{"0 0 * * *", false},
		{"*/5 * * * *", false},
		{"0 9 * * 1-5", false},
		{"0,30 * * * *", false},
		{"0 0 1 1 *", false},
		{"@hourly", false},
		{"@daily", false},
		{"@weekly", false},
		{"@monthly", false},
		{"@yearly", false},
		// 错误用例
		{"", true},
		{"* * *", true},
		{"60 * * * *", true},
		{"* 25 * * *", true},
		{"* * 32 * *", true},
		{"* * * 13 *", true},
		{"* * * * 7", true},
		{"abc * * * *", true},
	}

	for _, tc := range cases {
		_, err := Parse(tc.expr)
		if tc.wantErr && err == nil {
			t.Errorf("Parse(%q) 应该返回错误", tc.expr)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("Parse(%q) 意外错误: %v", tc.expr, err)
		}
	}
}

func TestMatchEveryMinute(t *testing.T) {
	expr, err := Parse("* * * * *")
	if err != nil {
		t.Fatal(err)
	}

	// 任意时间都应该匹配
	now := time.Now()
	if !expr.Matches(now) {
		t.Errorf("* * * * * 应匹配 %v", now)
	}
}

func TestMatchSpecific(t *testing.T) {
	expr, err := Parse("30 9 * * 1-5")
	if err != nil {
		t.Fatal(err)
	}

	// 周一 09:30 应匹配
	mon := time.Date(2026, 2, 9, 9, 30, 0, 0, time.Local) // 2026-02-09 是周一
	if !expr.Matches(mon) {
		t.Errorf("应匹配周一 09:30, weekday=%d", mon.Weekday())
	}

	// 周六 09:30 不应匹配
	sat := time.Date(2026, 2, 7, 9, 30, 0, 0, time.Local) // 2026-02-07 是周六
	if expr.Matches(sat) {
		t.Errorf("不应匹配周六 09:30, weekday=%d", sat.Weekday())
	}

	// 周一 10:30 不应匹配
	mon1030 := time.Date(2026, 2, 9, 10, 30, 0, 0, time.Local)
	if expr.Matches(mon1030) {
		t.Errorf("不应匹配周一 10:30")
	}
}

func TestMatchStep(t *testing.T) {
	expr, err := Parse("*/15 * * * *")
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 2, 7, 12, 0, 0, 0, time.Local)
	// 0, 15, 30, 45 分钟应匹配
	for _, m := range []int{0, 15, 30, 45} {
		tm := base.Add(time.Duration(m) * time.Minute)
		if !expr.Matches(tm) {
			t.Errorf("*/15 应匹配分钟 %d", m)
		}
	}
	// 5 分钟不应匹配
	tm5 := base.Add(5 * time.Minute)
	if expr.Matches(tm5) {
		t.Errorf("*/15 不应匹配分钟 5")
	}
}

func TestMatchList(t *testing.T) {
	expr, err := Parse("0,15,30,45 * * * *")
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 2, 7, 12, 0, 0, 0, time.Local)
	for _, m := range []int{0, 15, 30, 45} {
		tm := base.Add(time.Duration(m) * time.Minute)
		if !expr.Matches(tm) {
			t.Errorf("列表 应匹配分钟 %d", m)
		}
	}
}

func TestMatchRange(t *testing.T) {
	expr, err := Parse("0 9-17 * * *")
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 2, 7, 0, 0, 0, 0, time.Local)
	// 9-17 点整应匹配
	for h := 9; h <= 17; h++ {
		tm := time.Date(2026, 2, 7, h, 0, 0, 0, time.Local)
		if !expr.Matches(tm) {
			t.Errorf("9-17 应匹配小时 %d", h)
		}
	}
	// 8 点不应匹配
	tm8 := base.Add(8 * time.Hour)
	if expr.Matches(tm8) {
		t.Errorf("9-17 不应匹配小时 8")
	}
}

func TestMatchRangeStep(t *testing.T) {
	expr, err := Parse("0 8-18/2 * * *")
	if err != nil {
		t.Fatal(err)
	}

	// 8, 10, 12, 14, 16, 18 应匹配
	for _, h := range []int{8, 10, 12, 14, 16, 18} {
		tm := time.Date(2026, 2, 7, h, 0, 0, 0, time.Local)
		if !expr.Matches(tm) {
			t.Errorf("8-18/2 应匹配小时 %d", h)
		}
	}
	// 9, 11 不应匹配
	for _, h := range []int{9, 11} {
		tm := time.Date(2026, 2, 7, h, 0, 0, 0, time.Local)
		if expr.Matches(tm) {
			t.Errorf("8-18/2 不应匹配小时 %d", h)
		}
	}
}

func TestNextAfter(t *testing.T) {
	expr, err := Parse("0 9 * * *")
	if err != nil {
		t.Fatal(err)
	}

	// 从 08:30 开始，下一个应该是当天 09:00
	base := time.Date(2026, 2, 7, 8, 30, 0, 0, time.Local)
	next := expr.NextAfter(base)

	expected := time.Date(2026, 2, 7, 9, 0, 0, 0, time.Local)
	if !next.Equal(expected) {
		t.Errorf("NextAfter 期望 %v, 得到 %v", expected, next)
	}

	// 从 09:30 开始，下一个应该是明天 09:00
	base2 := time.Date(2026, 2, 7, 9, 30, 0, 0, time.Local)
	next2 := expr.NextAfter(base2)

	expected2 := time.Date(2026, 2, 8, 9, 0, 0, 0, time.Local)
	if !next2.Equal(expected2) {
		t.Errorf("NextAfter 期望 %v, 得到 %v", expected2, next2)
	}
}

func TestShortcuts(t *testing.T) {
	cases := map[string]string{
		"@hourly":  "0 * * * *",
		"@daily":   "0 0 * * *",
		"@weekly":  "0 0 * * 0",
		"@monthly": "0 0 1 * *",
		"@yearly":  "0 0 1 1 *",
	}

	for shortcut, expected := range cases {
		expr1, err := Parse(shortcut)
		if err != nil {
			t.Errorf("Parse(%q) 失败: %v", shortcut, err)
			continue
		}
		expr2, err := Parse(expected)
		if err != nil {
			t.Errorf("Parse(%q) 失败: %v", expected, err)
			continue
		}

		// 测试多个时间点匹配一致
		testTimes := []time.Time{
			time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local),
			time.Date(2026, 6, 15, 12, 30, 0, 0, time.Local),
			time.Date(2026, 12, 31, 23, 59, 0, 0, time.Local),
		}
		for _, tm := range testTimes {
			if expr1.Matches(tm) != expr2.Matches(tm) {
				t.Errorf("%s 与 %s 在 %v 时匹配不一致", shortcut, expected, tm)
			}
		}
	}
}

// --- 调度器测试 ---

func setupTestDB(t *testing.T) (*Scheduler, func()) {
	t.Helper()
	tmpFile := t.TempDir() + "/test_cron.db"
	s := NewScheduler(tmpFile, t.TempDir())

	return s, func() {
		s.Close()
		os.Remove(tmpFile)
	}
}

func TestCreateJob(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	job, err := s.CreateJob("test", "*/5 * * * *", "echo hello")
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}

	if job.ID == "" {
		t.Error("任务 ID 不应为空")
	}
	if job.Name != "test" {
		t.Errorf("名称期望 test, 得到 %s", job.Name)
	}
	if !job.Enabled {
		t.Error("新任务应默认启用")
	}
	if job.NextRun == nil {
		t.Error("NextRun 不应为空")
	}
}

func TestCreateJobInvalidCron(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	_, err := s.CreateJob("bad", "invalid", "echo hello")
	if err == nil {
		t.Error("无效 cron 表达式应返回错误")
	}
}

func TestListJobs(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	// 空列表
	jobs, err := s.ListJobs()
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Errorf("空列表期望 0 个, 得到 %d", len(jobs))
	}

	// 创建两个任务
	s.CreateJob("job1", "* * * * *", "echo 1")
	s.CreateJob("job2", "@daily", "echo 2")

	jobs, err = s.ListJobs()
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Errorf("期望 2 个任务, 得到 %d", len(jobs))
	}
}

func TestGetJob(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	job, _ := s.CreateJob("detail-test", "0 9 * * *", "date")

	got, logs, err := s.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "detail-test" {
		t.Errorf("名称不匹配: %s", got.Name)
	}
	if len(logs) != 0 {
		t.Errorf("新任务不应有日志, 得到 %d 条", len(logs))
	}

	// 不存在的任务
	_, _, err = s.GetJob("nonexistent")
	if err == nil {
		t.Error("查询不存在的任务应返回错误")
	}
}

func TestUpdateJob(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	job, _ := s.CreateJob("original", "* * * * *", "echo old")

	// 更新名称和命令
	updated, err := s.UpdateJob(job.ID, map[string]interface{}{
		"name":    "renamed",
		"command": "echo new",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "renamed" {
		t.Errorf("名称期望 renamed, 得到 %s", updated.Name)
	}

	// 暂停任务
	paused, err := s.UpdateJob(job.ID, map[string]interface{}{
		"enabled": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if paused.Enabled {
		t.Error("任务应被暂停")
	}

	// 更新 cron 表达式
	_, err = s.UpdateJob(job.ID, map[string]interface{}{
		"schedule": "@hourly",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 无效 cron 表达式
	_, err = s.UpdateJob(job.ID, map[string]interface{}{
		"schedule": "invalid",
	})
	if err == nil {
		t.Error("无效 cron 应返回错误")
	}

	// 不存在的任务
	_, err = s.UpdateJob("nonexistent", map[string]interface{}{"name": "x"})
	if err == nil {
		t.Error("更新不存在的任务应返回错误")
	}
}

func TestDeleteJob(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	job, _ := s.CreateJob("to-delete", "* * * * *", "echo bye")

	if err := s.DeleteJob(job.ID); err != nil {
		t.Fatal(err)
	}

	// 确认已删除
	jobs, _ := s.ListJobs()
	if len(jobs) != 0 {
		t.Errorf("删除后应无任务, 有 %d 个", len(jobs))
	}

	// 重复删除
	if err := s.DeleteJob(job.ID); err == nil {
		t.Error("删除不存在的任务应返回错误")
	}
}

func TestIsDangerous(t *testing.T) {
	cases := []struct {
		cmd       string
		dangerous bool
	}{
		{"echo hello", false},
		{"ls -la", false},
		{"rm -rf /", true},
		{"dd if=/dev/zero of=/dev/sda", true},
		{"mkfs.ext4 /dev/sda", true},
	}

	for _, tc := range cases {
		if isDangerous(tc.cmd) != tc.dangerous {
			t.Errorf("isDangerous(%q) = %v, 期望 %v", tc.cmd, !tc.dangerous, tc.dangerous)
		}
	}
}

func TestLogExecution(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	job, _ := s.CreateJob("log-test", "* * * * *", "echo log")

	// 手动写入日志
	s.logExecution(job.ID, time.Now(), "output here", "", 42)

	_, logs, err := s.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("期望 1 条日志, 得到 %d", len(logs))
	}
	if logs[0].Output != "output here" {
		t.Errorf("日志输出不匹配: %s", logs[0].Output)
	}
	if logs[0].DurationMs != 42 {
		t.Errorf("耗时不匹配: %d", logs[0].DurationMs)
	}
}

func TestSchedulerStartStop(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	s.Start()
	// 重复启动不应 panic
	s.Start()

	s.Stop()
	// 重复停止不应 panic
	s.Stop()
}

func TestGenerateID(t *testing.T) {
	ids := make(map[string]bool)
	for range 100 {
		id := generateID()
		if !isValidID(id) {
			t.Errorf("无效 ID 格式: %s", id)
		}
		if ids[id] {
			t.Errorf("ID 重复: %s", id)
		}
		ids[id] = true
	}
}

func isValidID(id string) bool {
	return len(id) > 3 && id[:3] == "cj_"
}

// --- 直接 DB 操作测试 ---

func TestConcurrentDBAccess(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	// 并发创建任务
	done := make(chan error, 10)
	for i := range 10 {
		go func(n int) {
			_, err := s.CreateJob(
				fmt.Sprintf("concurrent-%d", n),
				"* * * * *",
				fmt.Sprintf("echo %d", n),
			)
			done <- err
		}(i)
	}

	for range 10 {
		if err := <-done; err != nil {
			t.Errorf("并发创建失败: %v", err)
		}
	}

	jobs, _ := s.ListJobs()
	if len(jobs) != 10 {
		t.Errorf("期望 10 个任务, 得到 %d", len(jobs))
	}
}

func TestListEnabled(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()

	j1, _ := s.CreateJob("enabled", "* * * * *", "echo 1")
	j2, _ := s.CreateJob("disabled", "* * * * *", "echo 2")

	// 禁用第二个
	s.UpdateJob(j2.ID, map[string]interface{}{"enabled": false})

	enabled, err := s.listEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 1 {
		t.Errorf("期望 1 个启用任务, 得到 %d", len(enabled))
	}
	if enabled[0].ID != j1.ID {
		t.Errorf("启用的应是 %s, 得到 %s", j1.ID, enabled[0].ID)
	}
}
