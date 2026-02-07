package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// 双击检测阈值
const doublePressThreshold = 2 * time.Second

// handleKeyMsg 处理按键事件，返回是否已消费该事件
func (a *App) handleKeyMsg(keyMsg tea.KeyMsg) (consumed bool, cmd tea.Cmd) {
	sess := a.currentSession()

	// Ctrl+O: 切换设置面板
	if keyMsg.Type == tea.KeyCtrlO {
		if a.overlayMode == "settings" {
			a.overlayMode = ""
		} else {
			a.overlayMode = "settings"
		}
		return true, nil
	}

	// 设置面板模式下，大部分按键关闭面板
	if a.overlayMode == "settings" {
		if keyMsg.Type == tea.KeyEsc {
			a.overlayMode = ""
			return true, nil
		}
		// Alt+数字在设置面板中也可切换
		if keyMsg.Alt && keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) > 0 {
			if idx := runeToSessionIdx(keyMsg.Runes[0]); idx >= 0 && idx < len(a.sessions) {
				a.switchSession(idx)
				a.overlayMode = ""
				return true, nil
			}
		}
		return true, nil // 消费所有按键
	}

	switch {
	// Ctrl+]: 切换到下一个会话（循环）
	case keyMsg.Type == tea.KeyCtrlCloseBracket:
		if len(a.sessions) > 1 {
			next := (a.activeIdx + 1) % len(a.sessions)
			a.switchSession(next)
		}
		return true, nil

	// Ctrl+C: 双击退出
	case keyMsg.Type == tea.KeyCtrlC:
		now := time.Now()
		if now.Sub(a.lastCtrlC) < doublePressThreshold {
			return true, tea.Quit
		}
		a.lastCtrlC = now
		a.updateStatus("再按一次 Ctrl+C 退出")
		return true, nil

	// ESC: 中断流式/清补全，双击打断任务链
	case keyMsg.Type == tea.KeyEsc:
		now := time.Now()
		if sess.streaming {
			sess.streaming = false
			sess.eventChan = nil
			if len(sess.messages) > 0 && sess.messages[len(sess.messages)-1].IsStream {
				sess.messages[len(sess.messages)-1].Content = sess.streamBuffer + "\n\n[已中断]"
				sess.messages[len(sess.messages)-1].IsStream = false
			}
			sess.streamBuffer = ""
			a.refreshViewport()
			a.updateStatus("任务已中断")
			a.lastEsc = now
			return true, nil
		}
		// 双击 ESC: 打断任务链
		if sess.taskRunning && now.Sub(a.lastEsc) < doublePressThreshold {
			sess.taskRunning = false
			sess.streaming = false
			sess.eventChan = nil
			sess.streamBuffer = ""
			for i := range sess.messages {
				if sess.messages[i].IsStream {
					sess.messages[i].IsStream = false
				}
			}
			sess.AddMessage("assistant", "[任务链已打断]")
			a.refreshViewport()
			a.updateStatus("任务链已打断")
			return true, nil
		}
		a.lastEsc = now
		a.completionHint = ""
		a.suggestion = ""
		return true, nil

	// Tab: 接受补全
	case keyMsg.Type == tea.KeyTab:
		if sess.streaming {
			return true, nil
		}
		if a.suggestion != "" {
			a.textarea.SetValue(a.suggestion)
			a.textarea.CursorEnd()
			a.suggestion = ""
			a.completionHint = ""
			return true, nil
		}
		return true, nil

	// Ctrl+J: 换行
	case keyMsg.Type == tea.KeyCtrlJ:
		a.textarea.InsertString("\n")
		return true, nil

	// Ctrl+T: 新建会话
	case keyMsg.Type == tea.KeyCtrlT:
		a.createSession("")
		return true, nil

	// Ctrl+W: 关闭当前会话
	case keyMsg.Type == tea.KeyCtrlW:
		a.closeSession()
		return true, nil

	// Alt+数字: 切换会话
	case keyMsg.Alt && keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) > 0:
		if idx := runeToSessionIdx(keyMsg.Runes[0]); idx >= 0 {
			if idx < len(a.sessions) {
				a.switchSession(idx)
				return true, nil
			}
		}
		// 其他 Alt 组合不消费，传给 textarea

	// Up: 浏览历史
	case keyMsg.Type == tea.KeyUp:
		if !sess.streaming {
			text, ok := sess.HistoryUp(a.textarea.Value())
			if ok {
				a.textarea.SetValue(text)
				a.textarea.CursorEnd()
				return true, nil
			}
		}

	// Down: 浏览历史
	case keyMsg.Type == tea.KeyDown:
		if !sess.streaming && sess.historyIdx >= 0 {
			text, ok := sess.HistoryDown()
			if ok {
				a.textarea.SetValue(text)
				a.textarea.CursorEnd()
				return true, nil
			}
		}

	// Enter: 发送消息
	case keyMsg.Type == tea.KeyEnter:
		return true, a.handleEnter()
	}

	return false, nil
}

// runeToSessionIdx 将数字字符转为会话索引（0-based）
func runeToSessionIdx(r rune) int {
	if r >= '1' && r <= '9' {
		return int(r - '1')
	}
	return -1
}
