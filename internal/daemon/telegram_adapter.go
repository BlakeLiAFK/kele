package daemon

import (
	"fmt"
	"strings"

	"github.com/BlakeLiAFK/kele/internal/config"
	"github.com/BlakeLiAFK/kele/internal/llm"
	"github.com/BlakeLiAFK/kele/internal/telegram"
)

// TelegramAdapter 桥接 SessionManager 到 telegram.SessionProvider 接口
type TelegramAdapter struct {
	sessions     *SessionManager
	cfg          *config.Config
	daemonStatus func() string // daemon 级别状态回调
}

// GetOrCreateSession 根据 chatID 获取或创建会话
func (a *TelegramAdapter) GetOrCreateSession(chatID int64) (string, error) {
	sessionID := fmt.Sprintf("telegram-%d", chatID)
	sess := a.sessions.Get(sessionID)
	if sess != nil {
		return sessionID, nil
	}
	// 创建新会话，使用固定 ID
	a.sessions.mu.Lock()
	defer a.sessions.mu.Unlock()

	// 双重检查
	if s, ok := a.sessions.sessions[sessionID]; ok && s != nil {
		return sessionID, nil
	}

	sess = &Session{
		ID:   sessionID,
		Name: fmt.Sprintf("Telegram %d", chatID),
		brain: &SessionBrain{
			provider: a.sessions.provider,
			executor: a.sessions.executor,
			memory:   a.sessions.memory,
			history:  []llm.Message{},
			cfg:      a.sessions.cfg,
		},
	}
	a.sessions.sessions[sessionID] = sess
	return sessionID, nil
}

// ChatStream 发起流式对话
func (a *TelegramAdapter) ChatStream(sessionID string, input string) (<-chan telegram.StreamEvent, error) {
	sess := a.sessions.Get(sessionID)
	if sess == nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	events, err := sess.brain.ChatStream(input)
	if err != nil {
		return nil, err
	}

	// 转换事件类型
	out := make(chan telegram.StreamEvent, 100)
	go func() {
		defer close(out)
		for ev := range events {
			out <- telegram.StreamEvent{
				Type:    ev.Type,
				Content: eventContent(ev),
			}
		}
	}()
	return out, nil
}

// RunCommand 执行斜杠命令
func (a *TelegramAdapter) RunCommand(sessionID string, command string) (string, bool, error) {
	// daemon 级别命令拦截
	parts := strings.Fields(command)
	if len(parts) > 0 && parts[0] == "/status" && a.daemonStatus != nil {
		return a.daemonStatus(), false, nil
	}

	sess := a.sessions.Get(sessionID)
	if sess == nil {
		return "", false, fmt.Errorf("session not found: %s", sessionID)
	}
	result, exit := sess.brain.RunCommand(command)
	return result, exit, nil
}

// eventContent 提取事件内容
func eventContent(ev ChatEvent) string {
	switch ev.Type {
	case "content", "thinking", "error":
		if ev.Content != "" {
			return ev.Content
		}
		return ev.Error
	case "tool_call":
		return ev.ToolName
	case "tool_result":
		return ev.ToolResult
	default:
		return ""
	}
}
