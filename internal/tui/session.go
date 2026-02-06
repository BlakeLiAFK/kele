package tui

import (
	"fmt"

	"github.com/BlakeLiAFK/kele/internal/agent"
)

// Session 独立的聊天会话
type Session struct {
	id           int
	name         string
	messages     []Message
	brain        *agent.Brain
	streaming    bool
	streamBuffer string
	eventChan    <-chan streamEvent

	// 输入历史（Up/Down 导航）
	inputHistory []string
	historyIdx   int // -1 表示当前输入，0..n 表示历史
	savedInput   string // 按 Up 前暂存当前输入

	// 任务链
	taskRunning bool
}

// NewSession 创建新会话
func NewSession(id int) *Session {
	return &Session{
		id:         id,
		name:       fmt.Sprintf("Chat %d", id),
		messages:   []Message{},
		brain:      agent.NewBrain(),
		historyIdx: -1,
	}
}

// AddMessage 添加消息
func (s *Session) AddMessage(role, content string) {
	s.messages = append(s.messages, Message{
		Role:    role,
		Content: content,
	})
}

// PushHistory 保存用户输入到历史
func (s *Session) PushHistory(input string) {
	if input == "" {
		return
	}
	// 去重：与上一条相同则不追加
	if len(s.inputHistory) > 0 && s.inputHistory[len(s.inputHistory)-1] == input {
		return
	}
	s.inputHistory = append(s.inputHistory, input)
	// 限制历史长度
	if len(s.inputHistory) > 200 {
		s.inputHistory = s.inputHistory[len(s.inputHistory)-200:]
	}
	s.historyIdx = -1
	s.savedInput = ""
}

// HistoryUp 向上浏览历史，返回要填充的文本（空字符串表示无更多历史）
func (s *Session) HistoryUp(currentInput string) (string, bool) {
	if len(s.inputHistory) == 0 {
		return "", false
	}
	if s.historyIdx == -1 {
		// 首次按 Up，保存当前输入
		s.savedInput = currentInput
		s.historyIdx = len(s.inputHistory) - 1
	} else if s.historyIdx > 0 {
		s.historyIdx--
	} else {
		return s.inputHistory[0], false // 已到顶
	}
	return s.inputHistory[s.historyIdx], true
}

// HistoryDown 向下浏览历史
func (s *Session) HistoryDown() (string, bool) {
	if s.historyIdx == -1 {
		return "", false
	}
	if s.historyIdx < len(s.inputHistory)-1 {
		s.historyIdx++
		return s.inputHistory[s.historyIdx], true
	}
	// 回到最底部，恢复之前的输入
	s.historyIdx = -1
	return s.savedInput, true
}

// ResetHistoryNav 重置历史导航状态
func (s *Session) ResetHistoryNav() {
	s.historyIdx = -1
	s.savedInput = ""
}
