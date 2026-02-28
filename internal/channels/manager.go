// Package channels 频道管理器
// manager.go 实现了多频道的统一管理功能
// 负责根据配置文件启动、停止和管理所有已启用的聊天频道
package channels

import (
	"context"
	"log"
	"sync"

	"github.com/Ailoc/nanogrip/internal/bus"
	"github.com/Ailoc/nanogrip/internal/config"
)

// Manager 频道管理器，负责管理多个聊天频道的生命周期
// 主要功能：
// 1. 根据配置文件创建和初始化各个频道
// 2. 统一启动和停止所有频道
// 3. 提供频道查询和列表功能
// 4. 使用读写锁保证并发安全
type Manager struct {
	bus      *bus.MessageBus    // 消息总线，用于在频道间传递消息
	cfg      *config.Config     // 全局配置对象，包含所有频道的配置信息
	channels map[string]Channel // 频道映射表，key为频道名称，value为频道实例
	wg       sync.WaitGroup     // 等待组，用于优雅关闭时等待所有goroutine完成
	mu       sync.RWMutex       // 读写锁，保护channels映射表的并发访问
}

// NewManager 创建一个新的频道管理器实例
// 参数:
//
//	bus: 消息总线实例，所有频道共享同一个消息总线
//	cfg: 配置对象，包含所有频道的启用状态和配置参数
//
// 返回: 初始化后的Manager指针，channels为空map，等待StartAll调用
func NewManager(bus *bus.MessageBus, cfg *config.Config) *Manager {
	return &Manager{
		bus:      bus,
		cfg:      cfg,
		channels: make(map[string]Channel),
	}
}

// StartAll 启动所有已启用的频道
// 该方法会遍历配置文件中所有频道的启用状态，逐个创建和启动频道实例
// 工作流程：
// 1. 检查频道是否在配置中启用（Enabled字段）
// 2. 创建频道实例，传入配置和消息总线
// 3. 调用频道的Start方法启动服务
// 4. 如果启动成功，将频道加入到channels映射表中
// 5. 如果启动失败，记录错误但不影响其他频道的启动
// 参数:
//
//	ctx: 上下文对象，用于控制频道的生命周期和优雅关闭
//
// 返回: 始终返回nil（各频道启动失败不会导致方法失败）
func (m *Manager) StartAll(ctx context.Context) error {
	// 启动 Telegram 频道
	// Telegram使用HTTP长轮询方式接收消息，通过REST API发送消息
	if m.cfg.Channels.Telegram.Enabled {
		ch := NewTelegramChannel(&m.cfg.Channels.Telegram, m.bus)
		if err := ch.Start(ctx); err != nil {
			log.Printf("Failed to start Telegram: %v", err)
		} else {
			m.mu.Lock()
			m.channels["telegram"] = ch
			m.mu.Unlock()
			log.Println("Telegram channel started")
		}
	}

	// 启动 WhatsApp 频道
	// WhatsApp通过桥接服务器（Bridge）进行通信，使用REST API收发消息
	if m.cfg.Channels.WhatsApp.Enabled {
		ch := NewWhatsAppChannel(&m.cfg.Channels.WhatsApp, m.bus)
		if err := ch.Start(ctx); err != nil {
			log.Printf("Failed to start WhatsApp: %v", err)
		} else {
			m.mu.Lock()
			m.channels["whatsapp"] = ch
			m.mu.Unlock()
			log.Println("WhatsApp channel started")
		}
	}

	// 启动 Discord 频道
	// Discord使用WebSocket Gateway接收事件，通过REST API发送消息
	if m.cfg.Channels.Discord.Enabled {
		ch := NewDiscordChannel(&m.cfg.Channels.Discord, m.bus)
		if err := ch.Start(ctx); err != nil {
			log.Printf("Failed to start Discord: %v", err)
		} else {
			m.mu.Lock()
			m.channels["discord"] = ch
			m.mu.Unlock()
			log.Println("Discord channel started")
		}
	}

	// 启动 Slack 频道
	// Slack使用WebSocket或Webhook接收事件，通过REST API发送消息
	if m.cfg.Channels.Slack.Enabled {
		ch := NewSlackChannel(
			m.cfg.Channels.Slack.BotToken,
			m.cfg.Channels.Slack.AppToken,
			m.cfg.Channels.Slack.DM.AllowFrom,
			m.bus,
		)
		if err := ch.Start(ctx); err != nil {
			log.Printf("Failed to start Slack: %v", err)
		} else {
			m.mu.Lock()
			m.channels["slack"] = ch
			m.mu.Unlock()
			log.Println("Slack channel started")
		}
	}

	// 启动钉钉频道
	// 钉钉使用Webhook回调接收消息，通过REST API发送消息
	if m.cfg.Channels.DingTalk.Enabled {
		ch := NewDingTalkChannel(
			m.cfg.Channels.DingTalk.ClientID,
			m.cfg.Channels.DingTalk.ClientSecret,
			m.cfg.Channels.DingTalk.AllowFrom,
			m.bus,
		)
		if err := ch.Start(ctx); err != nil {
			log.Printf("Failed to start DingTalk: %v", err)
		} else {
			m.mu.Lock()
			m.channels["dingtalk"] = ch
			m.mu.Unlock()
			log.Println("DingTalk channel started")
		}
	}

	// 可以在此处添加更多频道的启动逻辑
	// 每个频道的启动流程保持一致：检查启用状态 -> 创建实例 -> 启动 -> 注册到映射表

	return nil
}

// StopAll 停止所有正在运行的频道
// 该方法会遍历所有已注册的频道，逐个调用Stop方法进行优雅关闭
// 使用读锁保证在停止过程中不会有新的频道被添加或删除
// 每个频道的Stop方法负责释放资源（关闭连接、停止goroutine等）
func (m *Manager) StopAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 遍历所有频道，逐个停止
	for name, ch := range m.channels {
		log.Printf("Stopping channel: %s", name)
		ch.Stop()
	}
}

// GetChannel 根据名称获取指定的频道实例
// 该方法用于在运行时访问特定频道，例如手动发送消息或查询频道状态
// 参数:
//
//	name: 频道名称，如 "telegram", "whatsapp", "discord" 等
//
// 返回: 频道实例，如果频道不存在或未启动则返回nil
func (m *Manager) GetChannel(name string) Channel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.channels[name]
}

// ListChannels 列出所有正在运行的频道名称
// 该方法返回当前所有已启动并注册的频道名称列表
// 可用于监控、调试或用户界面显示
// 返回: 频道名称的字符串切片，如 ["telegram", "discord", "slack"]
func (m *Manager) ListChannels() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 创建与频道数量相同容量的切片，避免动态扩容
	names := make([]string, 0, len(m.channels))
	for name := range m.channels {
		names = append(names, name)
	}
	return names
}
