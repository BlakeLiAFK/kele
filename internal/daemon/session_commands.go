package daemon

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/BlakeLiAFK/kele/internal/config"
	"github.com/BlakeLiAFK/kele/internal/llm"
)

// RunCommand executes a slash command and returns formatted output.
func (sb *SessionBrain) RunCommand(command string) (string, bool) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", false
	}

	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "/help":
		return fmt.Sprintf(`Kele v%s 命令帮助

对话控制
  /clear, /reset   清空对话历史

模型管理
  /model <name>     切换大模型（自动匹配供应商）
  /model-small <n>  切换小模型
  /models           列出可用模型
  /model-reset      重置为默认模型
  /model-info       显示模型详细信息

工具与记忆
  /tools            列出所有可用工具
  /remember <text>  添加到长期记忆
  /search <query>   搜索记忆
  /memory           查看记忆摘要

供应商管理
  /provider             列出所有供应商
  /provider add ...     添加自定义供应商
  /provider use <name>  切换活跃供应商
  /provider set ...     修改配置
  /provider remove <n>  删除
  /provider info [n]    查看详情

工作空间
  /works            列出所有工作空间
  /works create <n> 创建并切换工作空间
  /works use <n>    切换工作空间
  /works delete <n> 删除工作空间
  /works clear      清空所有工作空间

定时任务
  /cron             查看定时任务列表

配置管理
  /config           列出所有配置项
  /config set k v   设置配置项
  /config get k     获取配置项

信息查看
  /status           显示系统状态
  /history          显示完整对话历史
  /tokens           显示 token 估算

会话导出
  /save             保存当前会话
  /export           导出对话为 Markdown`, config.Version), false

	case "/clear", "/reset":
		sb.mu.Lock()
		sb.history = []llm.Message{}
		sb.mu.Unlock()
		return "对话已清空", false

	case "/model":
		if len(args) == 0 {
			return fmt.Sprintf("当前大模型: %s\n供应商: %s\n默认模型: %s\n小模型: %s\n\n使用 /model <name> 切换",
				sb.provider.GetModel(), sb.provider.GetActiveProviderName(),
				sb.provider.GetDefaultModel(), sb.provider.GetSmallModel()), false
		}
		modelName := strings.Join(args, " ")
		sb.provider.SetModel(modelName)
		config.SetValue("llm.openai_model", modelName)
		return fmt.Sprintf("已切换模型: %s (供应商: %s)", modelName, sb.provider.GetActiveProviderName()), false

	case "/model-small":
		if len(args) == 0 {
			return fmt.Sprintf("当前小模型: %s\n\n使用 /model-small <name> 切换", sb.provider.GetSmallModel()), false
		}
		modelName := strings.Join(args, " ")
		sb.provider.SetSmallModel(modelName)
		config.SetValue("llm.small_model", modelName)
		return fmt.Sprintf("已切换小模型: %s", modelName), false

	case "/models":
		providers := sb.provider.ListProviders()
		var s strings.Builder
		s.WriteString("可用模型列表\n\n")
		s.WriteString(fmt.Sprintf("已注册供应商: %s\n", strings.Join(providers, ", ")))
		s.WriteString(fmt.Sprintf("当前: %s (%s)\n\n", sb.provider.GetModel(), sb.provider.GetActiveProviderName()))
		s.WriteString("OpenAI:\n  gpt-4o, gpt-4o-mini, gpt-4-turbo, o1-preview\n\n")
		s.WriteString("Anthropic Claude:\n  claude-sonnet-4-5-20250929, claude-haiku-4-5-20251001\n\n")
		s.WriteString("DeepSeek (OpenAI 兼容):\n  deepseek-chat, deepseek-reasoner\n\n")
		s.WriteString("Ollama 本地模型 (名称含 :):\n  llama3:8b, qwen2:7b, codellama:13b")
		return s.String(), false

	case "/model-reset":
		sb.provider.ResetModel()
		config.DeleteValue("llm.openai_model")
		return fmt.Sprintf("已重置为默认模型: %s (%s)", sb.provider.GetDefaultModel(), sb.provider.GetActiveProviderName()), false

	case "/model-info":
		var s strings.Builder
		s.WriteString("模型详细信息\n\n")
		s.WriteString(fmt.Sprintf("  供应商:       %s\n", sb.provider.GetActiveProviderName()))
		s.WriteString(fmt.Sprintf("  当前模型:     %s\n", sb.provider.GetModel()))
		s.WriteString(fmt.Sprintf("  默认模型:     %s\n", sb.provider.GetDefaultModel()))
		s.WriteString(fmt.Sprintf("  小模型:       %s\n", sb.provider.GetSmallModel()))
		s.WriteString(fmt.Sprintf("  工具支持:     %v\n", sb.provider.ActiveSupportsTools()))
		s.WriteString(fmt.Sprintf("  已注册供应商: %s\n", strings.Join(sb.provider.ListProviders(), ", ")))
		return s.String(), false

	case "/tools":
		toolNames := sb.executor.ListTools()
		var s strings.Builder
		s.WriteString(fmt.Sprintf("可用工具 (%d 个)\n\n", len(toolNames)))
		for i, name := range toolNames {
			s.WriteString(fmt.Sprintf("  %d. %s\n", i+1, name))
		}
		s.WriteString("\nAI 会根据对话内容自动调用工具")
		return s.String(), false

	case "/status":
		return fmt.Sprintf(`系统状态

版本: Kele v%s
供应商: %s
可用供应商: %s
大模型: %s
小模型: %s
Token 估算: ~%d
时间: %s`,
			config.Version,
			sb.provider.GetActiveProviderName(),
			strings.Join(sb.provider.ListProviders(), ", "),
			sb.provider.GetModel(), sb.provider.GetSmallModel(),
			sb.estimateTokens(),
			time.Now().Format("2006-01-02 15:04:05")), false

	case "/config":
		if len(args) == 0 {
			all := config.AllSettings(sb.cfg)
			dbValues, _ := config.ListValues()

			keys := make([]string, 0, len(all))
			for k := range all {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			var s strings.Builder
			s.WriteString(fmt.Sprintf("Kele v%s 配置\n\n", config.Version))
			for _, k := range keys {
				v := all[k]
				if v == "" {
					v = "(未设置)"
				}
				tag := ""
				if _, ok := dbValues[k]; ok {
					tag = " [db]"
				}
				s.WriteString(fmt.Sprintf("  %-28s %s%s\n", k, v, tag))
			}
			s.WriteString("\n/config set <key> <value>  设置配置")
			s.WriteString("\n/config get <key>          获取配置")
			return s.String(), false
		}
		subCmd := args[0]
		subArgs := args[1:]
		switch subCmd {
		case "set":
			if len(subArgs) < 2 {
				return "用法: /config set <key> <value>", false
			}
			key := subArgs[0]
			val := strings.Join(subArgs[1:], " ")
			if err := config.SetValue(key, val); err != nil {
				return fmt.Sprintf("设置失败: %v", err), false
			}
			return fmt.Sprintf("%s = %s", key, val), false
		case "get":
			if len(subArgs) < 1 {
				return "用法: /config get <key>", false
			}
			val, err := config.GetValue(subArgs[0])
			if err != nil {
				return fmt.Sprintf("获取失败: %v", err), false
			}
			return fmt.Sprintf("%s = %s", subArgs[0], val), false
		default:
			return fmt.Sprintf("未知子命令: /config %s\n用法: /config [set|get]", subCmd), false
		}

	case "/history":
		sb.mu.RLock()
		historyCopy := make([]llm.Message, len(sb.history))
		copy(historyCopy, sb.history)
		sb.mu.RUnlock()

		var s strings.Builder
		s.WriteString("对话历史\n\n")
		for i, msg := range historyCopy {
			content := msg.Content
			if len([]rune(content)) > 100 {
				content = string([]rune(content)[:100]) + "..."
			}
			s.WriteString(fmt.Sprintf("%d. [%s] %s\n\n", i+1, msg.Role, content))
		}
		if len(historyCopy) == 0 {
			s.WriteString("(暂无历史记录)")
		}
		return s.String(), false

	case "/remember":
		if len(args) == 0 {
			return "用法: /remember <要记住的内容>", false
		}
		text := strings.Join(args, " ")
		key := fmt.Sprintf("note_%d", time.Now().Unix())
		if sb.memory == nil {
			return "记忆系统未初始化", false
		}
		if err := sb.memory.UpdateMemory(key, text); err != nil {
			return fmt.Sprintf("保存失败: %v", err), false
		}
		return "已添加到长期记忆", false

	case "/search":
		if len(args) == 0 {
			return "用法: /search <搜索关键词>", false
		}
		query := strings.Join(args, " ")
		if sb.memory == nil {
			return "记忆系统未初始化", false
		}
		results, err := sb.memory.Search(query, 5)
		if err != nil {
			return fmt.Sprintf("搜索失败: %v", err), false
		}
		if len(results) == 0 {
			return "未找到相关记忆", false
		}
		var s strings.Builder
		s.WriteString(fmt.Sprintf("搜索结果 (%d 条):\n\n", len(results)))
		for i, r := range results {
			s.WriteString(fmt.Sprintf("%d. %s\n\n", i+1, r))
		}
		return s.String(), false

	case "/memory":
		return fmt.Sprintf("记忆系统\n\n命令:\n  /remember <text>  添加到长期记忆\n  /search <query>   搜索记忆\n\n存储: %s", sb.cfg.Memory.DBPath), false

	case "/tokens":
		tokens := sb.estimateTokens()
		return fmt.Sprintf("Token 估算\n\n  历史消息数: %d\n  估算 Tokens: ~%d\n  模型: %s (%s)",
			len(sb.history), tokens, sb.provider.GetModel(), sb.provider.GetActiveProviderName()), false

	case "/cron":
		jobs, err := sb.executor.ListCronJobs()
		if err != nil {
			return fmt.Sprintf("查询失败: %v", err), false
		}
		if len(jobs) == 0 {
			return "暂无定时任务\n\n通过对话让 AI 帮你创建，例如：\n  \"每5分钟检查一次磁盘空间\"", false
		}
		var cs strings.Builder
		cs.WriteString(fmt.Sprintf("定时任务 (%d 个)\n\n", len(jobs)))
		for _, j := range jobs {
			status := "启用"
			if !j.Enabled {
				status = "暂停"
			}
			nextStr := "-"
			if j.NextRun != nil {
				nextStr = j.NextRun.Format("01-02 15:04")
			}
			cs.WriteString(fmt.Sprintf("  %s  %s  [%s]  %s  下次: %s\n",
				j.ID, j.Name, status, j.Schedule, nextStr))
		}
		cs.WriteString("\n通过对话管理: 创建/修改/删除/暂停")
		return cs.String(), false

	case "/provider":
		return sb.handleProvider(args), false

	case "/works":
		return sb.handleWorks(args), false

	case "/answer":
		if len(args) == 0 {
			return "用法: /answer <回答内容>", false
		}
		answer := strings.Join(args, " ")
		sb.Answer(answer)
		return fmt.Sprintf("已回答: %s", answer), false

	case "/exit", "/quit":
		return "再见!", true

	default:
		return fmt.Sprintf("未知命令: %s\n输入 /help 查看可用命令", command), false
	}
}

// handleProvider 处理 /provider 命令
func (sb *SessionBrain) handleProvider(args []string) string {
	if len(args) == 0 {
		return sb.providerList()
	}

	switch args[0] {
	case "list":
		return sb.providerList()

	case "add":
		// /provider add <name> <type> <base> [key] [model]
		if len(args) < 4 {
			return "用法: /provider add <name> <type> <api_base> [api_key] [model]\ntype: openai | anthropic"
		}
		name, pType, base := args[1], args[2], args[3]
		apiKey := ""
		model := ""
		if len(args) > 4 {
			apiKey = args[4]
		}
		if len(args) > 5 {
			model = args[5]
		}

		profile := config.ProviderProfile{
			Name: name, Type: pType, APIBase: base, APIKey: apiKey, DefaultModel: model,
		}
		if err := config.AddProvider(profile); err != nil {
			return fmt.Sprintf("添加失败: %v", err)
		}

		// 注册到 ProviderManager
		var p llm.Provider
		switch pType {
		case "anthropic":
			p = llm.NewAnthropicProviderDirect(name, base, apiKey)
		default:
			p = llm.NewOpenAIProviderDirect(name, base, apiKey)
		}
		sb.provider.RegisterProvider(name, p)

		return fmt.Sprintf("已添加供应商: %s (%s) %s", name, pType, base)

	case "set":
		// /provider set <name> <field> <value>
		if len(args) < 4 {
			return "用法: /provider set <name> <field> <value>\nfield: api_base | api_key | default_model | type"
		}
		name, field, value := args[1], args[2], strings.Join(args[3:], " ")
		if err := config.UpdateProviderField(name, field, value); err != nil {
			return fmt.Sprintf("修改失败: %v", err)
		}
		return fmt.Sprintf("已更新 %s.%s", name, field)

	case "remove":
		if len(args) < 2 {
			return "用法: /provider remove <name>"
		}
		name := args[1]
		// 内置供应商不可删
		builtins := map[string]bool{"openai": true, "anthropic": true, "ollama": true}
		if builtins[name] {
			return fmt.Sprintf("不能删除内置供应商: %s", name)
		}
		if err := sb.provider.RemoveProvider(name); err != nil {
			return fmt.Sprintf("移除失败: %v", err)
		}
		if err := config.RemoveProvider(name); err != nil {
			return fmt.Sprintf("DB 删除失败: %v", err)
		}
		return fmt.Sprintf("已删除供应商: %s", name)

	case "use":
		// /provider use <name> [model]
		if len(args) < 2 {
			return "用法: /provider use <name> [model]"
		}
		name := args[1]
		model := ""
		if len(args) > 2 {
			model = strings.Join(args[2:], " ")
		}
		if err := sb.provider.UseProvider(name, model); err != nil {
			return fmt.Sprintf("切换失败: %v", err)
		}
		return fmt.Sprintf("已切换到: %s (%s)", name, sb.provider.GetModel())

	case "info":
		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		return sb.providerInfo(name)

	default:
		return fmt.Sprintf("未知子命令: /provider %s\n用法: /provider [list|add|set|remove|use|info]", args[0])
	}
}

// providerList 列出所有供应商
func (sb *SessionBrain) providerList() string {
	var s strings.Builder
	activeName := sb.provider.GetActiveProviderName()
	locked := sb.provider.IsExplicitProvider()

	s.WriteString("供应商列表\n\n")

	// 内置供应商
	s.WriteString("内置:\n")
	for _, name := range sb.provider.ListProviders() {
		// 过滤自定义的
		if name == "openai" || name == "anthropic" || name == "ollama" {
			marker := "  "
			if name == activeName {
				marker = "* "
			}
			s.WriteString(fmt.Sprintf("  %s%-12s (内置)\n", marker, name))
		}
	}

	// 自定义供应商
	profiles, _ := config.ListProviderProfiles()
	if len(profiles) > 0 {
		s.WriteString("\n自定义:\n")
		for _, p := range profiles {
			marker := "  "
			if p.Name == activeName {
				marker = "* "
			}
			model := p.DefaultModel
			if model == "" {
				model = "-"
			}
			s.WriteString(fmt.Sprintf("  %s%-12s %-10s %s  模型: %s\n", marker, p.Name, p.Type, p.APIBase, model))
		}
	}

	if locked {
		s.WriteString(fmt.Sprintf("\n[锁定] 当前: %s (%s)", activeName, sb.provider.GetModel()))
	} else {
		s.WriteString(fmt.Sprintf("\n当前: %s (%s) [自动路由]", activeName, sb.provider.GetModel()))
	}

	s.WriteString("\n\n/provider add <name> <type> <base> [key] [model]")
	s.WriteString("\n/provider use <name> [model]")
	return s.String()
}

// providerInfo 显示供应商详情
func (sb *SessionBrain) providerInfo(name string) string {
	if name == "" {
		name = sb.provider.GetActiveProviderName()
	}

	var s strings.Builder
	s.WriteString(fmt.Sprintf("供应商: %s\n\n", name))

	// 尝试从 DB 获取自定义供应商详情
	if p, err := config.GetProvider(name); err == nil {
		s.WriteString(fmt.Sprintf("  类型:       %s\n", p.Type))
		s.WriteString(fmt.Sprintf("  API Base:   %s\n", p.APIBase))
		s.WriteString(fmt.Sprintf("  API Key:    %s\n", config.MaskProviderKey(p.APIKey)))
		s.WriteString(fmt.Sprintf("  默认模型:   %s\n", p.DefaultModel))
		s.WriteString(fmt.Sprintf("  创建时间:   %s\n", p.CreatedAt.Format("2006-01-02 15:04:05")))
	} else {
		// 内置供应商
		s.WriteString("  类型: 内置\n")
	}

	if sb.provider.GetActiveProviderName() == name {
		s.WriteString(fmt.Sprintf("\n  [活跃] 当前模型: %s\n", sb.provider.GetModel()))
	}
	return s.String()
}
