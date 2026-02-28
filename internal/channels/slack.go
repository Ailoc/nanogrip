// Package channels - Slack频道实现
// slack.go 实现了Slack Bot的集成
// 使用Webhook或WebSocket接收事件，通过REST API发送消息
// 这是一个基础实现，展示了Slack事件处理的框架
package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Ailoc/nanogrip/internal/bus"
)

// SlackChannel Slack机器人频道实现
// 主要特性：
// 1. 支持Slack Events API接收消息（通过Webhook）
// 2. 使用Slack Web API发送消息
// 3. 支持用户白名单进行访问控制
// 4. 支持消息（message）和提及（app_mention）事件
// 注意：完整实现需要配置Slack App和相应的OAuth权限
type SlackChannel struct {
	token         string          // Bot Token（xoxb-开头）
	signingSecret string          // Signing Secret，用于验证Webhook请求
	channel       string          // 默认频道（未使用）
	allowFrom     []string        // 用户白名单（Slack用户ID列表）
	msgBus        *bus.MessageBus // 消息总线
	wsConn        interface{}     // WebSocket连接（基础版本未实现）
	running       bool            // 运行状态标志
	mu            sync.RWMutex    // 读写锁
}

// NewSlackChannel 创建一个新的Slack频道实例
// 参数:
//
//	token: Slack Bot Token（用于API认证）
//	signingSecret: Slack Signing Secret（用于验证Webhook请求）
//	allowFrom: 用户白名单列表
//	msgBus: 消息总线
//
// 返回: 初始化后的SlackChannel指针
func NewSlackChannel(token string, signingSecret string, allowFrom []string, msgBus *bus.MessageBus) *SlackChannel {
	return &SlackChannel{
		token:         token,
		signingSecret: signingSecret,
		allowFrom:     allowFrom,
		msgBus:        msgBus,
	}
}

// Name 返回频道名称
// 实现Channel接口的Name方法
// 返回: 频道名称 "slack"
func (s *SlackChannel) Name() string {
	return "slack"
}

// Start 启动Slack频道服务
// 设置running状态为true
// 注意：完整的Slack集成需要配合HTTP服务器接收Webhook事件
// 参数:
//
//	ctx: 上下文对象
//
// 返回: 始终返回nil
func (s *SlackChannel) Start(ctx context.Context) error {
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	// 注意：这是基础实现的占位符
	// 完整实现需要：
	// 1. 建立WebSocket连接（Socket Mode）或
	// 2. 启动HTTP服务器接收Webhook（Events API）

	log.Println("Slack channel started (basic implementation)")
	return nil
}

// Stop 停止Slack频道服务
// 设置running状态为false
// 返回: 始终返回nil
func (s *SlackChannel) Stop() error {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	log.Println("Slack channel stopped")
	return nil
}

// Send 发送消息到Slack
// 发送流程：
// 1. 检查频道是否在运行
// 2. 调用Slack Web API发送消息（在完整实现中）
// 参数:
//
//	msg: 出站消息对象，ChatID为Slack的频道ID或用户ID
//
// 返回: 如果频道未运行则返回错误
func (s *SlackChannel) Send(msg bus.OutboundMessage) error {
	if !s.isRunning() {
		return fmt.Errorf("channel not running")
	}

	// 注意：这里是简化实现
	// 完整实现需要调用Slack的chat.postMessage API
	// API端点：https://slack.com/api/chat.postMessage
	// 需要在请求头中包含 "Authorization: Bearer {token}"
	log.Printf("Slack send to %s: %s", msg.ChatID, msg.Content)
	return nil
}

// isRunning 检查频道是否正在运行
// 使用读锁保护running状态的并发访问
// 返回: true表示正在运行，false表示已停止
func (s *SlackChannel) isRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// HandleSlackEvent 处理Slack事件
// 这个方法由Webhook处理器调用，用于处理不同类型的Slack事件
// 支持的事件类型：
// - message: 普通消息
// - app_mention: 机器人被提及
// 参数:
//
//	eventData: 从Slack接收的事件数据（map格式）
func (s *SlackChannel) HandleSlackEvent(eventData map[string]interface{}) {
	eventType, _ := eventData["type"].(string)

	switch eventType {
	case "message":
		s.handleMessage(eventData)
	case "app_mention":
		s.handleMention(eventData)
	}
}

// handleMessage 处理普通消息事件
// 处理流程：
// 1. 提取消息数据（用户、文本、频道）
// 2. 检查用户是否在白名单中
// 3. 构建入站消息并发布到消息总线
// 参数:
//
//	eventData: 消息事件数据
func (s *SlackChannel) handleMessage(eventData map[string]interface{}) {
	user, _ := eventData["user"].(string)       // 发送者用户ID
	text, _ := eventData["text"].(string)       // 消息文本
	channel, _ := eventData["channel"].(string) // 频道ID

	// 检查用户是否在白名单中
	if !s.isAllowed(user) {
		return
	}

	// 构建入站消息
	msg := bus.InboundMessage{
		Message: bus.Message{
			Channel:   "slack",
			SenderID:  user,
			ChatID:    channel,
			Content:   text,
			Timestamp: time.Now(),
		},
	}

	s.msgBus.PublishInbound(msg)
}

// handleMention 处理机器人被提及的事件
// 当有人在消息中@机器人时触发
// 处理流程与handleMessage类似
// 参数:
//
//	eventData: 提及事件数据
func (s *SlackChannel) handleMention(eventData map[string]interface{}) {
	user, _ := eventData["user"].(string)
	text, _ := eventData["text"].(string)
	channel, _ := eventData["channel"].(string)

	if !s.isAllowed(user) {
		return
	}

	msg := bus.InboundMessage{
		Message: bus.Message{
			Channel:   "slack",
			SenderID:  user,
			ChatID:    channel,
			Content:   text,
			Timestamp: time.Now(),
		},
	}

	s.msgBus.PublishInbound(msg)
}

// isAllowed 检查用户是否在白名单中
// 如果白名单为空，则允许所有用户
// 参数:
//
//	user: Slack用户ID
//
// 返回: true表示允许，false表示拒绝
func (s *SlackChannel) isAllowed(user string) bool {
	// 如果白名单为空，允许所有用户
	if len(s.allowFrom) == 0 {
		return true
	}

	// 检查用户是否在白名单中
	for _, allowed := range s.allowFrom {
		if allowed == user {
			return true
		}
	}

	return false
}

// SlackWebhookHandler 创建Slack Webhook处理器
// 这是一个HTTP handler，用于接收Slack的Webhook请求
// 功能：
// 1. 验证Slack签名（确保请求来自Slack）
// 2. 处理URL验证挑战（Slack配置时的验证步骤）
// 3. 解析和处理事件数据
// 参数:
//
//	token: Slack Bot Token
//	signingSecret: Slack Signing Secret
//	msgBus: 消息总线
//
// 返回: HTTP处理函数
func SlackWebhookHandler(token string, signingSecret string, msgBus *bus.MessageBus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 只接受POST请求
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 注意：完整实现需要验证Slack签名
		// 使用signingSecret和请求头中的X-Slack-Signature进行HMAC验证
		// 参考：https://api.slack.com/authentication/verifying-requests-from-slack

		// 解析请求体
		var eventData map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&eventData); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// 处理URL验证挑战
		// Slack在配置Events API时会发送一个challenge参数
		// 需要原样返回以完成验证
		if challenge, ok := eventData["challenge"].(string); ok {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(challenge))
			return
		}

		// 注意：在完整实现中，这里需要创建频道实例并处理事件
		// 可以根据event字段提取实际的事件数据并调用HandleSlackEvent
		log.Printf("Slack webhook received: %+v", eventData)
		w.WriteHeader(http.StatusOK)
	}
}
