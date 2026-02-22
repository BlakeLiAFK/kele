package cli

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/BlakeLiAFK/kele/internal/config"
	"github.com/BlakeLiAFK/kele/internal/daemon"
)

var daemonForeground bool

func newDaemonCmd() *cobra.Command {
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "管理 Kele 守护进程",
		Long:  "Kele daemon 是后台常驻服务，管理会话、LLM、工具、定时任务和看板。",
	}

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "启动 daemon（默认后台运行）",
		RunE:  runDaemonStart,
	}
	startCmd.Flags().BoolVar(&daemonForeground, "foreground", false, "前台运行（供 launchd/systemd 使用）")

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "停止 daemon",
		RunE:  runDaemonStop,
	}

	restartCmd := &cobra.Command{
		Use:   "restart",
		Short: "重启 daemon",
		RunE:  runDaemonRestart,
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "查看 daemon 状态",
		RunE:  runDaemonStatus,
	}

	daemonCmd.AddCommand(startCmd, stopCmd, restartCmd, statusCmd)
	return daemonCmd
}

func runDaemonStart(cmd *cobra.Command, args []string) error {
	if pid, running := daemon.IsRunning(); running {
		return fmt.Errorf("daemon 已在运行 (PID: %d)", pid)
	}

	cfg := config.Load()
	cfg.ApplyFlags(cfgModel, cfgDebug, cfgPath)

	// 前台模式：直接阻塞运行（供 launchd/systemd/调试使用）
	if daemonForeground {
		d := daemon.New(cfg)
		return d.Run()
	}

	// 后台模式：fork 子进程
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行路径失败: %w", err)
	}

	child := exec.Command(exe, "daemon", "start", "--foreground")
	child.Stdout = nil
	child.Stderr = nil
	child.SysProcAttr = daemonSysProcAttr()

	if err := child.Start(); err != nil {
		return fmt.Errorf("启动后台进程失败: %w", err)
	}

	// 回收子进程资源
	go func() {
		if err := child.Wait(); err != nil {
			log.Printf("daemon 进程退出: %v", err)
		}
	}()

	// 等待 daemon 就绪
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if pid, running := daemon.IsRunning(); running {
			fmt.Printf("Daemon 已启动 (PID: %d)\n", pid)
			return nil
		}
	}

	return fmt.Errorf("daemon 启动超时")
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

	// 等待进程退出
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if _, running := daemon.IsRunning(); !running {
			fmt.Printf("Daemon 已停止 (PID: %d)\n", pid)
			return nil
		}
	}

	fmt.Printf("Daemon 已发送停止信号 (PID: %d)\n", pid)
	return nil
}

func runDaemonRestart(cmd *cobra.Command, args []string) error {
	if _, running := daemon.IsRunning(); running {
		if err := runDaemonStop(cmd, args); err != nil {
			return err
		}
		// 等待完全停止
		time.Sleep(300 * time.Millisecond)
	}
	return runDaemonStart(cmd, args)
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
	fmt.Printf("配置存储: %s\n", config.ConfigStorePath())
	return nil
}
