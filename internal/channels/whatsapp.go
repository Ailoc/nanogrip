// Package channels - WhatsApp频道实现
// whatsapp.go 实现了WhatsApp Business API的集成
// 通过桥接服务器（Bridge Server）进行通信
// 使用REST API轮询接收消息，通过POST请求发送消息
package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Ailoc/nanogrip/internal/bus"
	"github.com/Ailoc/nanogrip/internal/config"
)

// WhatsAppChannel WhatsApp频道实现
// 主要特性：
// 1. 通过Bridge Server与WhatsApp通信（因为WhatsApp没有官方Bot API）
// 2. 使用REST API轮询方式接收消息
// 3. 支持用户白名单进行访问控制
// 4. 需要配置Bridge服务器的URL和认证Token
// 注意：需要独立部署WhatsApp Bridge服务器，如whatsmeow、baileys等
type WhatsAppChannel struct {
	*BaseChannel                        // 嵌入基础频道
	config       *config.WhatsAppConfig // WhatsApp配置
	bridgeURL    string                 // Bridge服务器的URL
	bridgeToken  string                 // Bridge服务器的认证Token
	allowFrom    map[string]bool        // 用户白名单
	httpClient   *http.Client           // HTTP客户端
	mu           sync.RWMutex           // 读写锁
}

// NewWhatsAppChannel 创建一个新的WhatsApp频道实例
// 参数:
//
//	cfg: WhatsApp配置对象
//	bus: 消息总线
//
// 返回: 初始化后的WhatsAppChannel指针
func NewWhatsAppChannel(cfg *config.WhatsAppConfig, bus *bus.MessageBus) *WhatsAppChannel {
	// 构建用户白名单映射表
	allowFrom := make(map[string]bool)
	for _, id := range cfg.AllowFrom {
		allowFrom[id] = true
	}

	return &WhatsAppChannel{
		BaseChannel: NewBaseChannel("whatsapp", cfg, bus),
		config:      cfg,
		bridgeURL:   cfg.BridgeURL,
		bridgeToken: cfg.BridgeToken,
		allowFrom:   allowFrom,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Start 启动WhatsApp频道服务
// 启动流程：
// 1. 检查Bridge URL是否配置
// 2. 设置running状态
// 3. 启动消息轮询goroutine
// 参数:
//
//	ctx: 上下文对象，用于控制生命周期
//
// 返回: 如果Bridge URL未配置则返回错误
func (c *WhatsAppChannel) Start(ctx context.Context) error {
	if c.bridgeURL == "" {
		return fmt.Errorf("WhatsApp bridge URL not configured")
	}

	c.running = true

	// 启动消息轮询goroutine（通过REST API轮询）
	go c.pollMessages(ctx)

	log.Println("WhatsApp channel started")
	return nil
}

// Stop 停止WhatsApp频道服务
// 设置running标志为false，轮询会自动停止
// 返回: 始终返回nil
func (c *WhatsAppChannel) Stop() error {
	c.running = false
	log.Println("WhatsApp channel stopped")
	return nil
}

// Send 通过WhatsApp发送消息
// 发送流程：
// 1. 构造消息数据（chatId和message）
// 2. 发送POST请求到Bridge服务器的/send端点
// 3. 如果配置了Token，在请求头中添加Authorization
// 参数:
//
//	msg: 出站消息对象
//
// 返回: 发送失败时返回错误
func (c *WhatsAppChannel) Send(msg bus.OutboundMessage) error {
	// 构造发送给Bridge的消息数据
	// Bridge服务器会将消息转发到WhatsApp
	data := map[string]interface{}{
		"chatId":  msg.ChatID,  // WhatsApp的聊天ID（通常是电话号码@s.whatsapp.net）
		"message": msg.Content, // 消息内容
	}

	body, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// 向Bridge服务器发送消息
	req, err := http.NewRequest("POST", c.bridgeURL+"/send", strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	// 如果配置了Token，添加认证头
	if c.bridgeToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bridgeToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("WhatsApp API error: %d", resp.StatusCode)
	}

	return nil
}

// pollMessages 轮询WhatsApp消息
// 工作流程：
// 1. 循环检查running状态和上下文
// 2. 调用getMessages从Bridge获取新消息
// 3. 为每条消息启动goroutine进行处理
// 4. 如果出错，等待5秒后重试
// 5. 正常情况下每2秒轮询一次
// 参数:
//
//	ctx: 上下文对象
func (c *WhatsAppChannel) pollMessages(ctx context.Context) {
	for c.running {
		select {
		case <-ctx.Done():
			return
		default:
			// 从Bridge服务器获取消息
			messages, err := c.getMessages()
			if err != nil {
				log.Printf("WhatsApp polling error: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}

			// 处理每条消息
			for _, msg := range messages {
				go c.handleMessage(msg)
			}

			// 轮询间隔，避免过于频繁的请求
			time.Sleep(2 * time.Second)
		}
	}
}

// getMessages 从Bridge服务器获取消息
// 发送GET请求到Bridge的/messages端点
// 返回: 消息列表和可能的错误
func (c *WhatsAppChannel) getMessages() ([]WhatsAppMessage, error) {
	req, err := http.NewRequest("GET", c.bridgeURL+"/messages", nil)
	if err != nil {
		return nil, err
	}

	// 如果配置了Token，添加认证头
	if c.bridgeToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bridgeToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("WhatsApp API error: %d", resp.StatusCode)
	}

	// 解析Bridge返回的消息列表
	var result struct {
		Messages []WhatsAppMessage `json:"messages"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Messages, nil
}

// handleMessage 处理接收到的WhatsApp消息
// 处理流程：
// 1. 检查发送者是否在白名单中
// 2. 构建入站消息对象
// 3. 发布到消息总线
// 参数:
//
//	msg: WhatsApp消息对象
func (c *WhatsAppChannel) handleMessage(msg WhatsAppMessage) {
	// 白名单检查
	if len(c.allowFrom) > 0 {
		if !c.allowFrom[msg.From] {
			log.Printf("Ignoring message from unauthorized user: %s", msg.From)
			return
		}
	}

	// 构建入站消息并发布到消息总线
	inbound := bus.InboundMessage{
		Message: bus.Message{
			ID:       msg.ID,     // 消息ID
			Channel:  "whatsapp", // 频道名称
			SenderID: msg.From,   // 发送者ID（电话号码）
			ChatID:   msg.From,   // 聊天ID（WhatsApp私聊中与发送者ID相同）
			Content:  msg.Body,   // 消息内容
		},
	}

	if err := c.bus.PublishInbound(inbound); err != nil {
		log.Printf("Error publishing inbound message: %v", err)
	}
}

// WhatsAppMessage 表示WhatsApp消息
// 包含消息的基本信息
type WhatsAppMessage struct {
	ID   string `json:"id"`   // 消息唯一ID
	From string `json:"from"` // 发送者ID（电话号码，格式：+86xxxxxxxxxxxx@s.whatsapp.net）
	Body string `json:"body"` // 消息内容（文本）
	Type string `json:"type"` // 消息类型：text, image, video, audio, document等
}
