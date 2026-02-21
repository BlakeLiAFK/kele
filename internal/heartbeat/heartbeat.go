package heartbeat

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BlakeLiAFK/kele/internal/llm"
	"github.com/BlakeLiAFK/kele/internal/tools"
)

// Record stores one heartbeat execution result.
type Record struct {
	Timestamp  time.Time
	Decision   string
	ActionsTaken int
	Duration   time.Duration
}

// Runner manages the heartbeat lifecycle.
type Runner struct {
	provider  *llm.ProviderManager
	executor  *tools.Executor
	interval  time.Duration
	active    bool
	done      chan struct{}
	mu        sync.Mutex

	// State
	totalHeartbeats int
	totalActions    int
	lastRun         time.Time
	lastDecision    string
	records         []Record

	// SessionCounter func for snapshot
	getSessionCount func() int
}

// NewRunner creates a new heartbeat runner.
func NewRunner(provider *llm.ProviderManager, executor *tools.Executor, getSessionCount func() int) *Runner {
	return &Runner{
		provider:        provider,
		executor:        executor,
		interval:        15 * time.Minute,
		done:            make(chan struct{}),
		getSessionCount: getSessionCount,
	}
}

// Start begins the heartbeat loop in a goroutine.
func (r *Runner) Start() {
	r.mu.Lock()
	r.active = true
	r.mu.Unlock()

	go r.loop()
	log.Println("Heartbeat started (interval: 15m)")
}

// Stop stops the heartbeat loop.
func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active {
		r.active = false
		close(r.done)
	}
}

// IsActive returns whether the heartbeat is running.
func (r *Runner) IsActive() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active
}

// IntervalMinutes returns the current interval in minutes.
func (r *Runner) IntervalMinutes() int {
	return int(r.interval.Minutes())
}

// LastRun returns the time of the last heartbeat.
func (r *Runner) LastRun() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastRun
}

// LastDecision returns the AI's last decision.
func (r *Runner) LastDecision() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastDecision
}

// TotalHeartbeats returns the total number of heartbeats executed.
func (r *Runner) TotalHeartbeats() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.totalHeartbeats
}

// TotalActions returns the total number of actions taken.
func (r *Runner) TotalActions() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.totalActions
}

// loop runs the heartbeat on a dynamic interval.
func (r *Runner) loop() {
	for {
		interval := r.getInterval()
		select {
		case <-time.After(interval):
			r.beat()
		case <-r.done:
			log.Println("Heartbeat stopped")
			return
		}
	}
}

// getInterval returns a dynamic interval based on time of day.
func (r *Runner) getInterval() time.Duration {
	hour := time.Now().Hour()

	// Night (23:00 - 07:00): slow heartbeat
	if hour >= 23 || hour < 7 {
		return 60 * time.Minute
	}

	// Work hours (09:00 - 18:00): normal
	if hour >= 9 && hour < 18 {
		return 15 * time.Minute
	}

	// Other times: moderate
	return 30 * time.Minute
}

// beat executes one heartbeat cycle.
func (r *Runner) beat() {
	start := time.Now()

	sessionCount := 0
	if r.getSessionCount != nil {
		sessionCount = r.getSessionCount()
	}

	snapshot := TakeSnapshot(sessionCount)

	// Read HEARTBEAT.md config
	heartbeatConfig := readHeartbeatConfig()

	// If no config, just record and skip
	if heartbeatConfig == "" {
		r.mu.Lock()
		r.totalHeartbeats++
		r.lastRun = start
		r.lastDecision = "no HEARTBEAT.md config found"
		r.mu.Unlock()
		log.Println("Heartbeat: no HEARTBEAT.md found, skipping")
		return
	}

	// Build prompt
	prompt := snapshot.FormatPrompt(heartbeatConfig)

	// Call LLM
	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	resp, err := r.provider.Chat(messages, r.executor.GetTools())
	if err != nil {
		log.Printf("Heartbeat LLM error: %v", err)
		r.mu.Lock()
		r.totalHeartbeats++
		r.lastRun = start
		r.lastDecision = fmt.Sprintf("error: %v", err)
		r.mu.Unlock()
		return
	}

	decision := ""
	actionsTaken := 0

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		decision = choice.Message.Content

		// Execute tool calls if any
		if len(choice.Message.ToolCalls) > 0 {
			for _, tc := range choice.Message.ToolCalls {
				result, err := r.executor.Execute(tc)
				if err != nil {
					log.Printf("Heartbeat tool %s error: %v", tc.Function.Name, err)
				} else {
					log.Printf("Heartbeat tool %s: %s", tc.Function.Name, truncate(result, 200))
					actionsTaken++
				}
			}
		}
	}

	duration := time.Since(start)

	r.mu.Lock()
	r.totalHeartbeats++
	r.totalActions += actionsTaken
	r.lastRun = start
	r.lastDecision = truncate(decision, 500)
	r.records = append(r.records, Record{
		Timestamp:    start,
		Decision:     decision,
		ActionsTaken: actionsTaken,
		Duration:     duration,
	})
	// Keep last 100 records
	if len(r.records) > 100 {
		r.records = r.records[len(r.records)-100:]
	}
	r.mu.Unlock()

	log.Printf("Heartbeat completed in %v (actions: %d, decision: %s)",
		duration, actionsTaken, truncate(decision, 100))
}

// readHeartbeatConfig reads HEARTBEAT.md from CWD or home dir.
func readHeartbeatConfig() string {
	// Try CWD first
	data, err := os.ReadFile("HEARTBEAT.md")
	if err == nil {
		return string(data)
	}

	// Try home directory
	homeDir, _ := os.UserHomeDir()
	data, err = os.ReadFile(filepath.Join(homeDir, "HEARTBEAT.md"))
	if err == nil {
		return string(data)
	}

	// Try ~/.kele/HEARTBEAT.md
	data, err = os.ReadFile(filepath.Join(homeDir, ".kele", "HEARTBEAT.md"))
	if err == nil {
		return string(data)
	}

	return ""
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
