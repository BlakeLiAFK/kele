package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

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

	log.Println("Telegram bot polling started")
	bot.Start(ctx)
	return nil
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

// handleMessage 处理收到的消息
func (b *Bot) handleMessage(ctx context.Context, bot *tgbot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	text := update.Message.Text
	if text == "" {
		return
	}

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

// handleCommand 处理斜杠命令
func (b *Bot) handleCommand(ctx context.Context, bot *tgbot.Bot, chatID int64, text string) {
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
