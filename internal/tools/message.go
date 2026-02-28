package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// message.go - 消息发送工具
// 此文件实现了向聊天频道发送消息的工具，支持多种消息平台

// MessageTool 提供消息发送功能
// 允许代理向指定的聊天频道发送消息，支持Telegram、WhatsApp、Discord等
type MessageTool struct {
	BaseTool
	sendChan chan<- string // 消息发送通道，用于异步发送消息
	channel  string        // 当前上下文频道
	chatID   string        // 当前上下文聊天ID
}

// NewMessageTool 创建一个新的消息工具
// 参数:
//
//	sendChan: 消息发送通道，工具会将消息发送到此通道
//
// 返回:
//
//	配置好的MessageTool实例
func NewMessageTool(sendChan chan<- string) *MessageTool {
	return &MessageTool{
		BaseTool: NewBaseTool(
			"message",
			"Send a message to a chat channel. Supports text and images (via URLs or local paths).",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"content": map[string]interface{}{
						"type":        "string",
						"description": "Message content to send",
					},
					"channel": map[string]interface{}{
						"type":        "string",
						"description": "Channel name (telegram, whatsapp, discord, etc.)",
					},
					"chat_id": map[string]interface{}{
						"type":        "string",
						"description": "Chat ID to send to",
					},
					"media": map[string]interface{}{
						"type":        "string",
						"description": "Media URL or local file path to send (images, files). For multiple, separate with commas.",
					},
					"media_type": map[string]interface{}{
						"type":        "string",
						"description": "Media type: photo, document, audio, video. Default: photo",
					},
				},
				"required": []string{"content"},
			},
		),
		sendChan: sendChan,
	}
}

// SetContext 设置当前上下文（频道和聊天ID）
// 这允许工具记住当前处理的会话，以便发送消息到正确的位置
// 参数:
//
//	channel: 当前频道
//	chatID: 当前聊天ID
func (t *MessageTool) SetContext(channel string, chatID string) {
	t.channel = channel
	t.chatID = chatID
}

// Execute 发送消息
// 将消息内容、频道和聊天ID打包成JSON，通过通道发送
// 如果未指定 channel 或 chat_id，将使用 SetContext 设置的默认值
// 支持发送图片、视频等媒体文件
// 参数:
//
//	ctx: 上下文对象
//	params: 参数map，必须包含"content"，可选"channel"、"chat_id"、"media"和"media_type"
//
// 返回:
//
//	成功返回"Message sent"，失败返回错误信息
func (t *MessageTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// 获取消息内容
	content, ok := params["content"].(string)
	if !ok || content == "" {
		return "", fmt.Errorf("missing or invalid content parameter")
	}

	// 获取可选参数，优先使用参数值，否则使用上下文默认值
	channel, _ := params["channel"].(string)
	chatID, _ := params["chat_id"].(string)

	// 获取媒体参数
	media, _ := params["media"].(string)
	mediaType, _ := params["media_type"].(string)

	// 如果未提供，使用上下文默认值
	if channel == "" {
		channel = t.channel
	}
	if chatID == "" {
		chatID = t.chatID
	}

	// 构建消息对象
	msg := map[string]interface{}{
		"content": content,
	}

	// 添加可选字段
	if channel != "" {
		msg["channel"] = channel
	}
	if chatID != "" {
		msg["chat_id"] = chatID
	}
	if media != "" {
		msg["media"] = media
	}
	if mediaType != "" {
		msg["media_type"] = mediaType
	}

	// 序列化为JSON
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return "", err
	}

	// 尝试发送到通道（非阻塞）
	select {
	case t.sendChan <- string(msgJSON):
		return "Message sent", nil
	default:
		return "", fmt.Errorf("message channel not ready")
	}
}
