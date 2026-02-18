package main

import (
	"flag"
	"fmt"
	"log"
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
	configPath := flag.String("config", "", "指定配置文件路径")
	flag.Parse()

	// 版本号
	if *version {
		fmt.Printf("Kele v%s\n", config.Version)
		os.Exit(0)
	}

	// 加载配置
	cfg := config.Load()
	cfg.ApplyFlags(*model, *debug, *configPath)

	// 调试模式：输出日志到文件
	if cfg.Debug {
		os.MkdirAll(".kele", 0755)
		logFile, err := os.OpenFile(".kele/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			log.SetOutput(logFile)
			log.Println("=== Kele debug mode started ===")
			defer logFile.Close()
		}
	}

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
