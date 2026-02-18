package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditLogger 审计日志记录器
type AuditLogger struct {
	logPath string
	mu      sync.Mutex
}

// AuditEntry 审计日志条目
type AuditEntry struct {
	Timestamp  string `json:"timestamp"`
	Tool       string `json:"tool"`
	Args       string `json:"args"`
	Result     string `json:"result"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// NewAuditLogger 创建审计日志记录器
func NewAuditLogger(logPath string) *AuditLogger {
	if logPath == "" {
		return nil
	}
	os.MkdirAll(filepath.Dir(logPath), 0755)
	return &AuditLogger{logPath: logPath}
}

// Log 记录一次工具调用
func (a *AuditLogger) Log(toolName string, args map[string]interface{}, result string, err error, duration time.Duration) {
	if a == nil {
		return
	}

	entry := AuditEntry{
		Timestamp:  time.Now().Format(time.RFC3339),
		Tool:       toolName,
		Args:       summarizeArgs(args),
		Result:     summarizeResult(result),
		DurationMs: duration.Milliseconds(),
	}
	if err != nil {
		entry.Error = err.Error()
	}

	line, jsonErr := json.Marshal(entry)
	if jsonErr != nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	f, openErr := os.OpenFile(a.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if openErr != nil {
		return
	}
	defer f.Close()
	f.Write(line)
	f.Write([]byte("\n"))
}

func summarizeArgs(args map[string]interface{}) string {
	if args == nil {
		return "{}"
	}
	// 截断过长的参数值
	summary := make(map[string]interface{})
	for k, v := range args {
		if s, ok := v.(string); ok && len(s) > 200 {
			summary[k] = s[:200] + "..."
		} else {
			summary[k] = v
		}
	}
	b, err := json.Marshal(summary)
	if err != nil {
		return fmt.Sprintf("%v", args)
	}
	return string(b)
}

func summarizeResult(result string) string {
	if len(result) > 500 {
		return result[:500] + "..."
	}
	return result
}
