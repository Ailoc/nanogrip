// Package channels - Discord频道实现
// discord.go 实现了Discord Bot的集成
// 使用Discord REST API发送消息，通过Gateway（WebSocket）接收事件
// 这是一个基础实现，主要展示了API调用方式
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

// DiscordChannel Discord机器人频道实现
// 主要特性：
// 1. 使用Discord REST API v10发送消息
// 2. 需要通过Gateway（WebSocket）接收消息和事件（未完整实现）
// 3. 支持用户白名单进行访问控制
// 4. 支持频道（Channel）和私聊（DM）
// 注意：完整实现需要WebSocket连接到Discord Gateway，这里是简化版本
type DiscordChannel struct {
	*BaseChannel                       // 嵌入基础频道
	config       *config.DiscordConfig // Discord配置
	token        string                // Bot Token，用于API认证
	allowFrom    map[string]bool       // 用户白名单
	httpClient   *http.Client          // HTTP客户端
	gatewayURL   string                // Gateway URL（用于WebSocket连接）
	mu           sync.RWMutex          // 读写锁
}

// NewDiscordChannel 创建一个新的Discord频道实例
// 参数:
//
//	cfg: Discord配置对象，包含token和白名单
//	bus: 消息总线
//
// 返回: 初始化后的DiscordChannel指针
func NewDiscordChannel(cfg *config.DiscordConfig, bus *bus.MessageBus) *DiscordChannel {
	// 构建用户白名单映射表
	allowFrom := make(map[string]bool)
	for _, id := range cfg.AllowFrom {
		allowFrom[id] = true
	}

	return &DiscordChannel{
		BaseChannel: NewBaseChannel("discord", cfg, bus),
		config:      cfg,
		token:       cfg.Token,
		allowFrom:   allowFrom,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		gatewayURL:  cfg.GatewayURL,
	}
}

// Start 启动Discord机器人服务
// 启动流程：
// 1. 检查Token是否配置
// 2. 获取Gateway URL（用于后续WebSocket连接）
// 3. 设置running状态
// 注意：完整实现需要建立WebSocket连接以接收消息
// 参数:
//
//	ctx: 上下文对象
//
// 返回: 如果Token未配置或获取Gateway失败则返回错误
func (c *DiscordChannel) Start(ctx context.Context) error {
	if c.token == "" {
		return fmt.Errorf("Discord bot token not configured")
	}

	c.running = true

	// 获取Gateway URL（Discord WebSocket连接地址）
	if err := c.getGateway(); err != nil {
		return err
	}

	log.Println("Discord channel started")
	return nil
}

// Stop 停止Discord机器人服务
// 设置running标志为false
// 返回: 始终返回nil
func (c *DiscordChannel) Stop() error {
	c.running = false
	log.Println("Discord channel stopped")
	return nil
}

// Send 通过Discord发送消息
// 发送流程：
// 1. 构造消息数据（content字段）
// 2. 发送POST请求到Discord API的/channels/{channelID}/messages端点
// 3. 使用Bot Token进行认证
// 参数:
//
//	msg: 出站消息对象，ChatID为Discord的频道ID
//
// 返回: 发送失败时返回错误
func (c *DiscordChannel) Send(msg bus.OutboundMessage) error {
	channelID := msg.ChatID // Discord的频道ID

	// 构造消息数据
	data := map[string]interface{}{
		"content": msg.Content, // 消息内容（纯文本或Markdown）
	}

	body, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Discord API v10的消息发送端点
	apiURL := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	// 使用Bot Token认证，格式为 "Bot <token>"
	req.Header.Set("Authorization", "Bot "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Discord API error: %d", resp.StatusCode)
	}

	return nil
}

// getGateway 获取Discord Gateway URL
// Gateway是Discord的WebSocket连接地址，用于接收实时事件
// 工作流程：
// 1. 调用/gateway/bot API获取Gateway信息
// 2. 解析返回的URL
// 3. 保存到gatewayURL字段
// 返回: API调用失败时返回错误
func (c *DiscordChannel) getGateway() error {
	req, err := http.NewRequest("GET", "https://discord.com/api/v10/gateway/bot", nil)
	if err != nil {
		return err
	}

	// 使用Bot Token认证
	req.Header.Set("Authorization", "Bot "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Discord API error: %d", resp.StatusCode)
	}

	// 解析Gateway URL
	var result struct {
		URL string `json:"url"` // WebSocket URL（格式：wss://gateway.discord.gg）
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	c.gatewayURL = result.URL
	return nil
}

// DiscordMessage 表示Discord消息
// 包含消息的基本信息
type DiscordMessage struct {
	ID        string      `json:"id"`         // 消息ID（雪花ID）
	ChannelID string      `json:"channel_id"` // 频道ID
	Content   string      `json:"content"`    // 消息内容
	Author    DiscordUser `json:"author"`     // 作者信息
	GuildID   string      `json:"guild_id"`   // 服务器ID（私聊时为空）
}

// DiscordUser 表示Discord用户
// 包含用户的基本身份信息
type DiscordUser struct {
	ID       string `json:"id"`       // 用户ID（雪花ID）
	Username string `json:"username"` // 用户名
}
