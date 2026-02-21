package heartbeat

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

// Snapshot captures system state at a point in time.
type Snapshot struct {
	Time       TimeInfo   `json:"time"`
	System     SystemInfo `json:"system"`
	Context    CtxInfo    `json:"context"`
}

// TimeInfo time-related snapshot data.
type TimeInfo struct {
	Current   time.Time `json:"current"`
	DayOfWeek string    `json:"day_of_week"`
	Hour      int       `json:"hour"`
}

// SystemInfo system resource snapshot.
type SystemInfo struct {
	GoRoutines int    `json:"go_routines"`
	MemAllocMB float64 `json:"mem_alloc_mb"`
	NumCPU     int    `json:"num_cpu"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
}

// CtxInfo context-related info.
type CtxInfo struct {
	ActiveSessions int `json:"active_sessions"`
	CWD            string `json:"cwd"`
}

// TakeSnapshot captures current system state.
func TakeSnapshot(activeSessions int) Snapshot {
	now := time.Now()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	cwd, _ := os.Getwd()

	return Snapshot{
		Time: TimeInfo{
			Current:   now,
			DayOfWeek: now.Weekday().String(),
			Hour:      now.Hour(),
		},
		System: SystemInfo{
			GoRoutines: runtime.NumGoroutine(),
			MemAllocMB: float64(memStats.Alloc) / 1024 / 1024,
			NumCPU:     runtime.NumCPU(),
			OS:         runtime.GOOS,
			Arch:       runtime.GOARCH,
		},
		Context: CtxInfo{
			ActiveSessions: activeSessions,
			CWD:            cwd,
		},
	}
}

// FormatPrompt formats the snapshot into a prompt string for the AI.
func (s Snapshot) FormatPrompt(heartbeatConfig string) string {
	var b strings.Builder

	b.WriteString("你是 Kele 智能助手。现在是系统心跳时刻。\n\n")
	b.WriteString(fmt.Sprintf("当前时间: %s\n", s.Time.Current.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("星期: %s\n", s.Time.DayOfWeek))
	b.WriteString(fmt.Sprintf("小时: %d\n\n", s.Time.Hour))

	b.WriteString("系统状态:\n")
	b.WriteString(fmt.Sprintf("- Go 协程数: %d\n", s.System.GoRoutines))
	b.WriteString(fmt.Sprintf("- 内存使用: %.2f MB\n", s.System.MemAllocMB))
	b.WriteString(fmt.Sprintf("- CPU 核数: %d\n", s.System.NumCPU))
	b.WriteString(fmt.Sprintf("- 系统: %s/%s\n\n", s.System.OS, s.System.Arch))

	b.WriteString(fmt.Sprintf("活跃会话: %d\n", s.Context.ActiveSessions))
	b.WriteString(fmt.Sprintf("工作目录: %s\n\n", s.Context.CWD))

	if heartbeatConfig != "" {
		b.WriteString("根据以下心跳配置，判断现在是否需要执行某些任务：\n\n")
		b.WriteString(heartbeatConfig)
		b.WriteString("\n\n")
	}

	b.WriteString("如果有需要执行的任务，请说明并执行。如果没有，回复\"无需行动\"。")

	return b.String()
}
