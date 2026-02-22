package tools

import (
	"fmt"
	"strings"
)

// MessageSender 消息发送接口（渠道无关）
type MessageSender interface {
	// Send 发送消息到指定渠道
	// channel: 消息渠道（如 "telegram"）
	// target: 目标 ID，空字符串表示使用默认目标
	// message: 消息内容
	Send(channel, target, message string) (string, error)

	// Channels 返回可用渠道列表
	Channels() []string
}

// SendMessageTool 消息推送工具
type SendMessageTool struct {
	sender MessageSender
}

// NewSendMessageTool 创建消息推送工具
func NewSendMessageTool(sender MessageSender) *SendMessageTool {
	return &SendMessageTool{sender: sender}
}

func (t *SendMessageTool) Name() string { return "send_message" }

func (t *SendMessageTool) Description() string {
	channels := t.sender.Channels()
	return fmt.Sprintf(
		"向用户发送消息。可用渠道: %s。用于主动通知任务进度、异常告警等。",
		strings.Join(channels, ", "),
	)
}

func (t *SendMessageTool) Parameters() map[string]interface{} {
	channels := t.sender.Channels()
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"channel": map[string]interface{}{
				"type":        "string",
				"description": fmt.Sprintf("消息渠道，可选: %s", strings.Join(channels, ", ")),
				"enum":        channels,
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "要发送的消息内容",
			},
			"target": map[string]interface{}{
				"type":        "string",
				"description": "目标 ID（如 Telegram chat ID）。省略则使用配置的默认目标",
			},
		},
		"required": []string{"channel", "message"},
	}
}

func (t *SendMessageTool) Execute(args map[string]interface{}) (string, error) {
	channel, _ := args["channel"].(string)
	if channel == "" {
		return "", fmt.Errorf("缺少 channel 参数")
	}

	message, _ := args["message"].(string)
	if message == "" {
		return "", fmt.Errorf("缺少 message 参数")
	}

	target, _ := args["target"].(string)

	return t.sender.Send(channel, target, message)
}
