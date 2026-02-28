// Package channels 提供了多渠道聊天机器人的基础接口和实现
// 该包定义了统一的频道接口，用于支持多种即时通讯平台（Telegram、WhatsApp、Discord、Slack、钉钉等）
// 所有具体的频道实现都需要实现 Channel 接口，提供消息收发和生命周期管理功能
package channels

import (
	"context"

	"github.com/Ailoc/nanogrip/internal/bus"
)

// Channel 是所有聊天频道必须实现的核心接口
// 该接口定义了频道的基本操作：
// - Name: 获取频道名称标识
// - Start: 启动频道，开始接收消息
// - Stop: 停止频道，清理资源
// - Send: 发送消息到指定聊天
type Channel interface {
	// Name 返回频道的唯一标识名称（如 "telegram", "whatsapp" 等）
	Name() string

	// Start 启动频道服务
	// ctx: 用于控制频道生命周期的上下文，可通过取消上下文来优雅关闭频道
	// 返回错误表示启动失败（如配置错误、网络问题等）
	Start(ctx context.Context) error

	// Stop 停止频道服务，释放所有资源（如关闭连接、停止轮询等）
	// 返回错误表示停止过程中出现问题
	Stop() error

	// Send 通过该频道发送消息
	// msg: 包含接收者ID、消息内容等信息的出站消息
	// 返回错误表示发送失败（如网络错误、API错误等）
	Send(msg bus.OutboundMessage) error
}

// BaseChannel 提供频道的通用功能实现
// 该结构体包含所有频道共享的基础字段和方法，可被具体频道实现嵌入使用
// 通过组合模式，避免代码重复，统一管理频道的基本属性
type BaseChannel struct {
	name    string          // 频道名称标识（如 "telegram", "whatsapp"）
	config  interface{}     // 频道特定的配置对象，由各具体实现定义
	bus     *bus.MessageBus // 消息总线，用于在频道间传递消息
	running bool            // 频道运行状态标志，true表示正在运行
}

// NewBaseChannel 创建一个新的基础频道实例
// 这是一个工厂函数，用于初始化 BaseChannel 结构体
// 参数:
//
//	name: 频道名称，应该是唯一的标识符
//	config: 频道配置对象，包含该频道所需的所有配置信息
//	bus: 消息总线实例，用于接收和发送消息
//
// 返回: 初始化后的 BaseChannel 指针
func NewBaseChannel(name string, config interface{}, bus *bus.MessageBus) *BaseChannel {
	return &BaseChannel{
		name:   name,
		config: config,
		bus:    bus,
	}
}

// Name 返回频道的名称标识
// 实现了 Channel 接口的 Name 方法
// 返回值用于在日志、管理器等处识别和引用该频道
func (c *BaseChannel) Name() string {
	return c.name
}

// IsRunning 返回频道是否正在运行
// 该方法用于检查频道状态，避免在频道未运行时执行操作
// 返回: true表示频道正在运行，false表示已停止
func (c *BaseChannel) IsRunning() bool {
	return c.running
}
