package tui

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// handleCommand 处理斜杠命令
func (a *App) handleCommand(cmd string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	sess := a.currentSession()
	command := parts[0]
	args := parts[1:]

	switch command {
	case "/help":
		sess.AddMessage("assistant", `Kele 命令帮助

快捷键
  Tab              智能补全（无建议时强制触发）
  Enter            发送消息
  Ctrl+J           换行（多行输入）
  Up/Down          浏览输入历史
  ESC x2           打断任务链（2秒内按两次）
  Ctrl+C x2        退出程序（2秒内按两次）
  Ctrl+E           展开/折叠 Thinking
  Ctrl+O           设置面板
  Ctrl+T           新建会话
  Ctrl+W           关闭当前会话
  Ctrl+Right       下一个会话
  Ctrl+Left        上一个会话
  Alt+1..9         切换会话（需终端支持）

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
  /model <name>     切换大模型
  /model-small <n>  切换小模型
  /models           列出常用模型
  /model-reset      重置为默认模型

记忆系统
  /remember <text>  添加到长期记忆
  /search <query>   搜索记忆
  /memory           查看记忆摘要

定时任务
  /cron             查看定时任务列表
  (创建/修改/删除请直接对话，AI 会使用工具)

信息查看
  /status           显示系统状态
  /config           显示当前配置
  /history          显示完整对话历史
  /tokens           显示 token 使用情况
  /debug            显示调试信息

会话导出
  /save             保存当前会话
  /export           导出对话为 Markdown`)

	case "/clear", "/reset":
		sess.messages = []Message{}
		sess.brain.ClearHistory()
		a.completion.ClearCache()
		a.viewport.SetContent("")
		a.updateStatus("对话已清空")

	case "/new":
		name := ""
		if len(args) > 0 {
			name = strings.Join(args, " ")
		}
		a.createSession(name)

	case "/sessions":
		var sb strings.Builder
		sb.WriteString("会话列表\n\n")
		for i, s := range a.sessions {
			marker := "  "
			if i == a.activeIdx {
				marker = "> "
			}
			sb.WriteString(fmt.Sprintf("%s%d: %s (%d 条消息)\n", marker, i+1, s.name, len(s.messages)))
		}
		sb.WriteString("\n使用 Ctrl+Right/Left / Alt+N / /switch N 切换")
		sess.AddMessage("assistant", sb.String())

	case "/switch":
		if len(args) == 0 {
			sess.AddMessage("assistant", "用法: /switch <会话编号>")
		} else {
			var idx int
			fmt.Sscanf(args[0], "%d", &idx)
			if idx >= 1 && idx <= len(a.sessions) {
				a.switchSession(idx - 1)
			} else {
				sess.AddMessage("assistant", fmt.Sprintf("无效会话编号，当前有 %d 个会话", len(a.sessions)))
			}
		}

	case "/rename":
		if len(args) == 0 {
			sess.AddMessage("assistant", "用法: /rename <新名称>")
		} else {
			sess.name = strings.Join(args, " ")
			sess.AddMessage("assistant", fmt.Sprintf("会话已重命名为: %s", sess.name))
		}

	case "/model":
		if len(args) == 0 {
			sess.AddMessage("assistant", fmt.Sprintf("当前大模型: %s\n默认模型: %s\n小模型: %s\n\n使用 /model <name> 切换",
				sess.brain.GetModel(), sess.brain.GetDefaultModel(), sess.brain.GetSmallModel()))
		} else {
			modelName := strings.Join(args, " ")
			sess.brain.SetModel(modelName)
			sess.AddMessage("assistant", fmt.Sprintf("已切换大模型: %s", modelName))
			a.updateStatus("Ready")
		}

	case "/model-small":
		if len(args) == 0 {
			sess.AddMessage("assistant", fmt.Sprintf("当前小模型: %s\n\n使用 /model-small <name> 切换", sess.brain.GetSmallModel()))
		} else {
			modelName := strings.Join(args, " ")
			sess.brain.SetSmallModel(modelName)
			sess.AddMessage("assistant", fmt.Sprintf("已切换小模型: %s", modelName))
		}

	case "/models":
		sess.AddMessage("assistant", `常用模型列表

OpenAI:
  gpt-4o            大模型推荐
  gpt-4o-mini       小模型推荐
  gpt-4-turbo       GPT-4 Turbo

Anthropic Claude:
  claude-3-5-sonnet-20241022   大模型推荐
  claude-3-haiku-20240307      小模型推荐

DeepSeek:
  deepseek-chat      大模型
  deepseek-reasoner  推理模型

用法:
  /model gpt-4o               设置大模型
  /model-small gpt-4o-mini    设置小模型`)

	case "/model-reset":
		sess.brain.ResetModel()
		sess.AddMessage("assistant", fmt.Sprintf("已重置为默认模型: %s", sess.brain.GetDefaultModel()))
		a.updateStatus("Ready")

	case "/status":
		sess.AddMessage("assistant", fmt.Sprintf(`系统状态

会话: %d/%d (当前: %s)
消息: %d 条, 历史: %d 条
大模型: %s
小模型: %s
窗口: %d x %d
时间: %s`,
			a.activeIdx+1, len(a.sessions), sess.name,
			len(sess.messages), len(sess.brain.GetHistory()),
			sess.brain.GetModel(), sess.brain.GetSmallModel(),
			a.width, a.height,
			time.Now().Format("2006-01-02 15:04:05")))

	case "/config":
		sess.AddMessage("assistant", fmt.Sprintf(`当前配置

环境变量:
  OPENAI_API_BASE:  %s
  OPENAI_MODEL:     %s
  KELE_SMALL_MODEL: %s

运行时:
  大模型: %s
  小模型: %s
  流式响应: 启用
  补全防抖: 500ms
  最大会话数: %d`,
			getEnvDefault("OPENAI_API_BASE", "(默认)"),
			getEnvDefault("OPENAI_MODEL", "gpt-4o"),
			getEnvDefault("KELE_SMALL_MODEL", "(回落到大模型)"),
			sess.brain.GetModel(), sess.brain.GetSmallModel(),
			maxSessions))

	case "/history":
		history := sess.brain.GetHistory()
		var sb strings.Builder
		sb.WriteString("对话历史\n\n")
		for i, msg := range history {
			sb.WriteString(fmt.Sprintf("%d. [%s] %s\n\n",
				i+1, msg.Role, truncateStr(msg.Content, 100)))
		}
		if len(history) == 0 {
			sb.WriteString("(暂无历史记录)")
		}
		sess.AddMessage("assistant", sb.String())

	case "/remember":
		if len(args) == 0 {
			sess.AddMessage("assistant", "用法: /remember <要记住的内容>")
		} else {
			text := strings.Join(args, " ")
			key := fmt.Sprintf("note_%d", time.Now().Unix())
			if err := sess.brain.SaveMemory(key, text); err != nil {
				sess.AddMessage("assistant", fmt.Sprintf("保存失败: %v", err))
			} else {
				sess.AddMessage("assistant", "已添加到长期记忆")
			}
		}

	case "/search":
		if len(args) == 0 {
			sess.AddMessage("assistant", "用法: /search <搜索关键词>")
		} else {
			query := strings.Join(args, " ")
			results, err := sess.brain.SearchMemory(query)
			if err != nil {
				sess.AddMessage("assistant", fmt.Sprintf("搜索失败: %v", err))
			} else if len(results) == 0 {
				sess.AddMessage("assistant", "未找到相关记忆")
			} else {
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("搜索结果 (%d 条):\n\n", len(results)))
				for i, r := range results {
					sb.WriteString(fmt.Sprintf("%d. %s\n\n", i+1, r))
				}
				sess.AddMessage("assistant", sb.String())
			}
		}

	case "/memory":
		sess.AddMessage("assistant", "记忆系统\n\n命令:\n  /remember <text>  添加到长期记忆\n  /search <query>   搜索记忆\n\n存储: .kele/memory.db")

	case "/tokens":
		sess.AddMessage("assistant", "Token 使用情况\n\n当前会话:\n  输入 tokens: 估算中\n  输出 tokens: 估算中\n\n(Token 计数功能开发中)")

	case "/save":
		sess.AddMessage("assistant", "会话已自动保存到 .kele/sessions/")

	case "/export":
		var export strings.Builder
		export.WriteString("# Kele 对话导出\n\n")
		export.WriteString(fmt.Sprintf("导出时间: %s\n\n---\n\n", time.Now().Format("2006-01-02 15:04:05")))
		for _, msg := range sess.messages {
			if msg.Role == "user" {
				export.WriteString(fmt.Sprintf("## You\n\n%s\n\n", msg.Content))
			} else {
				export.WriteString(fmt.Sprintf("## Kele\n\n%s\n\n", msg.Content))
			}
		}
		sess.AddMessage("assistant", fmt.Sprintf("对话已导出\n\n(功能开发中)"))

	case "/debug":
		sess.AddMessage("assistant", fmt.Sprintf(`调试信息

会话: %d/%d (%s)
消息数: %d
流式状态: %v
事件通道: %v
缓冲区: %d
AI补全中: %v
补全缓存: %d
当前suggestion: %q
大模型: %s
小模型: %s`,
			a.activeIdx+1, len(a.sessions), sess.name,
			len(sess.messages), sess.streaming,
			sess.eventChan != nil, len(sess.streamBuffer),
			a.aiPending, len(a.completion.cache),
			a.suggestion,
			sess.brain.GetModel(), sess.brain.GetSmallModel()))

	case "/cron":
		if a.scheduler == nil {
			sess.AddMessage("assistant", "定时任务调度器未初始化")
		} else {
			jobs, err := a.scheduler.ListJobs()
			if err != nil {
				sess.AddMessage("assistant", fmt.Sprintf("查询失败: %v", err))
			} else if len(jobs) == 0 {
				sess.AddMessage("assistant", "暂无定时任务\n\n通过对话让 AI 帮你创建，例如：\n  \"每5分钟检查一次磁盘空间\"\n  \"每天早上9点备份数据库\"")
			} else {
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("定时任务 (%d 个)\n\n", len(jobs)))
				for _, j := range jobs {
					status := "启用"
					if !j.Enabled {
						status = "暂停"
					}
					nextStr := "-"
					if j.NextRun != nil {
						nextStr = j.NextRun.Format("01-02 15:04")
					}
					sb.WriteString(fmt.Sprintf("  %s  %s  [%s]  %s  下次: %s\n",
						j.ID, j.Name, status, j.Schedule, nextStr))
				}
				sb.WriteString("\n通过对话管理：创建/修改/删除/暂停")
				sess.AddMessage("assistant", sb.String())
			}
		}

	case "/exit", "/quit":
		sess.AddMessage("assistant", "再见!")
		a.quitting = true
		return

	default:
		sess.AddMessage("assistant", fmt.Sprintf("未知命令: %s\n输入 /help 查看可用命令", cmd))
	}

	a.refreshViewport()
}

// getEnvDefault 获取环境变量
func getEnvDefault(key, defaultValue string) string {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	return v
}

// truncateStr 截断字符串
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
