package tui

import "github.com/charmbracelet/lipgloss"

var (
	// 状态栏样式
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("63")).
			Padding(0, 1).
			Bold(true)

	// 用户消息样式
	userMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("86")).
				Padding(0, 2).
				Bold(true)

	// AI 消息样式
	assistantMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("212")).
				Padding(0, 2)

	// 工具消息样式
	toolMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Padding(0, 2).
				Background(lipgloss.Color("235"))

	// 帮助文本样式
	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 1)
)
