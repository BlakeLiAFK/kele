package tui

import (
	"fmt"
	"strings"

	"github.com/BlakeLiAFK/kele/internal/config"
)

// TUI-local commands (never forwarded to daemon)
var tuiLocalCommands = map[string]bool{
	"/new": true, "/sessions": true, "/switch": true, "/rename": true,
	"/exit": true, "/quit": true,
	"/export": true, "/debug": true,
}

// handleCommand 处理斜杠命令
func (a *App) handleCommand(cmd string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	sess := a.currentSession()
	command := parts[0]
	args := parts[1:]

	// Daemon 模式：非 TUI-local 命令转发到 daemon
	if a.isDaemonMode() && !tuiLocalCommands[command] {
		a.handleDaemonCommand(command, args, cmd)
		return
	}

	switch command {
	case "/help":
		sess.AddMessage("assistant", fmt.Sprintf(`Kele v%s 命令帮助

快捷键
  Tab              智能补全（无建议时强制触发）
  Enter            发送消息
  Ctrl+J           换行（多行输入）
  Up/Down          浏览输入历史
  ESC x2           打断任务链（2秒内按两次）
  Ctrl+C x2        退出程序（2秒内按两次）
  Ctrl+E           展开/折叠 Thinking
  Ctrl+G           切换鼠标模式（滚轮滚动/文本选中）
  Ctrl+O           设置面板
  Ctrl+T           新建会话
  Ctrl+W           关闭当前会话
  Ctrl+N           下一个会话
  Ctrl+P           上一个会话

@ 引用
  @file.go         引用单个文件
  @src/            引用目录结构
  @*.go            引用匹配的文件

对话控制
  /clear, /reset   清空对话历史
  /exit, /quit     退出程序

会话管理
  /new [name]      新建会话
  /sessions        列出所有会话
  /switch <n>      切换到第 n 个会话
  /rename <name>   重命名当前会话

模型管理
  /model <name>     切换大模型（自动匹配供应商）
  /model-small <n>  切换小模型
  /models           列出可用模型
  /model-reset      重置为默认模型

供应商管理
  /provider             列出所有供应商
  /provider add ...     添加自定义供应商
  /provider use <name>  切换活跃供应商
  /provider set ...     修改配置
  /provider remove <n>  删除
  /provider info [n]    查看详情

工具与记忆
  /tools            列出所有可用工具
  /remember <text>  添加到长期记忆
  /search <query>   搜索记忆
  /memory           查看记忆摘要

定时任务
  /cron             查看定时任务列表
  (创建/修改/删除请直接对话，AI 会使用工具)

配置管理
  /config           列出所有配置项
  /config set k v   设置配置项
  /config get k     获取配置项

信息查看
  /status           显示系统状态
  /history          显示完整对话历史
  /tokens           显示 token 估算
  /debug            显示调试信息

会话导出
  /save             保存当前会话
  /export           导出对话为 Markdown`, config.Version))

	case "/clear", "/reset":
		sess.messages = []Message{}
		if sess.brain != nil {
			sess.brain.ClearHistory()
		}
		a.completion.ClearCache()
		a.viewport.SetContent("")
		a.updateStatus("对话已清空")

	case "/new":
		a.handleNewCmd(args)
	case "/sessions":
		a.handleSessionsCmd(sess)
	case "/switch":
		a.handleSwitchCmd(sess, args)
	case "/rename":
		a.handleRenameCmd(sess, args)

	case "/model":
		a.handleModelCmd(sess, args)
	case "/model-small":
		a.handleModelSmallCmd(sess, args)
	case "/models":
		a.handleModelsCmd(sess)
	case "/model-reset":
		a.handleModelResetCmd(sess)
	case "/model-info":
		a.handleModelInfoCmd(sess)

	case "/tools":
		a.handleToolsCmd(sess)
	case "/remember":
		a.handleRememberCmd(sess, args)
	case "/search":
		a.handleSearchCmd(sess, args)
	case "/memory":
		a.handleMemoryCmd(sess)

	case "/status":
		a.handleStatusCmd(sess)
	case "/history":
		a.handleHistoryCmd(sess)
	case "/tokens":
		a.handleTokensCmd(sess)
	case "/debug":
		a.handleDebugCmd(sess)
	case "/cron":
		a.handleCronCmd(sess)

	case "/provider":
		a.handleProviderCmd(sess, args)

	case "/config":
		if a.isDaemonMode() {
			a.handleDaemonCommand("/config", args, "/config "+strings.Join(args, " "))
			return
		}
		a.handleConfigLocal(args)

	case "/save":
		a.handleSaveCmd(sess)
	case "/export":
		a.handleExportCmd(sess)

	case "/load":
		a.handleLoadCmd(sess, args)

	case "/exit", "/quit":
		sess.AddMessage("assistant", "再见!")
		a.Close()
		a.quitting = true
		return

	default:
		sess.AddMessage("assistant", fmt.Sprintf("未知命令: %s\n输入 /help 查看可用命令", cmd))
	}

	a.refreshViewport()
}

// handleDaemonCommand 转发命令到 daemon
func (a *App) handleDaemonCommand(command string, args []string, fullCmd string) {
	sess := a.currentSession()

	// /clear 和 /reset 需要同时清理本地 TUI 消息
	if command == "/clear" || command == "/reset" {
		sess.messages = []Message{}
		a.completion.ClearCache()
		a.viewport.SetContent("")
	}

	output, quit, err := a.client.RunCommand(sess.daemonSessID, fullCmd)
	if err != nil {
		sess.AddMessage("assistant", fmt.Sprintf("命令执行失败: %v", err))
		a.refreshViewport()
		return
	}

	if output != "" {
		sess.AddMessage("assistant", output)
	}

	// 如果 daemon 返回的 /model 命令成功，更新本地缓存
	if command == "/model" && len(args) > 0 {
		sess.model = strings.Join(args, " ")
		a.updateStatus("Ready")
	}

	if quit {
		a.Close()
		a.quitting = true
	}

	a.refreshViewport()
}

// maskKey 遮蔽 API Key（显示前4位和后4位）
func maskKey(key string) string {
	if key == "" {
		return "(未设置)"
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

// memoryMessage 内部消息结构（用于 /save）
type memoryMessage struct {
	Role    string
	Content string
}

// truncateStr 截断字符串
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
