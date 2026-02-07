package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderMessages 渲染所有消息为气泡样式
func renderMessages(messages []Message, width int) string {
	var b strings.Builder
	maxBubble := width * 3 / 4 // 气泡最大宽度为屏幕 3/4
	if maxBubble < 30 {
		maxBubble = 30
	}

	for _, msg := range messages {
		switch {
		case msg.Role == "user":
			b.WriteString(renderUserBubble(msg.Content, width, maxBubble))
		case strings.Contains(msg.Content, "tool:"):
			b.WriteString(renderToolMessage(msg.Content, maxBubble))
		default:
			b.WriteString(renderAIBubble(msg.Content, msg.IsStream, maxBubble))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// renderUserBubble 渲染用户消息（右对齐气泡）
func renderUserBubble(content string, termWidth, maxBubble int) string {
	// 标签
	label := userLabelStyle.Render("You")
	// 气泡内容
	bubble := userBubbleStyle.
		MaxWidth(maxBubble).
		Render(content)

	// 计算右对齐
	bubbleWidth := lipgloss.Width(bubble)
	labelWidth := lipgloss.Width(label)

	pad := termWidth - bubbleWidth - 2
	if pad < 0 {
		pad = 0
	}
	labelPad := termWidth - labelWidth - 2
	if labelPad < 0 {
		labelPad = 0
	}

	return fmt.Sprintf("%s%s\n%s%s",
		strings.Repeat(" ", labelPad), label,
		strings.Repeat(" ", pad), bubble)
}

// renderAIBubble 渲染 AI 消息（左对齐气泡）
func renderAIBubble(content string, isStream bool, maxBubble int) string {
	label := aiLabelStyle.Render("Kele")
	displayContent := content
	if isStream {
		displayContent += "\u258b" // 闪烁光标
	}

	bubble := aiBubbleStyle.
		MaxWidth(maxBubble).
		Render(displayContent)

	return fmt.Sprintf("  %s\n  %s", label, bubble)
}

// renderToolMessage 渲染工具执行消息
func renderToolMessage(content string, maxBubble int) string {
	bubble := toolMessageStyle.
		MaxWidth(maxBubble).
		Render(content)
	return fmt.Sprintf("  %s", bubble)
}

// renderTabBar 渲染 Tab 栏
func renderTabBar(sessions []*Session, activeIdx, width int) string {
	var tabs []string
	for i, s := range sessions {
		label := fmt.Sprintf(" %d:%s ", i+1, s.name)
		if i == activeIdx {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}

	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)

	// 右侧填充状态信息
	tabWidth := lipgloss.Width(tabBar)
	remaining := width - tabWidth
	if remaining > 0 {
		filler := statusStyle.Width(remaining).Render("")
		tabBar = lipgloss.JoinHorizontal(lipgloss.Top, tabBar, filler)
	}

	return tabBar
}

// renderCompletionHint 渲染补全候选行
func renderCompletionHintLine(hint string, width int) string {
	emptyStyle := lipgloss.NewStyle().Width(width)

	if hint == "" {
		return emptyStyle.Render("")
	}

	prefix := "[Tab] "
	maxContent := width - len(prefix) - 2
	if len([]rune(hint)) > maxContent {
		hint = string([]rune(hint)[:maxContent-3]) + "..."
	}

	hintStyle := lipgloss.NewStyle().
		Width(width).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("117")).
		Padding(0, 1)

	prefixStyled := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("214")).
		Bold(true).
		Render(prefix)

	return hintStyle.Render(prefixStyled + hint)
}

// renderOverlay 渲染 Ctrl+O 设置叠加层
func renderOverlay(a *App, width, height int) string {
	sess := a.currentSession()

	// 标题
	title := overlayTitleStyle.Width(width).Render("  Kele Settings  (Ctrl+O to close)")

	// 内容
	var content strings.Builder
	content.WriteString(fmt.Sprintf("  Sessions (%d/%d)\n\n", len(a.sessions), maxSessions))
	for i, s := range a.sessions {
		marker := "  "
		if i == a.activeIdx {
			marker = "> "
		}
		content.WriteString(fmt.Sprintf("  %s%d: %s (%d msgs)\n", marker, i+1, s.name, len(s.messages)))
	}

	content.WriteString(fmt.Sprintf("\n  Model: %s\n", sess.brain.GetModel()))
	content.WriteString(fmt.Sprintf("  Small Model: %s\n", sess.brain.GetSmallModel()))
	content.WriteString(fmt.Sprintf("\n  Keybindings:\n"))
	content.WriteString("    Ctrl+]      Next session\n")
	content.WriteString("    Alt+1..9    Switch session\n")
	content.WriteString("    Ctrl+T      New session\n")
	content.WriteString("    Ctrl+W      Close session\n")
	content.WriteString("    Ctrl+J      Newline\n")
	content.WriteString("    Up/Down     Input history\n")
	content.WriteString("    Tab         Completion\n")
	content.WriteString("    Ctrl+C x2   Quit\n")
	content.WriteString("    ESC x2      Break tasks\n")

	body := overlayContentStyle.
		Width(width).
		Height(height - 2).
		Render(content.String())

	return lipgloss.JoinVertical(lipgloss.Left, title, body)
}
