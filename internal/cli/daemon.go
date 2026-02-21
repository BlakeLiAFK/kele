package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/BlakeLiAFK/kele/internal/config"
	"github.com/BlakeLiAFK/kele/internal/daemon"
)

var daemonForeground bool

func newDaemonCmd() *cobra.Command {
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "管理 Kele 守护进程",
		Long:  "Kele daemon 是后台常驻服务，管理 AI Brain、Memory、LLM Provider、Cron 和 Heartbeat。",
	}

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "启动 daemon",
		RunE:  runDaemonStart,
	}
	startCmd.Flags().BoolVar(&daemonForeground, "foreground", false, "前台运行（调试用）")

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "停止 daemon",
		RunE:  runDaemonStop,
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "查看 daemon 状态",
		RunE:  runDaemonStatus,
	}

	daemonCmd.AddCommand(startCmd, stopCmd, statusCmd)
	return daemonCmd
}

func runDaemonStart(cmd *cobra.Command, args []string) error {
	// Check if already running
	if pid, running := daemon.IsRunning(); running {
		return fmt.Errorf("daemon 已在运行 (PID: %d)", pid)
	}

	cfg := config.Load()
	cfg.ApplyFlags(cfgModel, cfgDebug, cfgPath)

	if daemonForeground {
		// Run in foreground (blocking)
		d := daemon.New(cfg)
		return d.Run()
	}

	// Background mode: the TUI's ensureDaemon() handles forking.
	// If called directly, we just run in foreground.
	d := daemon.New(cfg)
	return d.Run()
}

func runDaemonStop(cmd *cobra.Command, args []string) error {
	pid, running := daemon.IsRunning()
	if !running {
		fmt.Println("Daemon 未在运行")
		return nil
	}

	if err := daemon.Stop(); err != nil {
		return fmt.Errorf("停止失败: %w", err)
	}

	fmt.Printf("Daemon 已停止 (PID: %d)\n", pid)
	return nil
}

func runDaemonStatus(cmd *cobra.Command, args []string) error {
	pid, running := daemon.IsRunning()
	if !running {
		fmt.Println("Daemon 状态: 未运行")
		return nil
	}

	fmt.Printf("Daemon 状态: 运行中\n")
	fmt.Printf("PID: %d\n", pid)
	fmt.Printf("Socket: %s\n", daemon.SocketPath())
	return nil
}
