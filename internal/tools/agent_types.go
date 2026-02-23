package tools

import "time"

// AgentSpawner 子 agent 调度接口
// 由 agent.WorkerPool 实现，工具层通过此接口操作子 agent
type AgentSpawner interface {
	Spawn(task string) (string, error)
	Status(id string) (AgentInfo, error)
	ListAll() []AgentInfo
	Result(id string, timeout time.Duration) (string, error)
	RecentLogs(id string, n int) ([]AgentLogEntry, error)
}

// AgentInfo 子 agent 状态信息
type AgentInfo struct {
	ID        string    `json:"id"`
	Task      string    `json:"task"`
	Status    string    `json:"status"` // pending/running/completed/failed
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time,omitempty"`
	LogCount  int       `json:"log_count"`
	Result    string    `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// AgentLogEntry 子 agent 日志条目
type AgentLogEntry struct {
	Time    time.Time `json:"time"`
	Type    string    `json:"type"`
	Content string    `json:"content"`
}
