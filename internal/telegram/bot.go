package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// SessionProvider 解耦 telegram 包与 daemon 的依赖
type SessionProvider interface {
	GetOrCreateSession(chatID int64) (sessionID string, err error)
	ChatStream(sessionID string, input string) (<-chan StreamEvent, error)
	RunCommand(sessionID string, command string) (string, bool, error)
}

// StreamEvent 统一事件类型
type StreamEvent struct {
	Type    string // "content", "thinking", "tool_use", "tool_result", "error", "done"
	Content string
}

// Bot Telegram Bot 实例
type Bot struct {
	bot         *tgbot.Bot
	provider    SessionProvider
	token       string
	allowedChat int64
	lastChatID  int64 // 最近一次对话的 chatID
	cancel      context.CancelFunc
	mu          sync.Mutex
}

// New 创建 Telegram Bot 实例
func New(token string, allowedChat int64, provider SessionProvider) *Bot {
	return &Bot{
		token:       token,
		allowedChat: allowedChat,
		provider:    provider,
	}
}

// Start 启动长轮询
func (b *Bot) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	b.mu.Lock()
	b.cancel = cancel
	b.mu.Unlock()

	opts := []tgbot.Option{
		tgbot.WithDefaultHandler(b.handleMessage),
	}

	bot, err := tgbot.New(b.token, opts...)
	if err != nil {
		cancel()
		return fmt.Errorf("create telegram bot: %w", err)
	}

	b.mu.Lock()
	b.bot = bot
	b.mu.Unlock()

	// 注册 Telegram 命令菜单
	b.registerCommands(ctx, bot)

	log.Println("Telegram bot polling started")
	bot.Start(ctx)
	return nil
}

// registerCommands 向 Telegram 注册命令菜单
func (b *Bot) registerCommands(ctx context.Context, bot *tgbot.Bot) {
	commands := []models.BotCommand{
		{Command: "status", Description: "系统状态"},
		{Command: "model", Description: "查看/切换大模型"},
		{Command: "model_small", Description: "查看/切换小模型"},
		{Command: "model_reset", Description: "重置为默认模型"},
		{Command: "model_info", Description: "模型详细信息"},
		{Command: "models", Description: "列出可用模型"},
		{Command: "tools", Description: "查看可用工具"},
		{Command: "clear", Description: "清空对话历史"},
		{Command: "history", Description: "查看对话历史"},
		{Command: "tokens", Description: "Token 估算"},
		{Command: "remember", Description: "添加到长期记忆"},
		{Command: "search", Description: "搜索记忆"},
		{Command: "config", Description: "查看配置"},
		{Command: "cron", Description: "定时任务"},
		{Command: "works", Description: "工作空间管理"},
		{Command: "help", Description: "帮助"},
	}
	_, err := bot.SetMyCommands(ctx, &tgbot.SetMyCommandsParams{
		Commands: commands,
	})
	if err != nil {
		log.Printf("register telegram commands failed: %v", err)
	}
}

// Stop 停止 Bot
func (b *Bot) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
}

// LastChatID 返回最近一次对话的 chatID
func (b *Bot) LastChatID() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastChatID
}

// SendTyping 发送 typing 状态指示
func (b *Bot) SendTyping(chatID int64) error {
	b.mu.Lock()
	bot := b.bot
	b.mu.Unlock()
	if bot == nil {
		return fmt.Errorf("bot not started")
	}
	bot.SendChatAction(context.Background(), &tgbot.SendChatActionParams{
		ChatID: chatID,
		Action: models.ChatActionTyping,
	})
	return nil
}

// SendText 主动发送文本消息到指定 chatID（自动显示 typing 状态）
func (b *Bot) SendText(chatID int64, text string) error {
	b.mu.Lock()
	bot := b.bot
	b.mu.Unlock()
	if bot == nil {
		return fmt.Errorf("bot not started")
	}
	// 发送前先显示 typing
	bot.SendChatAction(context.Background(), &tgbot.SendChatActionParams{
		ChatID: chatID,
		Action: models.ChatActionTyping,
	})
	b.sendLongMessage(context.Background(), bot, chatID, text)
	return nil
}

// keepTyping 持续发送 typing 状态直到 ctx 取消
// Telegram typing 指示约 5 秒过期，每 4 秒刷新一次
func (b *Bot) keepTyping(ctx context.Context, bot *tgbot.Bot, chatID int64) {
	bot.SendChatAction(ctx, &tgbot.SendChatActionParams{
		ChatID: chatID,
		Action: models.ChatActionTyping,
	})
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			bot.SendChatAction(ctx, &tgbot.SendChatActionParams{
				ChatID: chatID,
				Action: models.ChatActionTyping,
			})
		}
	}
}

// handleMessage 处理收到的消息
func (b *Bot) handleMessage(ctx context.Context, bot *tgbot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	text := update.Message.Text
	log.Printf("Telegram message from chat %d: %s", chatID, truncate(text, 50))
	if text == "" {
		return
	}

	// 记录最近对话的 chatID
	b.mu.Lock()
	b.lastChatID = chatID
	b.mu.Unlock()

	// 检查 chat 权限
	if b.allowedChat != 0 && chatID != b.allowedChat {
		bot.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   "Unauthorized",
		})
		return
	}

	// 斜杠命令处理
	if strings.HasPrefix(text, "/") && !strings.HasPrefix(text, "/start") {
		b.handleCommand(ctx, bot, chatID, text)
		return
	}

	// /start 命令
	if strings.HasPrefix(text, "/start") {
		bot.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   "Kele Bot 已就绪，发送消息开始对话",
		})
		return
	}

	// 普通消息 -> 对话
	b.handleChat(ctx, bot, chatID, text)
}

// normalizeCommand 将 Telegram 命令格式转换为内部格式
// Telegram 命令菜单只支持下划线，内部命令使用连字符
// /model_small -> /model-small, /model_reset -> /model-reset
func normalizeCommand(text string) string {
	// 去除 Telegram 可能附带的 @botname 后缀
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return text
	}
	cmd := parts[0]
	if idx := strings.Index(cmd, "@"); idx > 0 {
		cmd = cmd[:idx]
	}
	// 下划线转连字符（仅命令部分）
	cmd = strings.ReplaceAll(cmd, "_", "-")
	parts[0] = cmd
	return strings.Join(parts, " ")
}

// handleCommand 处理斜杠命令
func (b *Bot) handleCommand(ctx context.Context, bot *tgbot.Bot, chatID int64, text string) {
	// 显示 typing 状态
	bot.SendChatAction(ctx, &tgbot.SendChatActionParams{
		ChatID: chatID,
		Action: models.ChatActionTyping,
	})

	// 规范化命令格式
	text = normalizeCommand(text)

	sessionID, err := b.provider.GetOrCreateSession(chatID)
	if err != nil {
		bot.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   fmt.Sprintf("Error: %v", err),
		})
		return
	}

	result, _, err := b.provider.RunCommand(sessionID, text)
	if err != nil {
		bot.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   fmt.Sprintf("Error: %v", err),
		})
		return
	}

	b.sendLongMessage(ctx, bot, chatID, result)
}

// handleChat 处理普通对话消息
func (b *Bot) handleChat(ctx context.Context, bot *tgbot.Bot, chatID int64, text string) {
	// 持续 typing 直到响应完成
	typingCtx, stopTyping := context.WithCancel(ctx)
	go b.keepTyping(typingCtx, bot, chatID)
	defer stopTyping()

	sessionID, err := b.provider.GetOrCreateSession(chatID)
	if err != nil {
		bot.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   fmt.Sprintf("Error: %v", err),
		})
		return
	}

	events, err := b.provider.ChatStream(sessionID, text)
	if err != nil {
		bot.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   fmt.Sprintf("Error: %v", err),
		})
		return
	}

	// 收集所有事件
	var thinking strings.Builder
	var content strings.Builder

	for ev := range events {
		switch ev.Type {
		case "thinking":
			thinking.WriteString(ev.Content)
		case "content":
			content.WriteString(ev.Content)
		case "tool_use":
			// 工具调用中，不需要额外处理
		case "tool_result":
			// 工具结果，不需要额外处理
		case "error":
			content.WriteString(fmt.Sprintf("\n[Error: %s]", ev.Content))
		case "done":
			// 完成
		}
	}

	// 拼接回复
	var reply strings.Builder
	if thinking.Len() > 0 {
		thinkText := thinking.String()
		// 截断 thinking 到 500 字
		runes := []rune(thinkText)
		if len(runes) > 500 {
			thinkText = string(runes[:500]) + "..."
		}
		reply.WriteString("<blockquote>")
		reply.WriteString(tgbot.EscapeMarkdown(thinkText))
		reply.WriteString("</blockquote>\n\n")
	}
	reply.WriteString(content.String())

	replyText := reply.String()
	if replyText == "" {
		replyText = "(empty response)"
	}

	b.sendLongMessage(ctx, bot, chatID, replyText)
}

// sendLongMessage 发送消息，超过 4096 字符自动分段
func (b *Bot) sendLongMessage(ctx context.Context, bot *tgbot.Bot, chatID int64, text string) {
	const maxLen = 4096

	for len(text) > 0 {
		chunk := text
		if len(chunk) > maxLen {
			// 在换行处截断
			cutAt := maxLen
			if idx := strings.LastIndex(text[:maxLen], "\n"); idx > maxLen/2 {
				cutAt = idx + 1
			}
			chunk = text[:cutAt]
			text = text[cutAt:]
		} else {
			text = ""
		}

		_, err := bot.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   chunk,
		})
		if err != nil {
			log.Printf("Telegram send error: %v", err)
			return
		}
	}
}

// truncate 截断字符串用于日志
func truncate(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "..."
}
