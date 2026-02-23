package agent

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BlakeLiAFK/kele/internal/config"
	"github.com/BlakeLiAFK/kele/internal/llm"
	"github.com/BlakeLiAFK/kele/internal/prompt"
	"github.com/BlakeLiAFK/kele/internal/tools"
)

// agentToolBlacklist 子 agent 不应使用的工具名（防止递归）
var agentToolBlacklist = map[string]bool{
	"spawn_agent":  true,
	"agent_status": true,
	"agent_result": true,
}

// WorkerStatus 工作状态
type WorkerStatus string

const (
	StatusPending   WorkerStatus = "pending"
	StatusRunning   WorkerStatus = "running"
	StatusCompleted WorkerStatus = "completed"
	StatusFailed    WorkerStatus = "failed"
)

// Worker 子 agent 执行器
type Worker struct {
	id        string
	task      string
	status    WorkerStatus
	result    string
	errMsg    string
	log       []tools.AgentLogEntry
	startTime time.Time
	endTime   time.Time

	provider *llm.ProviderManager
	executor *tools.Executor
	cfg      *config.Config

	done chan struct{} // 完成信号
	mu   sync.RWMutex
}

// WorkerPool 子 agent 管理池
type WorkerPool struct {
	workers map[string]*Worker
	counter atomic.Int64
	mu      sync.RWMutex

	// 共享资源引用
	provider *llm.ProviderManager
	executor *tools.Executor
	cfg      *config.Config

	maxConcurrent int
	running       int  // 受 mu 保护
	stopped       bool // 受 mu 保护
}

// 确保 WorkerPool 实现 tools.AgentSpawner
var _ tools.AgentSpawner = (*WorkerPool)(nil)

// NewWorkerPool 创建 worker 管理池
func NewWorkerPool(provider *llm.ProviderManager, executor *tools.Executor, cfg *config.Config) *WorkerPool {
	return &WorkerPool{
		workers:       make(map[string]*Worker),
		provider:      provider,
		executor:      executor,
		cfg:           cfg,
		maxConcurrent: 5,
	}
}

// Spawn 启动新的子 agent
func (p *WorkerPool) Spawn(task string) (string, error) {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return "", fmt.Errorf("agent pool 已关闭")
	}
	if p.running >= p.maxConcurrent {
		p.mu.Unlock()
		return "", fmt.Errorf("并发子 agent 数已达上限 (%d)", p.maxConcurrent)
	}

	id := fmt.Sprintf("w%d", p.counter.Add(1))
	w := &Worker{
		id:       id,
		task:     task,
		status:   StatusPending,
		provider: p.provider,
		executor: p.executor,
		cfg:      p.cfg,
		done:     make(chan struct{}),
	}
	p.workers[id] = w
	p.running++
	p.mu.Unlock()

	go func() {
		defer func() {
			p.mu.Lock()
			p.running--
			p.mu.Unlock()
		}()
		w.run()
	}()

	return id, nil
}

// Shutdown 优雅关闭，等待运行中的 worker 完成
func (p *WorkerPool) Shutdown(timeout time.Duration) {
	p.mu.Lock()
	p.stopped = true
	p.mu.Unlock()

	deadline := time.After(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return
		case <-ticker.C:
			p.mu.RLock()
			n := p.running
			p.mu.RUnlock()
			if n == 0 {
				return
			}
		}
	}
}

// Status 查看子 agent 状态
func (p *WorkerPool) Status(id string) (tools.AgentInfo, error) {
	p.mu.RLock()
	w, ok := p.workers[id]
	p.mu.RUnlock()
	if !ok {
		return tools.AgentInfo{}, fmt.Errorf("子 agent 不存在: %s", id)
	}
	return w.info(), nil
}

// ListAll 列出所有子 agent
func (p *WorkerPool) ListAll() []tools.AgentInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var result []tools.AgentInfo
	for _, w := range p.workers {
		result = append(result, w.info())
	}
	return result
}

// Result 等待并获取子 agent 结果（阻塞直到完成或超时）
func (p *WorkerPool) Result(id string, timeout time.Duration) (string, error) {
	p.mu.RLock()
	w, ok := p.workers[id]
	p.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("子 agent 不存在: %s", id)
	}

	select {
	case <-w.done:
	case <-time.After(timeout):
		return "", fmt.Errorf("等待子 agent %s 超时 (%v)", id, timeout)
	}

	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.status == StatusFailed {
		return "", fmt.Errorf("子 agent 执行失败: %s", w.errMsg)
	}
	return w.result, nil
}

// RecentLogs 获取最近的日志条目
func (p *WorkerPool) RecentLogs(id string, n int) ([]tools.AgentLogEntry, error) {
	p.mu.RLock()
	w, ok := p.workers[id]
	p.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("子 agent 不存在: %s", id)
	}

	w.mu.RLock()
	defer w.mu.RUnlock()
	if n <= 0 || n > len(w.log) {
		n = len(w.log)
	}
	start := len(w.log) - n
	result := make([]tools.AgentLogEntry, n)
	copy(result, w.log[start:])
	return result, nil
}

// --- Worker 内部方法 ---

func (w *Worker) run() {
	defer close(w.done)
	defer func() {
		if r := recover(); r != nil {
			w.finish(StatusFailed, "", fmt.Sprintf("panic: %v", r))
		}
	}()

	w.mu.Lock()
	w.status = StatusRunning
	w.startTime = time.Now()
	w.mu.Unlock()

	w.appendLog("info", fmt.Sprintf("子 agent 启动，任务: %s", w.task))

	// 构建初始消息
	systemPrompt := w.buildSystemPrompt()
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: w.task},
	}

	// 获取过滤后的工具列表（排除 agent 相关工具）
	filteredTools := w.filterTools(w.executor.GetTools())

	maxRounds := w.cfg.LLM.MaxToolRounds
	if maxRounds <= 0 {
		maxRounds = 20
	}

	var finalContent string

	for round := 0; round < maxRounds; round++ {
		events := w.provider.ChatStream(messages, filteredTools)

		var roundContent string
		var pendingToolCalls []llm.ToolCall
		gotToolCalls := false

		for event := range events {
			switch event.Type {
			case "content":
				roundContent += event.Content
			case "tool_calls":
				gotToolCalls = true
				pendingToolCalls = event.ToolCalls
			case "error":
				errMsg := "unknown error"
				if event.Error != nil {
					errMsg = event.Error.Error()
				}
				w.appendLog("error", errMsg)
				w.finish(StatusFailed, "", errMsg)
				return
			case "done":
				if roundContent != "" {
					finalContent = roundContent
					w.appendLog("content", truncate(roundContent, 500))
				}
				w.finish(StatusCompleted, finalContent, "")
				return
			}
		}

		if gotToolCalls {
			if roundContent != "" {
				w.appendLog("content", truncate(roundContent, 200))
			}

			assistantMsg := llm.Message{
				Role:      "assistant",
				Content:   roundContent,
				ToolCalls: pendingToolCalls,
			}
			messages = append(messages, assistantMsg)

			for _, tc := range pendingToolCalls {
				w.appendLog("tool_call", tc.Function.Name)

				result, err := w.executor.Execute(tc)
				if err != nil {
					result = fmt.Sprintf("Error: %v", err)
				}
				result = compressOutput(result, w.cfg.Tools.MaxOutputSize)

				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
				})

				w.appendLog("tool_result", fmt.Sprintf("%s: %s", tc.Function.Name, truncate(result, 200)))
			}
			continue
		}

		// 没有工具调用也没有 done 事件，视为完成
		if roundContent != "" {
			finalContent = roundContent
			w.appendLog("content", truncate(roundContent, 500))
		}
		w.finish(StatusCompleted, finalContent, "")
		return
	}

	// 达到最大轮数
	w.appendLog("info", "达到最大工具调用轮数")
	w.finish(StatusCompleted, finalContent, "")
}

func (w *Worker) buildSystemPrompt() string {
	base := prompt.Build(prompt.BuildParams{
		ToolNames: w.filterToolNames(w.executor.ListTools()),
		WorkDir:   w.executor.GetWorkDir(),
	})

	return base + `
## 子 Agent 角色
你是一个子 agent，负责独立完成分配给你的具体任务。
- 专注于任务本身，不要询问用户
- 完成后在最终回复中总结你做了什么
- 遇到错误时尝试修复，无法修复则说明原因
`
}

// filterTools 过滤掉子 agent 不应使用的工具
func (w *Worker) filterTools(allTools []llm.Tool) []llm.Tool {
	var filtered []llm.Tool
	for _, t := range allTools {
		if !agentToolBlacklist[t.Function.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// filterToolNames 过滤工具名列表
func (w *Worker) filterToolNames(names []string) []string {
	var filtered []string
	for _, name := range names {
		if !agentToolBlacklist[name] {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

func (w *Worker) finish(status WorkerStatus, result, errMsg string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.status = status
	w.result = result
	w.errMsg = errMsg
	w.endTime = time.Now()
	w.appendLogLocked("done", fmt.Sprintf("状态: %s, 耗时: %v", status, w.endTime.Sub(w.startTime).Round(time.Millisecond)))
}

func (w *Worker) appendLog(typ, content string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.appendLogLocked(typ, content)
}

func (w *Worker) appendLogLocked(typ, content string) {
	w.log = append(w.log, tools.AgentLogEntry{
		Time:    time.Now(),
		Type:    typ,
		Content: content,
	})
}

func (w *Worker) info() tools.AgentInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return tools.AgentInfo{
		ID:        w.id,
		Task:      truncate(w.task, 100),
		Status:    string(w.status),
		StartTime: w.startTime,
		EndTime:   w.endTime,
		LogCount:  len(w.log),
		Result:    truncate(w.result, 500),
		Error:     w.errMsg,
	}
}

// --- 工具函数 ---

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func compressOutput(output string, maxSize int) string {
	if maxSize <= 0 {
		maxSize = 51200
	}
	if len(output) > maxSize {
		output = output[:maxSize] + fmt.Sprintf("\n\n... [输出被截断，原始 %d 字节]", len(output))
	}
	compressThreshold := 2048
	if len(output) > compressThreshold {
		headSize := compressThreshold * 3 / 4
		tailSize := compressThreshold / 4
		omitted := len(output) - headSize - tailSize
		output = output[:headSize] +
			fmt.Sprintf("\n\n... [省略 %d 字节] ...\n\n", omitted) +
			output[len(output)-tailSize:]
	}
	return output
}
