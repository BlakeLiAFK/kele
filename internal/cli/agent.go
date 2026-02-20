package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	pb "github.com/BlakeLiAFK/kele/internal/proto"
)

var (
	agentSession string
	agentModel   string
	agentOneshot bool
	agentPrompt  string
)

func newAgentCmd() *cobra.Command {
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "无头 Agent 模式（管道/脚本）",
		Long:  "Agent 模式通过 gRPC 连接 daemon，从 stdin 读取输入，输出到 stdout。适合 CI/CD 和脚本嵌入。",
		RunE:  runAgent,
	}

	agentCmd.Flags().StringVar(&agentSession, "session", "", "指定会话 ID")
	agentCmd.Flags().StringVar(&agentModel, "model", "", "指定模型")
	agentCmd.Flags().BoolVar(&agentOneshot, "oneshot", false, "单次问答模式")
	agentCmd.Flags().StringVarP(&agentPrompt, "prompt", "p", "", "直接指定提示词")

	return agentCmd
}

func runAgent(cmd *cobra.Command, args []string) error {
	// Ensure daemon is running
	conn, err := ensureDaemon()
	if err != nil {
		return fmt.Errorf("daemon 连接失败: %w", err)
	}
	defer conn.Close()

	client := pb.NewKeleServiceClient(conn)
	ctx := context.Background()

	// Create or use existing session
	sessionID := agentSession
	if sessionID == "" {
		resp, err := client.CreateSession(ctx, &pb.CreateSessionRequest{Name: "agent"})
		if err != nil {
			return fmt.Errorf("创建会话失败: %w", err)
		}
		sessionID = resp.Id
		defer client.DeleteSession(ctx, &pb.DeleteSessionRequest{SessionId: sessionID})
	}

	// Switch model if specified
	if agentModel != "" {
		_, err := client.RunCommand(ctx, &pb.RunCommandRequest{
			SessionId: sessionID,
			Command:   "/model " + agentModel,
		})
		if err != nil {
			return fmt.Errorf("切换模型失败: %w", err)
		}
	}

	// If prompt is given directly, use it
	if agentPrompt != "" {
		// Read stdin as context if available
		input := agentPrompt
		if stat, _ := os.Stdin.Stat(); (stat.Mode() & os.ModeCharDevice) == 0 {
			stdinData, err := io.ReadAll(os.Stdin)
			if err == nil && len(stdinData) > 0 {
				input = string(stdinData) + "\n\n" + agentPrompt
			}
		}
		return agentChat(ctx, client, sessionID, input)
	}

	// Oneshot or pipe mode: read all stdin
	if agentOneshot || !isTerminal() {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("读取 stdin: %w", err)
		}
		text := strings.TrimSpace(string(input))
		if text == "" {
			return fmt.Errorf("输入为空")
		}
		return agentChat(ctx, client, sessionID, text)
	}

	// Interactive mode: read line by line
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Fprint(os.Stderr, "> ")
	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			fmt.Fprint(os.Stderr, "> ")
			continue
		}
		if input == "/exit" || input == "/quit" {
			break
		}
		if err := agentChat(ctx, client, sessionID, input); err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		}
		fmt.Println()
		fmt.Fprint(os.Stderr, "> ")
	}

	return nil
}

func agentChat(ctx context.Context, client pb.KeleServiceClient, sessionID, input string) error {
	stream, err := client.Chat(ctx, &pb.ChatRequest{
		SessionId: sessionID,
		Input:     input,
	})
	if err != nil {
		return fmt.Errorf("chat: %w", err)
	}

	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("recv: %w", err)
		}

		switch ev.Type {
		case "content":
			fmt.Print(ev.Content)
		case "tool_call":
			fmt.Fprintf(os.Stderr, "[tool: %s]\n", ev.ToolName)
		case "tool_result":
			fmt.Fprintf(os.Stderr, "[result: %s]\n", truncate(ev.ToolResult, 100))
		case "error":
			fmt.Fprintf(os.Stderr, "Error: %s\n", ev.Error)
		case "done":
			// done
		}
	}

	return nil
}

func isTerminal() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
