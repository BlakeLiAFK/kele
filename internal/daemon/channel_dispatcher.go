package daemon

import (
	"fmt"
	"strconv"
	"sync"

	tgbot "github.com/BlakeLiAFK/kele/internal/telegram"
)

// ChannelDispatcher 消息分发器，实现 tools.MessageSender 接口
type ChannelDispatcher struct {
	mu          sync.RWMutex
	telegram    *tgbot.Bot
	defaultChat int64
}

// NewChannelDispatcher 创建消息分发器
func NewChannelDispatcher() *ChannelDispatcher {
	return &ChannelDispatcher{}
}

// RegisterTelegram 注册 Telegram 渠道
func (d *ChannelDispatcher) RegisterTelegram(bot *tgbot.Bot, defaultChat int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.telegram = bot
	d.defaultChat = defaultChat
}

// Send 发送消息到指定渠道
func (d *ChannelDispatcher) Send(channel, target, message string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	switch channel {
	case "telegram":
		return d.sendTelegram(target, message)
	default:
		return "", fmt.Errorf("未知渠道: %s，可用: %v", channel, d.Channels())
	}
}

// sendTelegram 通过 Telegram 发送消息
func (d *ChannelDispatcher) sendTelegram(target, message string) (string, error) {
	if d.telegram == nil {
		return "", fmt.Errorf("Telegram 渠道未配置")
	}

	chatID := d.defaultChat
	if target != "" {
		id, err := strconv.ParseInt(target, 10, 64)
		if err != nil {
			return "", fmt.Errorf("无效的 chat ID: %s", target)
		}
		chatID = id
	}

	if chatID == 0 {
		return "", fmt.Errorf("未指定目标且无默认 chat ID")
	}

	if err := d.telegram.SendText(chatID, message); err != nil {
		return "", fmt.Errorf("发送失败: %w", err)
	}

	return fmt.Sprintf("消息已发送到 Telegram (chat: %d)", chatID), nil
}

// Channels 返回可用渠道列表
func (d *ChannelDispatcher) Channels() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var channels []string
	if d.telegram != nil {
		channels = append(channels, "telegram")
	}
	return channels
}
