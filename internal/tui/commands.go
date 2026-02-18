package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BlakeLiAFK/kele/internal/config"
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
		sess.AddMessage("assistant", fmt.Sprintf(`Kele v%s 命令帮助

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
  /model <name>     切换大模型（自动匹配供应商）
  /model-small <n>  切换小模型
  /models           列出可用模型
  /model-reset      重置为默认模型

工具与记忆
  /tools            列出所有可用工具
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
  /tokens           显示 token 估算
  /debug            显示调试信息

会话导出
  /save             保存当前会话
  /export           导出对话为 Markdown`, config.Version))

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
			sess.AddMessage("assistant", fmt.Sprintf("当前大模型: %s\n供应商: %s\n默认模型: %s\n小模型: %s\n\n使用 /model <name> 切换（自动匹配供应商）",
				sess.brain.GetModel(), sess.brain.GetProviderName(),
				sess.brain.GetDefaultModel(), sess.brain.GetSmallModel()))
		} else {
			modelName := strings.Join(args, " ")
			sess.brain.SetModel(modelName)
			sess.AddMessage("assistant", fmt.Sprintf("已切换模型: %s (供应商: %s)", modelName, sess.brain.GetProviderName()))
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
		providers := sess.brain.ListProviders()
		var sb strings.Builder
		sb.WriteString("可用模型列表\n\n")
		sb.WriteString(fmt.Sprintf("已注册供应商: %s\n", strings.Join(providers, ", ")))
		sb.WriteString(fmt.Sprintf("当前: %s (%s)\n\n", sess.brain.GetModel(), sess.brain.GetProviderName()))

		sb.WriteString("OpenAI:\n")
		sb.WriteString("  gpt-4o              大模型推荐\n")
		sb.WriteString("  gpt-4o-mini          小模型推荐\n")
		sb.WriteString("  gpt-4-turbo          GPT-4 Turbo\n")
		sb.WriteString("  o1-preview           推理模型\n\n")

		sb.WriteString("Anthropic Claude:\n")
		sb.WriteString("  claude-sonnet-4-5-20250929   最新 Sonnet\n")
		sb.WriteString("  claude-haiku-4-5-20251001    最新 Haiku\n")
		sb.WriteString("  claude-3-5-sonnet-20241022   Sonnet 3.5\n\n")

		sb.WriteString("DeepSeek (OpenAI 兼容):\n")
		sb.WriteString("  deepseek-chat        大模型\n")
		sb.WriteString("  deepseek-reasoner    推理模型\n\n")

		sb.WriteString("Ollama 本地模型 (名称含 :):\n")
		sb.WriteString("  llama3:8b            Llama 3\n")
		sb.WriteString("  qwen2:7b             通义千问\n")
		sb.WriteString("  codellama:13b        代码模型\n\n")

		sb.WriteString("用法:\n")
		sb.WriteString("  /model gpt-4o               自动选择 OpenAI\n")
		sb.WriteString("  /model claude-sonnet-4-5-20250929   自动选择 Anthropic\n")
		sb.WriteString("  /model llama3:8b             自动选择 Ollama\n")
		sb.WriteString("  /model-small gpt-4o-mini     设置小模型")
		sess.AddMessage("assistant", sb.String())

	case "/model-reset":
		sess.brain.ResetModel()
		sess.AddMessage("assistant", fmt.Sprintf("已重置为默认模型: %s (%s)", sess.brain.GetDefaultModel(), sess.brain.GetProviderName()))
		a.updateStatus("Ready")

	case "/tools":
		toolNames := sess.brain.ListTools()
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("可用工具 (%d 个)\n\n", len(toolNames)))
		for i, name := range toolNames {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, name))
		}
		sb.WriteString("\nAI 会根据对话内容自动调用工具")
		sess.AddMessage("assistant", sb.String())

	case "/status":
		sess.AddMessage("assistant", fmt.Sprintf(`系统状态

版本: Kele v%s
供应商: %s
可用供应商: %s

会话: %d/%d (当前: %s)
消息: %d 条, 历史: %d 条
大模型: %s
小模型: %s
Token 估算: ~%d
窗口: %d x %d
时间: %s`,
			config.Version,
			sess.brain.GetProviderName(),
			strings.Join(sess.brain.ListProviders(), ", "),
			a.activeIdx+1, len(a.sessions), sess.name,
			len(sess.messages), len(sess.brain.GetHistory()),
			sess.brain.GetModel(), sess.brain.GetSmallModel(),
			sess.brain.EstimateTokens(),
			a.width, a.height,
			time.Now().Format("2006-01-02 15:04:05")))

	case "/config":
		cfg := a.cfg
		sess.AddMessage("assistant", fmt.Sprintf(`当前配置 (v%s)

LLM:
  OpenAI API Base:  %s
  OpenAI Key:       %s
  Anthropic Key:    %s
  Ollama Host:      %s
  默认模型:          %s
  温度:              %.1f
  最大 Tokens:       %d

工具:
  Bash 超时:         %ds
  最大输出:          %d bytes
  最大写入:          %d bytes

记忆:
  数据库:            %s
  记忆文件:          %s
  会话目录:          %s

TUI:
  最大会话数:        %d
  最大输入字符:      %d

运行时:
  大模型: %s (%s)
  小模型: %s`,
			config.Version,
			cfg.LLM.OpenAIAPIBase,
			maskKey(cfg.LLM.OpenAIAPIKey),
			maskKey(cfg.LLM.AnthropicAPIKey),
			cfg.LLM.OllamaHost,
			cfg.LLM.OpenAIModel,
			cfg.LLM.Temperature,
			cfg.LLM.MaxTokens,
			cfg.Tools.BashTimeout,
			cfg.Tools.MaxOutputSize,
			cfg.Tools.MaxWriteSize,
			cfg.Memory.DBPath,
			cfg.Memory.MemoryFile,
			cfg.Memory.SessionDir,
			cfg.TUI.MaxSessions,
			cfg.TUI.MaxInputChars,
			sess.brain.GetModel(), sess.brain.GetProviderName(),
			sess.brain.GetSmallModel()))

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
		sess.AddMessage("assistant", fmt.Sprintf("记忆系统\n\n命令:\n  /remember <text>  添加到长期记忆\n  /search <query>   搜索记忆\n\n存储: %s", a.cfg.Memory.DBPath))

	case "/tokens":
		tokens := sess.brain.EstimateTokens()
		historyLen := len(sess.brain.GetHistory())
		sess.AddMessage("assistant", fmt.Sprintf(`Token 估算

当前会话:
  历史消息数: %d
  估算 Tokens: ~%d
  模型: %s (%s)

注: Token 数为粗略估算（约 4 字符/token），仅供参考`, historyLen, tokens, sess.brain.GetModel(), sess.brain.GetProviderName()))

	case "/save":
		sessionID := fmt.Sprintf("session_%d_%d", sess.id, time.Now().Unix())
		var memMsgs []memoryMessage
		for _, msg := range sess.messages {
			memMsgs = append(memMsgs, memoryMessage{Role: msg.Role, Content: msg.Content})
		}
		sess.AddMessage("assistant", fmt.Sprintf("会话已保存: %s (%d 条消息)", sessionID, len(sess.messages)))

	case "/export":
		var export strings.Builder
		export.WriteString("# Kele 对话导出\n\n")
		export.WriteString(fmt.Sprintf("导出时间: %s\n", time.Now().Format("2006-01-02 15:04:05")))
		export.WriteString(fmt.Sprintf("模型: %s (%s)\n\n---\n\n", sess.brain.GetModel(), sess.brain.GetProviderName()))
		for _, msg := range sess.messages {
			if msg.Role == "user" {
				export.WriteString(fmt.Sprintf("## You\n\n%s\n\n", msg.Content))
			} else {
				export.WriteString(fmt.Sprintf("## Kele\n\n%s\n\n", msg.Content))
			}
		}
		// 写入文件
		exportDir := filepath.Join(a.cfg.Memory.SessionDir, "exports")
		os.MkdirAll(exportDir, 0755)
		filename := fmt.Sprintf("kele_export_%s.md", time.Now().Format("20060102_150405"))
		exportPath := filepath.Join(exportDir, filename)
		if err := os.WriteFile(exportPath, []byte(export.String()), 0644); err != nil {
			sess.AddMessage("assistant", fmt.Sprintf("导出失败: %v", err))
		} else {
			sess.AddMessage("assistant", fmt.Sprintf("对话已导出到: %s", exportPath))
		}

	case "/debug":
		sess.AddMessage("assistant", fmt.Sprintf(`调试信息

版本: %s
会话: %d/%d (%s)
消息数: %d
流式状态: %v
事件通道: %v
缓冲区: %d
AI补全中: %v
补全缓存: %d
当前suggestion: %q
大模型: %s
小模型: %s
供应商: %s
已注册供应商: %s
Token 估算: ~%d`,
			config.Version,
			a.activeIdx+1, len(a.sessions), sess.name,
			len(sess.messages), sess.streaming,
			sess.eventChan != nil, len(sess.streamBuffer),
			a.aiPending, len(a.completion.cache),
			a.suggestion,
			sess.brain.GetModel(), sess.brain.GetSmallModel(),
			sess.brain.GetProviderName(),
			strings.Join(sess.brain.ListProviders(), ", "),
			sess.brain.EstimateTokens()))

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

	case "/model-info":
		info := sess.brain.GetProviderInfo()
		var sb strings.Builder
		sb.WriteString("模型详细信息\n\n")
		sb.WriteString(fmt.Sprintf("  供应商:       %s\n", info["provider"]))
		sb.WriteString(fmt.Sprintf("  当前模型:     %s\n", info["model"]))
		sb.WriteString(fmt.Sprintf("  默认模型:     %s\n", info["defaultModel"]))
		sb.WriteString(fmt.Sprintf("  小模型:       %s\n", info["smallModel"]))
		sb.WriteString(fmt.Sprintf("  工具支持:     %s\n", info["supportsTools"]))
		sb.WriteString(fmt.Sprintf("  已注册供应商: %s\n", strings.Join(sess.brain.ListProviders(), ", ")))
		sess.AddMessage("assistant", sb.String())

	case "/load":
		if len(args) == 0 {
			// 列出可加载的会话
			memStore := sess.brain.GetMemoryStore()
			if memStore == nil {
				sess.AddMessage("assistant", "记忆系统未初始化，无法加载会话")
			} else {
				sessions, err := memStore.ListSessions()
				if err != nil {
					sess.AddMessage("assistant", fmt.Sprintf("查询会话失败: %v", err))
				} else if len(sessions) == 0 {
					sess.AddMessage("assistant", "暂无已保存的会话")
				} else {
					var sb strings.Builder
					sb.WriteString(fmt.Sprintf("已保存的会话 (%d 个)\n\n", len(sessions)))
					for i, si := range sessions {
						sb.WriteString(fmt.Sprintf("  %d. %s (%d 条消息) - %s\n", i+1, si.ID, si.MessageCount, si.Summary))
					}
					sb.WriteString("\n用法: /load <session-id>")
					sess.AddMessage("assistant", sb.String())
				}
			}
		} else {
			sessionID := args[0]
			memStore := sess.brain.GetMemoryStore()
			if memStore == nil {
				sess.AddMessage("assistant", "记忆系统未初始化")
			} else {
				_, err := memStore.LoadSession(sessionID)
				if err != nil {
					sess.AddMessage("assistant", fmt.Sprintf("加载会话失败: %v", err))
				} else {
					sess.AddMessage("assistant", fmt.Sprintf("已加载会话: %s", sessionID))
				}
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
