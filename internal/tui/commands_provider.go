package tui

import (
	"fmt"
	"strings"

	"github.com/BlakeLiAFK/kele/internal/config"
	"github.com/BlakeLiAFK/kele/internal/llm"
)

// handleProviderCmd 处理 /provider 命令
func (a *App) handleProviderCmd(sess *Session, args []string) {
	if sess.brain == nil {
		sess.AddMessage("assistant", "standalone 模式下 brain 未初始化")
		return
	}

	if len(args) == 0 {
		a.handleProviderList(sess)
		return
	}

	switch args[0] {
	case "list":
		a.handleProviderList(sess)
	case "add":
		a.handleProviderAdd(sess, args[1:])
	case "set":
		a.handleProviderSet(sess, args[1:])
	case "remove":
		a.handleProviderRemove(sess, args[1:])
	case "use":
		a.handleProviderUse(sess, args[1:])
	case "info":
		a.handleProviderInfo(sess, args[1:])
	default:
		sess.AddMessage("assistant", fmt.Sprintf("未知子命令: /provider %s\n用法: /provider [list|add|set|remove|use|info]", args[0]))
	}
}

// handleProviderList 列出所有供应商
func (a *App) handleProviderList(sess *Session) {
	var s strings.Builder
	activeName := sess.brain.GetProviderName()
	locked := sess.brain.IsExplicitProvider()

	s.WriteString("供应商列表\n\n")

	// 内置供应商
	s.WriteString("内置:\n")
	for _, name := range sess.brain.ListProviders() {
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
		s.WriteString(fmt.Sprintf("\n[锁定] 当前: %s (%s)", activeName, sess.brain.GetModel()))
	} else {
		s.WriteString(fmt.Sprintf("\n当前: %s (%s) [自动路由]", activeName, sess.brain.GetModel()))
	}

	s.WriteString("\n\n/provider add <name> <type> <base> [key] [model]")
	s.WriteString("\n/provider use <name> [model]")
	sess.AddMessage("assistant", s.String())
}

// handleProviderAdd 添加自定义供应商
func (a *App) handleProviderAdd(sess *Session, args []string) {
	// /provider add <name> <type> <base> [key] [model]
	if len(args) < 3 {
		sess.AddMessage("assistant", "用法: /provider add <name> <type> <api_base> [api_key] [model]\ntype: openai | anthropic")
		return
	}
	name, pType, base := args[0], args[1], args[2]
	apiKey := ""
	model := ""
	if len(args) > 3 {
		apiKey = args[3]
	}
	if len(args) > 4 {
		model = args[4]
	}

	profile := config.ProviderProfile{
		Name: name, Type: pType, APIBase: base, APIKey: apiKey, DefaultModel: model,
	}
	if err := config.AddProvider(profile); err != nil {
		sess.AddMessage("assistant", fmt.Sprintf("添加失败: %v", err))
		return
	}

	// 注册到 ProviderManager
	var p llm.Provider
	switch pType {
	case "anthropic":
		p = llm.NewAnthropicProviderDirect(name, base, apiKey)
	default:
		p = llm.NewOpenAIProviderDirect(name, base, apiKey)
	}
	sess.brain.RegisterProvider(name, p)

	sess.AddMessage("assistant", fmt.Sprintf("已添加供应商: %s (%s) %s", name, pType, base))
}

// handleProviderSet 更新供应商字段
func (a *App) handleProviderSet(sess *Session, args []string) {
	if len(args) < 3 {
		sess.AddMessage("assistant", "用法: /provider set <name> <field> <value>\nfield: api_base | api_key | default_model | type")
		return
	}
	name, field, value := args[0], args[1], strings.Join(args[2:], " ")
	if err := config.UpdateProviderField(name, field, value); err != nil {
		sess.AddMessage("assistant", fmt.Sprintf("修改失败: %v", err))
		return
	}
	sess.AddMessage("assistant", fmt.Sprintf("已更新 %s.%s", name, field))
}

// handleProviderRemove 删除供应商
func (a *App) handleProviderRemove(sess *Session, args []string) {
	if len(args) < 1 {
		sess.AddMessage("assistant", "用法: /provider remove <name>")
		return
	}
	name := args[0]
	builtins := map[string]bool{"openai": true, "anthropic": true, "ollama": true}
	if builtins[name] {
		sess.AddMessage("assistant", fmt.Sprintf("不能删除内置供应商: %s", name))
		return
	}
	if err := sess.brain.RemoveProvider(name); err != nil {
		sess.AddMessage("assistant", fmt.Sprintf("移除失败: %v", err))
		return
	}
	if err := config.RemoveProvider(name); err != nil {
		sess.AddMessage("assistant", fmt.Sprintf("DB 删除失败: %v", err))
		return
	}
	sess.AddMessage("assistant", fmt.Sprintf("已删除供应商: %s", name))
}

// handleProviderUse 切换活跃供应商
func (a *App) handleProviderUse(sess *Session, args []string) {
	if len(args) < 1 {
		sess.AddMessage("assistant", "用法: /provider use <name> [model]")
		return
	}
	name := args[0]
	model := ""
	if len(args) > 1 {
		model = strings.Join(args[1:], " ")
	}
	if err := sess.brain.UseProvider(name, model); err != nil {
		sess.AddMessage("assistant", fmt.Sprintf("切换失败: %v", err))
		return
	}
	sess.AddMessage("assistant", fmt.Sprintf("已切换到: %s (%s)", name, sess.brain.GetModel()))
	a.updateStatus("Ready")
}

// handleProviderInfo 查看供应商详情
func (a *App) handleProviderInfo(sess *Session, args []string) {
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	if name == "" {
		name = sess.brain.GetProviderName()
	}

	var s strings.Builder
	s.WriteString(fmt.Sprintf("供应商: %s\n\n", name))

	if p, err := config.GetProvider(name); err == nil {
		s.WriteString(fmt.Sprintf("  类型:       %s\n", p.Type))
		s.WriteString(fmt.Sprintf("  API Base:   %s\n", p.APIBase))
		s.WriteString(fmt.Sprintf("  API Key:    %s\n", config.MaskProviderKey(p.APIKey)))
		s.WriteString(fmt.Sprintf("  默认模型:   %s\n", p.DefaultModel))
		s.WriteString(fmt.Sprintf("  创建时间:   %s\n", p.CreatedAt.Format("2006-01-02 15:04:05")))
	} else {
		s.WriteString("  类型: 内置\n")
	}

	if sess.brain.GetProviderName() == name {
		s.WriteString(fmt.Sprintf("\n  [活跃] 当前模型: %s\n", sess.brain.GetModel()))
	}
	sess.AddMessage("assistant", s.String())
}
