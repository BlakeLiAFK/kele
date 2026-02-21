package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	pb "github.com/BlakeLiAFK/kele/internal/proto"
)

func newWorkspaceCmd() *cobra.Command {
	wsCmd := &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"ws"},
		Short:   "工作区管理",
		Long:    "管理 TaskBoard 工作区：创建、列出、查看、暂停、恢复、删除。",
	}

	createCmd := &cobra.Command{
		Use:   "create <name>",
		Short: "创建工作区",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorkspaceCreate,
	}
	createCmd.Flags().IntP("max-concurrent", "c", 3, "最大并发任务数")
	createCmd.Flags().StringP("context", "x", "", "工作区上下文（注入 system prompt）")
	createCmd.Flags().StringP("goal", "g", "", "工作区目标")
	createCmd.Flags().StringP("work-dir", "d", "", "工作目录")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出所有工作区",
		RunE:  runWorkspaceList,
	}

	showCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "查看工作区详情",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorkspaceShow,
	}

	pauseCmd := &cobra.Command{
		Use:   "pause <id>",
		Short: "暂停工作区",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorkspaceSetStatus("paused"),
	}

	resumeCmd := &cobra.Command{
		Use:   "resume <id>",
		Short: "恢复工作区",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorkspaceSetStatus("active"),
	}

	deleteCmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "删除工作区",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorkspaceDelete,
	}

	summaryCmd := &cobra.Command{
		Use:   "summary <id>",
		Short: "查看工作区完成报告",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorkspaceSummary,
	}

	wsCmd.AddCommand(createCmd, listCmd, showCmd, pauseCmd, resumeCmd, deleteCmd, summaryCmd)
	return wsCmd
}

func runWorkspaceCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	maxConcurrent, _ := cmd.Flags().GetInt("max-concurrent")
	ctx_, _ := cmd.Flags().GetString("context")
	goal, _ := cmd.Flags().GetString("goal")
	workDir, _ := cmd.Flags().GetString("work-dir")

	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 连接失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)
	ctx := context.Background()

	ws, err := client.CreateWorkspace(ctx, &pb.CreateWorkspaceRequest{
		Name:          name,
		Goal:          goal,
		MaxConcurrent: int32(maxConcurrent),
		Context:       ctx_,
		WorkDir:       workDir,
	})
	if err != nil {
		return fmt.Errorf("创建工作区失败: %w", err)
	}

	fmt.Printf("工作区已创建: %s (ID: %s, 并发: %d)\n", ws.Name, ws.Id, ws.MaxConcurrent)
	return nil
}

func runWorkspaceList(cmd *cobra.Command, args []string) error {
	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 连接失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)
	ctx := context.Background()

	resp, err := client.ListWorkspaces(ctx, &pb.Empty{})
	if err != nil {
		return fmt.Errorf("列出工作区失败: %w", err)
	}

	if len(resp.Workspaces) == 0 {
		fmt.Println("暂无工作区。")
		return nil
	}

	fmt.Printf("%-20s %-10s %-6s %-8s %s\n", "ID", "状态", "任务数", "运行中", "名称")
	fmt.Println(fmt.Sprintf("%s", "────────────────────────────────────────────────────────────────"))
	for _, ws := range resp.Workspaces {
		fmt.Printf("%-20s %-10s %-6d %-8d %s\n",
			ws.Id, ws.Status, ws.TaskCount, ws.RunningCount, ws.Name)
	}
	return nil
}

func runWorkspaceShow(cmd *cobra.Command, args []string) error {
	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 连接失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)
	ctx := context.Background()

	ws, err := client.GetWorkspace(ctx, &pb.GetWorkspaceRequest{Id: args[0]})
	if err != nil {
		return fmt.Errorf("获取工作区失败: %w", err)
	}

	fmt.Printf("工作区: %s\n", ws.Name)
	fmt.Printf("  ID:          %s\n", ws.Id)
	fmt.Printf("  状态:        %s\n", ws.Status)
	fmt.Printf("  目标:        %s\n", ws.Goal)
	fmt.Printf("  并发数:      %d\n", ws.MaxConcurrent)
	fmt.Printf("  任务数:      %d (运行中: %d)\n", ws.TaskCount, ws.RunningCount)
	fmt.Printf("  工作目录:    %s\n", ws.WorkDir)
	fmt.Printf("  创建时间:    %s\n", ws.CreatedAt)
	if ws.Description != "" {
		fmt.Printf("  描述:        %s\n", ws.Description)
	}
	if ws.Summary != "" {
		fmt.Printf("\n完成报告:\n%s\n", ws.Summary)
	}

	// List tasks
	tasks, err := client.ListTasks(ctx, &pb.ListTasksRequest{WorkspaceId: ws.Id})
	if err == nil && len(tasks.Tasks) > 0 {
		fmt.Printf("\n任务列表:\n")
		for _, t := range tasks.Tasks {
			icon := " "
			switch t.Status {
			case "done":
				icon = "✓"
			case "running":
				icon = "●"
			case "failed":
				icon = "✗"
			case "ready":
				icon = "◌"
			case "cancelled":
				icon = "⊘"
			}
			fmt.Printf("  %s [%s] %s  (优先级:%d, 状态:%s)\n",
				icon, t.Id, t.Title, t.Priority, t.Status)
		}
	}

	return nil
}

func runWorkspaceSetStatus(status string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		conn, err := ensureDaemon()
		if err != nil {
			return fmt.Errorf("daemon 连接失败: %w", err)
		}
		defer conn.Close()

		client := pb.NewKeleServiceClient(conn)
		ctx := context.Background()

		ws, err := client.UpdateWorkspace(ctx, &pb.UpdateWorkspaceRequest{
			Id:     args[0],
			Status: status,
		})
		if err != nil {
			return fmt.Errorf("更新工作区失败: %w", err)
		}

		fmt.Printf("工作区 %s 状态已更新为: %s\n", ws.Name, ws.Status)
		return nil
	}
}

func runWorkspaceDelete(cmd *cobra.Command, args []string) error {
	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 连接失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)
	ctx := context.Background()

	_, err = client.DeleteWorkspace(ctx, &pb.DeleteWorkspaceRequest{Id: args[0]})
	if err != nil {
		return fmt.Errorf("删除工作区失败: %w", err)
	}

	fmt.Printf("工作区 %s 已删除\n", args[0])
	return nil
}

func runWorkspaceSummary(cmd *cobra.Command, args []string) error {
	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 连接失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)
	ctx := context.Background()

	ws, err := client.GetWorkspace(ctx, &pb.GetWorkspaceRequest{Id: args[0]})
	if err != nil {
		return fmt.Errorf("获取工作区失败: %w", err)
	}

	if ws.Summary == "" {
		fmt.Println("该工作区尚无完成报告。")
		return nil
	}

	fmt.Printf("工作区 %s 完成报告\n", ws.Name)
	fmt.Println("────────────────────────────────────────")
	fmt.Println(ws.Summary)
	return nil
}
