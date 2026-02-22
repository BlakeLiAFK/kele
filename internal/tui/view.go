package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderMessages 渲染所有消息为气泡样式
func renderMessages(messages []Message, width int, thinkingExpanded bool, spinnerFrame int) string {
	var b strings.Builder
	maxBubble := width * 3 / 4
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
			b.WriteString(renderAIBubble(msg, thinkingExpanded, spinnerFrame, maxBubble))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// renderUserBubble 渲染用户消息（右对齐气泡）
func renderUserBubble(content string, termWidth, maxBubble int) string {
	label := userLabelStyle.Render("You")
	bubble := userBubbleStyle.
		MaxWidth(maxBubble).
		Render(content)

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

// renderAIBubble 渲染 AI 消息（左对齐气泡，含 Thinking 块）
func renderAIBubble(msg Message, thinkingExpanded bool, spinnerFrame int, maxBubble int) string {
	var parts []string

	label := aiLabelStyle.Render("Kele")
	parts = append(parts, fmt.Sprintf("  %s", label))

	// 渲染 Thinking 块（content 开始后 thinking 动画停止）
	thinkingActive := msg.IsStream && msg.Content == ""
	if msg.Thinking != "" || thinkingActive {
		thinkingBlock := renderThinkingBlock(msg.Thinking, thinkingActive, thinkingExpanded, spinnerFrame, maxBubble)
		parts = append(parts, thinkingBlock)
	}

	// 渲染内容
	displayContent := msg.Content
	if msg.IsStream && displayContent != "" {
		displayContent += "\u258b"
	}

	if displayContent != "" {
		bubble := aiBubbleStyle.
			MaxWidth(maxBubble).
			Render(displayContent)
		parts = append(parts, fmt.Sprintf("  %s", bubble))
	}

	return strings.Join(parts, "\n")
}

// renderThinkingBlock 渲染思考过程块
func renderThinkingBlock(thinking string, isStreaming, expanded bool, spinnerFrame int, maxBubble int) string {
	spinner := spinnerFrames[spinnerFrame%len(spinnerFrames)]

	// 流式中：始终展开，显示 spinner
	if isStreaming {
		if thinking == "" {
			// 等待推理内容，只显示动画
			label := thinkingLabelStyle.Render(fmt.Sprintf("  [%s Thinking...]", spinner))
			return label
		}
		// 有推理内容，展开显示
		label := thinkingLabelStyle.Render(fmt.Sprintf("  [%s Thinking]", spinner))
		content := renderThinkingContent(thinking, maxBubble)
		return label + "\n" + content
	}

	// 已完成：根据 expanded 状态决定展开/折叠
	if thinking == "" {
		return ""
	}

	lines := strings.Split(strings.TrimRight(thinking, "\n"), "\n")
	lineCount := len(lines)

	if expanded {
		// 展开：显示全部内容
		label := thinkingLabelStyle.Render(fmt.Sprintf("  [Thinking] (%d%s, Ctrl+E %s)", lineCount, "\u884c", "\u6298\u53e0"))
		content := renderThinkingContent(thinking, maxBubble)
		return label + "\n" + content
	}

	// 折叠：显示最后一行摘要
	lastLine := lines[lineCount-1]
	if len([]rune(lastLine)) > 40 {
		lastLine = string([]rune(lastLine)[:40]) + "..."
	}
	label := thinkingLabelStyle.Render(
		fmt.Sprintf("  [Thinking] ...%s (%d%s, Ctrl+E %s)", lastLine, lineCount, "\u884c", "\u5c55\u5f00"))
	return label
}

// renderThinkingContent 渲染思考内容文本
func renderThinkingContent(thinking string, maxBubble int) string {
	lines := strings.Split(strings.TrimRight(thinking, "\n"), "\n")
	var rendered []string
	for _, line := range lines {
		styled := thinkingContentStyle.
			MaxWidth(maxBubble).
			Render(line)
		rendered = append(rendered, styled)
	}
	return strings.Join(rendered, "\n")
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

	tabWidth := lipgloss.Width(tabBar)
	remaining := width - tabWidth
	if remaining > 0 {
		filler := statusStyle.Width(remaining).Render("")
		tabBar = lipgloss.JoinHorizontal(lipgloss.Top, tabBar, filler)
	}

	return tabBar
}

// renderCompletionHintLine 渲染补全候选行 + 补全状态
func renderCompletionHintLine(hint, completionState, completionError string, spinnerFrame int, width int) string {
	emptyStyle := lipgloss.NewStyle().Width(width)

	// 补全状态指示器
	statusIndicator := renderCompletionStatus(completionState, completionError, spinnerFrame)

	if hint == "" && statusIndicator == "" {
		return emptyStyle.Render("")
	}

	// 只有状态没有提示
	if hint == "" {
		return emptyStyle.Render(statusIndicator)
	}

	// 有提示：左侧提示 + 右侧状态
	prefix := "[Tab] "
	statusWidth := lipgloss.Width(statusIndicator)
	maxContent := width - len(prefix) - statusWidth - 4
	if maxContent < 10 {
		maxContent = 10
	}
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

	left := prefixStyled + hint
	if statusIndicator != "" {
		// 计算右对齐填充
		leftWidth := lipgloss.Width(prefixStyled) + len([]rune(hint))
		padLen := width - leftWidth - statusWidth - 4
		if padLen < 1 {
			padLen = 1
		}
		left = left + strings.Repeat(" ", padLen) + statusIndicator
	}

	return hintStyle.Render(left)
}

// renderCompletionStatus 渲染补全状态指示器
func renderCompletionStatus(state, errMsg string, spinnerFrame int) string {
	switch state {
	case "loading":
		spinner := spinnerFrames[spinnerFrame%len(spinnerFrames)]
		return completionStatusStyle.Render(fmt.Sprintf("[AI: %s %s]", spinner, "\u8bf7\u6c42\u4e2d"))
	case "error":
		short := errMsg
		if len([]rune(short)) > 20 {
			short = string([]rune(short)[:20]) + "..."
		}
		return completionErrorStyle.Render(fmt.Sprintf("[AI: %s %s]", "\u5931\u8d25", short))
	case "done":
		return completionStatusStyle.Render("[AI: OK]")
	default:
		return ""
	}
}

// renderOverlay 渲染 Ctrl+O 设置叠加层
func renderOverlay(a *App, width, height int) string {
	sess := a.currentSession()

	title := overlayTitleStyle.Width(width).Render("  Kele Settings  (Ctrl+O to close)")

	var content strings.Builder
	content.WriteString(fmt.Sprintf("  Sessions (%d/%d)\n\n", len(a.sessions), a.cfg.TUI.MaxSessions))
	for i, s := range a.sessions {
		marker := "  "
		if i == a.activeIdx {
			marker = "> "
		}
		content.WriteString(fmt.Sprintf("  %s%d: %s (%d msgs)\n", marker, i+1, s.name, len(s.messages)))
	}

	if sess.brain != nil {
		content.WriteString(fmt.Sprintf("\n  Provider: %s\n", sess.brain.GetProviderName()))
		content.WriteString(fmt.Sprintf("  Model: %s\n", sess.brain.GetModel()))
		content.WriteString(fmt.Sprintf("  Small Model: %s\n", sess.brain.GetSmallModel()))
	} else {
		provider := sess.provider
		model := sess.model
		if provider == "" {
			provider = "daemon"
		}
		if model == "" {
			model = "unknown"
		}
		content.WriteString(fmt.Sprintf("\n  Mode: daemon\n"))
		content.WriteString(fmt.Sprintf("  Provider: %s\n", provider))
		content.WriteString(fmt.Sprintf("  Model: %s\n", model))
	}
	content.WriteString(fmt.Sprintf("\n  Keybindings:\n"))
	content.WriteString("    Ctrl+N      Next session\n")
	content.WriteString("    Ctrl+P      Prev session\n")
	content.WriteString("    Ctrl+T      New session\n")
	content.WriteString("    Ctrl+W      Close session\n")
	content.WriteString("    Ctrl+E      Toggle thinking\n")
	content.WriteString("    Ctrl+G      Toggle mouse mode\n")
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
