package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/BlakeLiAFK/kele/internal/tui"
)

func main() {
	// 创建 TUI 应用
	app := tui.NewApp()

	// 运行程序
	p := tea.NewProgram(
		app,
		tea.WithAltScreen(),       // 使用备用屏幕
		tea.WithMouseCellMotion(), // 启用鼠标支持
	)

	if _, err := p.Run(); err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}
}
