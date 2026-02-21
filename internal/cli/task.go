package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	pb "github.com/BlakeLiAFK/kele/internal/proto"
)

func newTaskCmd() *cobra.Command {
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "任务管理",
		Long:  "管理 TaskBoard 任务：创建、列出、查看、执行控制、日志查看。",
	}

	createCmd := &cobra.Command{
		Use:   "create <workspace-id>",
		Short: "创建任务",
		Args:  cobra.ExactArgs(1),
		RunE:  runTaskCreate,
	}
	createCmd.Flags().StringP("title", "t", "", "任务标题（必填）")
	createCmd.Flags().StringP("prompt", "p", "", "任务 prompt（必填）")
	createCmd.Flags().StringP("description", "d", "", "任务描述")
	createCmd.Flags().IntP("priority", "P", 2, "优先级 (0=critical, 1=high, 2=normal, 3=low)")
	createCmd.Flags().String("depends", "", "依赖任务 ID（逗号分隔）")
	createCmd.Flags().Int("max-retries", 1, "最大重试次数")
	createCmd.Flags().StringSlice("tags", nil, "标签")
	createCmd.Flags().Bool("auto-ready", false, "直接设为 ready 状态")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出任务",
		RunE:  runTaskList,
	}
	listCmd.Flags().StringP("workspace", "w", "", "按工作区过滤")
	listCmd.Flags().StringP("status", "s", "", "按状态过滤（逗号分隔）")

	showCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "查看任务详情",
		Args:  cobra.ExactArgs(1),
		RunE:  runTaskShow,
	}

	startCmd := &cobra.Command{
		Use:   "start <id>",
		Short: "手动启动任务",
		Args:  cobra.ExactArgs(1),
		RunE:  runTaskStart,
	}

	cancelCmd := &cobra.Command{
		Use:   "cancel <id>",
		Short: "取消任务",
		Args:  cobra.ExactArgs(1),
		RunE:  runTaskCancel,
	}

	retryCmd := &cobra.Command{
		Use:   "retry <id>",
		Short: "重试失败的任务",
		Args:  cobra.ExactArgs(1),
		RunE:  runTaskRetry,
	}

	logCmd := &cobra.Command{
		Use:   "log <id>",
		Short: "查看任务执行日志",
		Args:  cobra.ExactArgs(1),
		RunE:  runTaskLog,
	}
	logCmd.Flags().IntP("limit", "n", 100, "最多显示条数")

	taskCmd.AddCommand(createCmd, listCmd, showCmd, startCmd, cancelCmd, retryCmd, logCmd)
	return taskCmd
}

func runTaskCreate(cmd *cobra.Command, args []string) error {
	workspaceID := args[0]
	title, _ := cmd.Flags().GetString("title")
	prompt, _ := cmd.Flags().GetString("prompt")
	description, _ := cmd.Flags().GetString("description")
	priority, _ := cmd.Flags().GetInt("priority")
	dependsStr, _ := cmd.Flags().GetString("depends")
	maxRetries, _ := cmd.Flags().GetInt("max-retries")
	tags, _ := cmd.Flags().GetStringSlice("tags")
	autoReady, _ := cmd.Flags().GetBool("auto-ready")

	if title == "" {
		return fmt.Errorf("需要指定 --title")
	}
	if prompt == "" {
		return fmt.Errorf("需要指定 --prompt")
	}

	var depends []string
	if dependsStr != "" {
		for _, d := range strings.Split(dependsStr, ",") {
			depends = append(depends, strings.TrimSpace(d))
		}
	}

	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 连接失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)
	ctx := context.Background()

	t, err := client.CreateTask(ctx, &pb.CreateTaskRequest{
		WorkspaceId: workspaceID,
		Title:       title,
		Description: description,
		Prompt:      prompt,
		Priority:    int32(priority),
		DependsOn:   depends,
		MaxRetries:  int32(maxRetries),
		Tags:        tags,
		AutoReady:   autoReady,
	})
	if err != nil {
		return fmt.Errorf("创建任务失败: %w", err)
	}

	fmt.Printf("任务已创建: [%s] %s (状态: %s)\n", t.Id, t.Title, t.Status)
	return nil
}

func runTaskList(cmd *cobra.Command, args []string) error {
	workspaceID, _ := cmd.Flags().GetString("workspace")
	statusFilter, _ := cmd.Flags().GetString("status")

	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 连接失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)
	ctx := context.Background()

	resp, err := client.ListTasks(ctx, &pb.ListTasksRequest{
		WorkspaceId:  workspaceID,
		StatusFilter: statusFilter,
	})
	if err != nil {
		return fmt.Errorf("列出任务失败: %w", err)
	}

	if len(resp.Tasks) == 0 {
		fmt.Println("暂无任务。")
		return nil
	}

	fmt.Printf("%-16s %-10s %-5s %-30s %s\n", "ID", "状态", "优先级", "标题", "工作区")
	fmt.Println("────────────────────────────────────────────────────────────────────────────")
	for _, t := range resp.Tasks {
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
		title := t.Title
		if len(title) > 28 {
			title = title[:28] + ".."
		}
		fmt.Printf("%-16s %s %-8s %-5d %-30s %s\n",
			t.Id, icon, t.Status, t.Priority, title, t.WorkspaceId)
	}
	return nil
}

func runTaskShow(cmd *cobra.Command, args []string) error {
	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 连接失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)
	ctx := context.Background()

	t, err := client.GetTask(ctx, &pb.GetTaskRequest{Id: args[0]})
	if err != nil {
		return fmt.Errorf("获取任务失败: %w", err)
	}

	fmt.Printf("任务: %s\n", t.Title)
	fmt.Printf("  ID:          %s\n", t.Id)
	fmt.Printf("  工作区:      %s\n", t.WorkspaceId)
	fmt.Printf("  状态:        %s\n", t.Status)
	fmt.Printf("  优先级:      %d\n", t.Priority)
	fmt.Printf("  重试:        %d/%d\n", t.RetryCount, t.MaxRetries)
	fmt.Printf("  创建时间:    %s\n", t.CreatedAt)
	if t.StartedAt != "" {
		fmt.Printf("  开始时间:    %s\n", t.StartedAt)
	}
	if t.CompletedAt != "" {
		fmt.Printf("  完成时间:    %s\n", t.CompletedAt)
	}
	if len(t.DependsOn) > 0 {
		fmt.Printf("  依赖:        %s\n", strings.Join(t.DependsOn, ", "))
	}
	if len(t.Tags) > 0 {
		fmt.Printf("  标签:        %s\n", strings.Join(t.Tags, ", "))
	}
	if t.Description != "" {
		fmt.Printf("  描述:        %s\n", t.Description)
	}

	if t.Prompt != "" {
		fmt.Printf("\nPrompt:\n")
		prompt := t.Prompt
		if len(prompt) > 500 {
			prompt = prompt[:500] + "..."
		}
		fmt.Println(prompt)
	}

	if t.Result != "" {
		fmt.Printf("\n结果:\n")
		result := t.Result
		if len(result) > 1000 {
			result = result[:1000] + "..."
		}
		fmt.Println(result)
	}

	if t.Error != "" {
		fmt.Printf("\n错误: %s\n", t.Error)
	}

	return nil
}

func runTaskStart(cmd *cobra.Command, args []string) error {
	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 连接失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)
	ctx := context.Background()

	t, err := client.StartTask(ctx, &pb.StartTaskRequest{Id: args[0]})
	if err != nil {
		return fmt.Errorf("启动任务失败: %w", err)
	}

	fmt.Printf("任务 %s 已设为 ready，调度器将自动执行\n", t.Title)
	return nil
}

func runTaskCancel(cmd *cobra.Command, args []string) error {
	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 连接失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)
	ctx := context.Background()

	t, err := client.CancelTask(ctx, &pb.CancelTaskRequest{Id: args[0]})
	if err != nil {
		return fmt.Errorf("取消任务失败: %w", err)
	}

	fmt.Printf("任务 %s 已取消\n", t.Title)
	return nil
}

func runTaskRetry(cmd *cobra.Command, args []string) error {
	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 连接失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)
	ctx := context.Background()

	t, err := client.RetryTask(ctx, &pb.RetryTaskRequest{Id: args[0]})
	if err != nil {
		return fmt.Errorf("重试任务失败: %w", err)
	}

	fmt.Printf("任务 %s 已重置为 ready，将自动重新执行\n", t.Title)
	return nil
}

func runTaskLog(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")

	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 连接失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)
	ctx := context.Background()

	resp, err := client.GetTaskLog(ctx, &pb.GetTaskLogRequest{
		TaskId: args[0],
		Limit:  int32(limit),
	})
	if err != nil {
		return fmt.Errorf("获取任务日志失败: %w", err)
	}

	if len(resp.Entries) == 0 {
		fmt.Println("暂无日志记录。")
		return nil
	}

	for _, entry := range resp.Entries {
		prefix := ""
		switch entry.EventType {
		case "content":
			prefix = "[内容]"
		case "thinking":
			prefix = "[思考]"
		case "tool_call":
			prefix = fmt.Sprintf("[工具] %s", entry.ToolName)
		case "tool_result":
			prefix = fmt.Sprintf("[结果] %s", entry.ToolName)
		case "error":
			prefix = "[错误]"
		default:
			prefix = fmt.Sprintf("[%s]", entry.EventType)
		}

		content := entry.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		fmt.Printf("%s %s %s\n", entry.Timestamp, prefix, content)
	}
	return nil
}
