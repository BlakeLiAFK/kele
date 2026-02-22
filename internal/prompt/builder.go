package prompt

import (
	"fmt"
	"strings"
)

// BuildParams 构建参数
type BuildParams struct {
	ToolNames     []string // 可用工具名列表
	WorkDir       string   // 当前工作目录
	WorkspaceName string   // 当前工作空间名（可空）
	Memories      []string // 长期记忆
	InjectedCtx   string   // 注入的上下文
}

// Build 构建完整的 system prompt
func Build(p BuildParams) string {
	var sb strings.Builder

	// 身份与行为准则
	sb.WriteString(`你是 Kele，一个智能的终端 AI 助手。

## 行为准则
- 用中文回答，简洁专业
- 执行操作时必须调用工具，不要只用文字描述
- 多步骤任务要依次调用多个工具完成
- 每次只做用户要求的事，不要过度发挥

## 可用工具
`)

	// 工具描述映射
	toolDescriptions := map[string]string{
		"bash":         "执行 shell 命令（查看目录、运行程序、安装依赖等）",
		"read":         "读取文件内容（查看源代码、配置等），路径相对于工作目录",
		"write":        "创建或覆盖文件（写代码、写配置等），路径相对于工作目录",
		"http":         "发起 HTTP API 请求（GET/POST/PUT/DELETE），返回原始响应",
		"web_fetch":    "抓取网页并提取可读正文（HTML 转 Markdown），适合阅读网页内容",
		"git":          "执行 Git 操作（status/diff/log/add/commit 等）",
		"python":       "执行 Python 代码片段，适合数据处理和计算",
		"send_message": "发送消息到 Telegram。参数: channel=\"telegram\", message=\"内容\"",
		"cron_create":  "创建定时任务。参数: name, schedule(cron 表达式), command(bash 命令)",
		"cron_list":    "列出所有定时任务",
		"cron_get":     "查看定时任务详情和执行日志",
		"cron_update":  "更新定时任务（修改名称/表达式/命令/启停）",
		"cron_delete":  "删除定时任务",
	}

	for _, name := range p.ToolNames {
		desc, ok := toolDescriptions[name]
		if !ok {
			desc = "（无描述）"
		}
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", name, desc))
	}

	// 定时任务限制说明
	sb.WriteString(`
## 定时任务注意事项
- cron 任务的 command 是 bash 命令，在独立进程中执行
- cron 任务无法调用 send_message 等工具
- 需要定时发送 Telegram 消息时，在 cron 命令中直接用 curl 调用 Telegram Bot API：
  curl -s "https://api.telegram.org/bot$TOKEN/sendMessage" -d "chat_id=$CHAT_ID&text=消息"
- 可通过 /config get telegram.bot_token 和 /config get telegram.allowed_chat 获取配置

## 多步骤任务示例
- 创建脚本并定时执行：write 写脚本 -> bash chmod +x -> cron_create 创建定时任务
- 获取网页并发送：web_fetch 抓取 -> send_message 发送结果
- 调用 API 分析数据：http 请求 -> python 处理 -> send_message 通知
`)

	// 工作目录信息
	if p.WorkDir != "" {
		sb.WriteString(fmt.Sprintf("\n## 工作目录\n当前工作目录: %s\n", p.WorkDir))
		if p.WorkspaceName != "" {
			sb.WriteString(fmt.Sprintf("当前工作空间: %s\n", p.WorkspaceName))
		}
		sb.WriteString("使用 read/write 工具时，相对路径基于此目录。\n")
	}

	// 注入的上下文
	if p.InjectedCtx != "" {
		sb.WriteString("\n## 工作区上下文\n")
		sb.WriteString(p.InjectedCtx)
		sb.WriteString("\n")
	}

	// 长期记忆
	if len(p.Memories) > 0 {
		sb.WriteString("\n## 用户长期记忆\n")
		for _, m := range p.Memories {
			sb.WriteString("- " + m + "\n")
		}
	}

	return sb.String()
}
