package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/BlakeLiAFK/kele/internal/config"
	"github.com/BlakeLiAFK/kele/internal/tui"
)

func main() {
	// CLI 参数
	version := flag.Bool("version", false, "显示版本号")
	model := flag.String("model", "", "指定初始模型")
	debug := flag.Bool("debug", false, "启用调试模式")
	flag.Parse()

	// 版本号
	if *version {
		fmt.Printf("Kele v%s\n", config.Version)
		os.Exit(0)
	}

	// 加载配置
	cfg := config.Load()
	cfg.ApplyFlags(*model, *debug)

	// 创建 TUI 应用
	app := tui.NewApp(cfg)

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
