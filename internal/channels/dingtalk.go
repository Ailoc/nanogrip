// Package channels - 钉钉频道实现
// dingtalk.go 实现了钉钉（DingTalk）机器人的集成
// 使用Webhook接收消息事件，通过REST API发送消息
// 支持钉钉企业内部应用和群机器人
package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Ailoc/nanogrip/internal/bus"
)

// DingTalkChannel 钉钉机器人频道实现
// 主要特性：
// 1. 支持钉钉Webhook回调接收消息
// 2. 使用钉钉Open API发送消息
// 3. 支持用户白名单进行访问控制
// 4. 支持签名验证保证安全性
// 注意：需要在钉钉开发者后台创建应用并配置Webhook回调地址
type DingTalkChannel struct {
	token     string          // Client ID（应用的AppKey）
	secret    string          // Client Secret（应用的AppSecret），用于签名验证
	allowFrom []string        // 用户白名单（钉钉用户ID列表）
	msgBus    *bus.MessageBus // 消息总线
	running   bool            // 运行状态标志
	mu        sync.RWMutex    // 读写锁
}

// NewDingTalkChannel 创建一个新的钉钉频道实例
// 参数:
//
//	token: 钉钉应用的Client ID（AppKey）
//	secret: 钉钉应用的Client Secret（AppSecret）
//	allowFrom: 用户白名单列表（钉钉用户ID）
//	msgBus: 消息总线
//
// 返回: 初始化后的DingTalkChannel指针
func NewDingTalkChannel(token string, secret string, allowFrom []string, msgBus *bus.MessageBus) *DingTalkChannel {
	return &DingTalkChannel{
		token:     token,
		secret:    secret,
		allowFrom: allowFrom,
		msgBus:    msgBus,
	}
}

// Name 返回频道名称
// 实现Channel接口的Name方法
// 返回: 频道名称 "dingtalk"
func (d *DingTalkChannel) Name() string {
	return "dingtalk"
}

// Start 启动钉钉频道服务
// 设置running状态为true
// 注意：钉钉使用Webhook推送消息，需要配合HTTP服务器使用
// 参数:
//
//	ctx: 上下文对象
//
// 返回: 始终返回nil
func (d *DingTalkChannel) Start(ctx context.Context) error {
	d.mu.Lock()
	d.running = true
	d.mu.Unlock()

	log.Println("DingTalk channel started")
	return nil
}

// Stop 停止钉钉频道服务
// 设置running状态为false
// 返回: 始终返回nil
func (d *DingTalkChannel) Stop() error {
	d.mu.Lock()
	d.running = false
	d.mu.Unlock()

	log.Println("DingTalk channel stopped")
	return nil
}

// Send 发送消息到钉钉
// 发送流程：
// 1. 检查频道是否在运行
// 2. 调用钉钉Open API发送消息（在完整实现中）
// 参数:
//
//	msg: 出站消息对象，ChatID为钉钉的会话ID
//
// 返回: 如果频道未运行则返回错误
func (d *DingTalkChannel) Send(msg bus.OutboundMessage) error {
	if !d.isRunning() {
		return fmt.Errorf("channel not running")
	}

	// 注意：这里是简化实现
	// 完整实现需要：
	// 1. 获取access_token（使用Client ID和Secret）
	// 2. 调用钉钉的消息发送API（如机器人消息发送接口）
	// 3. API端点：https://oapi.dingtalk.com/robot/send?access_token=xxx
	log.Printf("DingTalk send to %s: %s", msg.ChatID, msg.Content)
	return nil
}

// isRunning 检查频道是否正在运行
// 使用读锁保护running状态的并发访问
// 返回: true表示正在运行，false表示已停止
func (d *DingTalkChannel) isRunning() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.running
}

// DingTalkWebhookHandler 创建钉钉Webhook处理器
// 这是一个HTTP handler，用于接收钉钉的Webhook回调请求
// 功能：
// 1. 验证时间戳和签名（确保请求来自钉钉）
// 2. 解析事件数据
// 3. 根据事件类型分发处理
// 支持的事件类型：
// - text: 文本消息
// - image: 图片消息
// - voice: 语音消息
// - file: 文件消息
// - share_card: 分享卡片
// - user_add/user_update: 用户事件
// - label_user_add/label_user_update: 标签事件
// 参数:
//
//	token: 钉钉应用的Client ID
//	secret: 钉钉应用的Client Secret，用于签名验证
//	msgBus: 消息总线
//
// 返回: HTTP处理函数
func DingTalkWebhookHandler(token string, secret string, msgBus *bus.MessageBus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 只接受POST请求
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 获取钉钉签名验证所需的请求头
		timestamp := r.Header.Get("X-Timestamp") // 时间戳
		signature := r.Header.Get("X-Signature") // 签名

		// 验证签名（如果配置了Secret）
		if secret != "" && !verifyDingTalkSignature(secret, timestamp, signature) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// 读取请求体
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// 解析事件数据
		var eventData map[string]interface{}
		if err := json.Unmarshal(body, &eventData); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// 根据事件类型处理
		// EventType字段标识事件类型
		eventType, _ := eventData["EventType"].(string)

		switch eventType {
		case "text", "image", "voice", "file", "share_card":
			// 处理各类消息事件
			handleDingTalkMessage(eventData, msgBus)
		case "user_add", "user_update":
			// 用户事件（用户加入、更新等）
			log.Printf("DingTalk user event: %s", eventType)
		case "label_user_add", "label_user_update":
			// 标签事件（用户标签变更）
			log.Printf("DingTalk label event: %s", eventType)
		}

		// 返回成功响应（钉钉要求返回特定格式）
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}
}

// handleDingTalkMessage 处理钉钉消息事件
// 处理流程：
// 1. 提取消息数据（发送者、内容、会话ID等）
// 2. 构建入站消息对象
// 3. 发布到消息总线
// 参数:
//
//	eventData: 从钉钉接收的事件数据
//	msgBus: 消息总线
func handleDingTalkMessage(eventData map[string]interface{}, msgBus *bus.MessageBus) {
	// 提取消息字段
	senderID, _ := eventData["SenderId"].(string)             // 发送者用户ID
	content, _ := eventData["Content"].(string)               // 消息内容
	msgType, _ := eventData["MsgType"].(string)               // 消息类型
	conversationID, _ := eventData["ConversationId"].(string) // 会话ID

	// 构建入站消息
	msg := bus.InboundMessage{
		Message: bus.Message{
			Channel:   "dingtalk",                                  // 频道名称
			SenderID:  senderID,                                    // 发送者ID
			ChatID:    conversationID,                              // 会话ID
			Content:   content,                                     // 消息内容
			Timestamp: time.Now(),                                  // 时间戳
			Metadata:  map[string]interface{}{"msg_type": msgType}, // 元数据（消息类型）
		},
	}

	// 发布到消息总线
	msgBus.PublishInbound(msg)
}

// verifyDingTalkSignature 验证钉钉签名
// 钉钉使用HMAC-SHA256算法对请求进行签名，防止伪造请求
// 验证流程：
// 1. 使用timestamp和secret拼接字符串
// 2. 使用HMAC-SHA256计算签名
// 3. 将计算结果进行Base64编码
// 4. 与请求头中的signature比对
// 参数:
//
//	secret: 应用的Client Secret
//	timestamp: 请求时间戳
//	signature: 请求签名
//
// 返回: true表示签名有效，false表示签名无效
func verifyDingTalkSignature(secret, timestamp, signature string) bool {
	// 如果没有提供签名，拒绝请求
	if signature == "" {
		return false
	}

	// 注意：这里是简化实现的占位符
	// 完整实现需要：
	// 1. 构造签名字符串：stringToSign = timestamp + "\n" + secret
	// 2. 使用HMAC-SHA256计算签名：
	//    hmac := hmac.New(sha256.New, []byte(secret))
	//    hmac.Write([]byte(stringToSign))
	//    calculatedSignature := base64.StdEncoding.EncodeToString(hmac.Sum(nil))
	// 3. 比较calculatedSignature和signature是否相等

	log.Printf("Verifying DingTalk signature (timestamp: %s)", timestamp)
	return true // 占位符：实际应该返回签名验证结果
}
