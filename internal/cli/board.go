package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	pb "github.com/BlakeLiAFK/kele/internal/proto"
)

func newBoardCmd() *cobra.Command {
	boardCmd := &cobra.Command{
		Use:   "board",
		Short: "看板总览与任务规划",
		Long:  "显示看板总览，包括所有工作区和任务状态。支持 AI 目标分解和实时监控。",
		RunE:  runBoardOverview,
	}

	planCmd := &cobra.Command{
		Use:   "plan [goal]",
		Short: "AI 自动分解目标为任务计划",
		Long:  "使用 AI Agent 分析代码库，将模糊目标分解为具体的结构化任务。",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runBoardPlan,
	}

	approveCmd := &cobra.Command{
		Use:   "approve",
		Short: "审批并执行任务计划",
		RunE:  runBoardApprove,
	}
	approveCmd.Flags().StringP("plan", "p", "", "计划 JSON 文件路径或内联 JSON")
	approveCmd.Flags().StringP("goal", "g", "", "原始目标")
	approveCmd.Flags().StringP("work-dir", "d", "", "工作目录")
	approveCmd.Flags().Bool("auto-start", true, "批准后自动开始执行")

	watchCmd := &cobra.Command{
		Use:   "watch",
		Short: "实时监听看板事件流",
		RunE:  runBoardWatch,
	}
	watchCmd.Flags().StringP("workspace", "w", "", "仅监听指定工作区")

	boardCmd.AddCommand(planCmd, approveCmd, watchCmd)
	return boardCmd
}

func runBoardOverview(cmd *cobra.Command, args []string) error {
	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 连接失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)
	ctx := context.Background()

	overview, err := client.GetBoardOverview(ctx, &pb.Empty{})
	if err != nil {
		return fmt.Errorf("获取看板总览失败: %w", err)
	}

	if len(overview.Workspaces) == 0 {
		fmt.Println("看板为空。使用 `kele board plan \"目标\"` 创建任务计划。")
		return nil
	}

	fmt.Printf("看板总览  总任务: %d  运行中: %d  待处理: %d  已完成: %d\n\n",
		overview.TotalTasks, overview.RunningTasks, overview.PendingTasks, overview.CompletedTasks)

	for _, ws := range overview.Workspaces {
		statusIcon := "●"
		switch ws.Status {
		case "paused":
			statusIcon = "⏸"
		case "archived":
			statusIcon = "◆"
		}
		fmt.Printf("%s %s [%s]  %d/%d slots\n",
			statusIcon, ws.Name, ws.Status, ws.Running, ws.MaxConcurrent)
		fmt.Printf("  backlog:%d  ready:%d  running:%d  done:%d  failed:%d\n\n",
			ws.Backlog, ws.Ready, ws.Running, ws.Done, ws.Failed)
	}

	return nil
}

func runBoardPlan(cmd *cobra.Command, args []string) error {
	goal := strings.Join(args, " ")

	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 连接失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)
	ctx := context.Background()

	stream, err := client.PlanWorkspace(ctx, &pb.PlanWorkspaceRequest{
		Goal: goal,
	})
	if err != nil {
		return fmt.Errorf("规划失败: %w", err)
	}

	var planJSON string
	for {
		ev, err := stream.Recv()
		if err != nil {
			break
		}
		switch ev.Type {
		case "thinking":
			fmt.Printf("[Planner] %s\n", ev.Content)
		case "reading":
			fmt.Printf("[Planner] %s\n", ev.Content)
		case "plan_ready":
			planJSON = ev.PlanJson
			fmt.Println("\n[Planner] 计划生成完成!")
		case "error":
			return fmt.Errorf("规划错误: %s", ev.Content)
		}
	}

	if planJSON == "" {
		return fmt.Errorf("未生成有效计划")
	}

	// Pretty print the plan
	var plan map[string]interface{}
	json.Unmarshal([]byte(planJSON), &plan)

	fmt.Printf("\n工作区: %s\n", plan["workspace_name"])
	if mc, ok := plan["max_concurrent"]; ok {
		fmt.Printf("并发数: %v\n", mc)
	}

	if tasks, ok := plan["tasks"].([]interface{}); ok {
		fmt.Printf("任务数: %d\n\n", len(tasks))
		for i, t := range tasks {
			task := t.(map[string]interface{})
			deps := ""
			if d, ok := task["depends_on"].([]interface{}); ok && len(d) > 0 {
				depStrs := make([]string, len(d))
				for j, dep := range d {
					depStrs[j] = fmt.Sprintf("#%v", dep)
				}
				deps = "  依赖: " + strings.Join(depStrs, ", ")
			}
			priority := "normal"
			if p, ok := task["priority"].(float64); ok {
				switch int(p) {
				case 0:
					priority = "critical"
				case 1:
					priority = "high"
				case 3:
					priority = "low"
				}
			}
			fmt.Printf("  #%d  %s  优先级: %s%s\n", i+1, task["title"], priority, deps)
		}
	}

	// Save plan for approve command
	fmt.Printf("\n计划已保存。使用以下命令批准执行:\n")
	fmt.Printf("  kele board approve -p '%s' -g '%s' --auto-start\n", planJSON, goal)

	return nil
}

func runBoardApprove(cmd *cobra.Command, args []string) error {
	planStr, _ := cmd.Flags().GetString("plan")
	goal, _ := cmd.Flags().GetString("goal")
	workDir, _ := cmd.Flags().GetString("work-dir")
	autoStart, _ := cmd.Flags().GetBool("auto-start")

	if planStr == "" {
		return fmt.Errorf("需要指定计划 JSON（-p 参数）")
	}

	// Check if it's a file path
	if _, err := os.Stat(planStr); err == nil {
		data, err := os.ReadFile(planStr)
		if err != nil {
			return fmt.Errorf("读取计划文件: %w", err)
		}
		planStr = string(data)
	}

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

	resp, err := client.ApprovePlan(ctx, &pb.ApprovePlanRequest{
		PlanJson:  planStr,
		Goal:      goal,
		WorkDir:   workDir,
		AutoStart: autoStart,
	})
	if err != nil {
		return fmt.Errorf("批准计划失败: %w", err)
	}

	fmt.Printf("工作区已创建: %s (%s)\n", resp.Workspace.Name, resp.Workspace.Id)
	fmt.Printf("任务数: %d\n\n", len(resp.Tasks))

	for _, t := range resp.Tasks {
		fmt.Printf("  [%s] %s  状态: %s  优先级: %d\n", t.Id, t.Title, t.Status, t.Priority)
	}

	if autoStart {
		fmt.Printf("\n调度器已启动。使用 `kele board watch -w %s` 监听进度。\n", resp.Workspace.Id)
	}

	return nil
}

func runBoardWatch(cmd *cobra.Command, args []string) error {
	workspaceID, _ := cmd.Flags().GetString("workspace")

	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 连接失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)
	ctx := context.Background()

	stream, err := client.WatchBoard(ctx, &pb.WatchBoardRequest{
		WorkspaceId: workspaceID,
	})
	if err != nil {
		return fmt.Errorf("监听失败: %w", err)
	}

	fmt.Println("监听看板事件... (Ctrl+C 退出)")
	for {
		ev, err := stream.Recv()
		if err != nil {
			return nil
		}

		icon := " "
		switch ev.Type {
		case "task_started":
			icon = "●"
		case "task_completed":
			icon = "✓"
		case "task_failed":
			icon = "✗"
		case "task_cancelled":
			icon = "⊘"
		case "task_ready":
			icon = "◌"
		case "workspace_completed":
			icon = "★"
		}

		taskInfo := ""
		if ev.TaskId != "" {
			taskInfo = fmt.Sprintf(" %s", ev.TaskId)
		}

		fmt.Printf("[%s] %s %-20s%s  %s\n",
			ev.Timestamp, icon, ev.Type, taskInfo, ev.Detail)
	}
}
