package telegram

import (
	"context"
	"encoding/json"
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
	bot            *tgbot.Bot
	provider       SessionProvider
	token          string
	allowedChat    int64
	lastChatID     int64 // 最近一次对话的 chatID
	cancel         context.CancelFunc
	mu             sync.Mutex
	pendingAnswers map[int64]chan string // chatID -> answer channel
}

// New 创建 Telegram Bot 实例
func New(token string, allowedChat int64, provider SessionProvider) *Bot {
	return &Bot{
		token:          token,
		allowedChat:    allowedChat,
		provider:       provider,
		pendingAnswers: make(map[int64]chan string),
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
	// 处理 InlineKeyboard 回调
	if update.CallbackQuery != nil {
		b.handleCallbackQuery(ctx, bot, update.CallbackQuery)
		return
	}

	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	text := update.Message.Text
	log.Printf("Telegram message from chat %d: %s", chatID, truncate(text, 50))
	if text == "" {
		return
	}

	// 检查是否有待回答的问题（自由文本模式）
	b.mu.Lock()
	ch, hasPending := b.pendingAnswers[chatID]
	b.mu.Unlock()
	if hasPending {
		select {
		case ch <- text:
		default:
		}
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
		case "question":
			// 发送已有内容
			if content.Len() > 0 {
				b.sendLongMessage(ctx, bot, chatID, content.String())
				content.Reset()
			}
			// 解析问题并发送 InlineKeyboard
			answer := b.handleQuestionEvent(ctx, bot, chatID, sessionID, ev.Content)
			_ = answer
		case "tool_use", "tool_call":
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

// handleCallbackQuery 处理 InlineKeyboard 按钮点击
func (b *Bot) handleCallbackQuery(ctx context.Context, bot *tgbot.Bot, cq *models.CallbackQuery) {
	data := cq.Data
	if !strings.HasPrefix(data, "askuser:") {
		return
	}

	answer := strings.TrimPrefix(data, "askuser:")
	chatID := cq.Message.Message.Chat.ID

	// 写入 pendingAnswers
	b.mu.Lock()
	ch, ok := b.pendingAnswers[chatID]
	b.mu.Unlock()

	if ok {
		select {
		case ch <- answer:
		default:
		}
	}

	// 回复回调并编辑消息标记已回答
	bot.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
		CallbackQueryID: cq.ID,
		Text:            "已选择: " + answer,
	})

	// 编辑原消息，移除按钮
	if cq.Message.Message != nil {
		bot.EditMessageReplyMarkup(ctx, &tgbot.EditMessageReplyMarkupParams{
			ChatID:    chatID,
			MessageID: cq.Message.Message.ID,
		})
		originalText := cq.Message.Message.Text
		bot.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: cq.Message.Message.ID,
			Text:      originalText + "\n\n[已回答: " + answer + "]",
		})
	}
}

// handleQuestionEvent 处理 question 流事件：发送 InlineKeyboard 或等待自由文本
func (b *Bot) handleQuestionEvent(ctx context.Context, bot *tgbot.Bot, chatID int64, sessionID, questionJSON string) string {
	var q struct {
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	if err := json.Unmarshal([]byte(questionJSON), &q); err != nil {
		return ""
	}

	// 创建 answer channel
	answerCh := make(chan string, 1)
	b.mu.Lock()
	b.pendingAnswers[chatID] = answerCh
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.pendingAnswers, chatID)
		b.mu.Unlock()
	}()

	if len(q.Options) > 0 {
		// 发送带 InlineKeyboard 的消息
		b.sendQuestionMessage(ctx, bot, chatID, q.Question, q.Options)
	} else {
		// 无选项：纯文本提问
		bot.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   "[Question] " + q.Question + "\n\n请直接回复文字作为答案",
		})
	}

	// 等待回答（5 分钟超时）
	var answer string
	select {
	case answer = <-answerCh:
	case <-time.After(5 * time.Minute):
		answer = "[timeout]"
		bot.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   "[超时] 未在 5 分钟内回答，已跳过",
		})
	}

	// 通过 /answer 命令回传到 SessionBrain
	if answer != "" {
		b.provider.RunCommand(sessionID, "/answer "+answer)
	}

	return answer
}

// sendQuestionMessage 发送带 InlineKeyboard 按钮的问题消息
func (b *Bot) sendQuestionMessage(ctx context.Context, bot *tgbot.Bot, chatID int64, question string, options []string) {
	var rows [][]models.InlineKeyboardButton
	for _, opt := range options {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: opt, CallbackData: "askuser:" + opt},
		})
	}

	bot.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID: chatID,
		Text:   "[Question] " + question,
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: rows,
		},
	})
}

// truncate 截断字符串用于日志
func truncate(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "..."
}
