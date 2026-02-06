package tui

import "github.com/charmbracelet/lipgloss"

var (
	// 状态栏样式
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("63")).
			Padding(0, 1).
			Bold(true)

	// Tab 样式（活跃）
	activeTabStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("63")).
			Padding(0, 1).
			Bold(true)

	// Tab 样式（非活跃）
	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("250")).
				Background(lipgloss.Color("238")).
				Padding(0, 1)

	// 用户消息气泡
	userBubbleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("62")).
			Padding(0, 1).
			MarginLeft(4)

	// 用户名标签
	userLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62")).
			Bold(true)

	// AI 消息气泡
	aiBubbleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("236")).
			Padding(0, 1).
			MarginRight(4)

	// AI 名标签
	aiLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Bold(true)

	// 工具消息样式
	toolMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Background(lipgloss.Color("235")).
				Padding(0, 1).
				MarginLeft(2)

	// 帮助文本样式
	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 1)

	// 分隔线样式
	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	// Ctrl+O 叠加层标题
	overlayTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("62")).
				Padding(0, 1).
				Bold(true)

	// Ctrl+O 叠加层内容
	overlayContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Background(lipgloss.Color("235")).
				Padding(1, 2)
)
