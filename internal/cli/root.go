package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/BlakeLiAFK/kele/internal/config"
	"github.com/BlakeLiAFK/kele/internal/daemon"
	pb "github.com/BlakeLiAFK/kele/internal/proto"
	"github.com/BlakeLiAFK/kele/internal/tui"
)

var (
	cfgModel  string
	cfgDebug  bool
	cfgPath   string
)

// NewRootCmd creates the root cobra command.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kele",
		Short: "Kele - 智能终端 AI 助手",
		Long:  "Kele 是一个智能终端 AI 助手，支持多模型、工具调用、定时任务和自主心跳。",
		RunE:  runTUI,
		// Silence usage on errors to keep output clean
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgModel, "model", "", "指定初始模型")
	rootCmd.PersistentFlags().BoolVar(&cfgDebug, "debug", false, "启用调试模式")
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", "", "指定配置文件路径")

	// Add subcommands
	rootCmd.AddCommand(newDaemonCmd())
	rootCmd.AddCommand(newAgentCmd())
	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}

// Execute runs the root command.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

// runTUI is the default command - starts TUI mode connected to daemon.
func runTUI(cmd *cobra.Command, args []string) error {
	cfg := config.Load()
	cfg.ApplyFlags(cfgModel, cfgDebug, cfgPath)

	// Setup debug logging
	if cfg.Debug {
		os.MkdirAll(".kele", 0755)
		logFile, err := os.OpenFile(".kele/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			defer logFile.Close()
		}
	}

	// Ensure daemon is running
	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 启动失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)

	// Create TUI app with gRPC client
	app := tui.NewAppWithClient(cfg, client)

	p := tea.NewProgram(
		app,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI 错误: %w", err)
	}

	return nil
}

// ensureDaemon makes sure the daemon is running and returns a gRPC connection.
func ensureDaemon() (*grpc.ClientConn, error) {
	socketPath := daemon.SocketPath()

	// Try connecting first
	conn, err := tryConnect(socketPath)
	if err == nil {
		return conn, nil
	}

	// Daemon not running, start it
	if _, running := daemon.IsRunning(); !running {
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("get executable: %w", err)
		}

		daemonCmd := exec.Command(exe, "daemon", "start", "--foreground")
		daemonCmd.Stdout = nil
		daemonCmd.Stderr = nil
		// Detach from terminal
		daemonCmd.SysProcAttr = daemonSysProcAttr()
		if err := daemonCmd.Start(); err != nil {
			return nil, fmt.Errorf("start daemon: %w", err)
		}
		// Don't wait for it
		go daemonCmd.Wait()
	}

	// Wait for socket to be ready
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		conn, err = tryConnect(socketPath)
		if err == nil {
			return conn, nil
		}
	}

	return nil, fmt.Errorf("daemon 未就绪（等待超时）")
}

// tryConnect attempts a gRPC connection to the daemon.
func tryConnect(socketPath string) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, err
	}

	return conn, nil
}
