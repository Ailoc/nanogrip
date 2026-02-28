package heartbeat

import (
	"context" // context 用于控制协程生命周期
	"log"     // log 用于日志记录
	"sync"    // sync 用于同步原语
	"time"    // time 用于时间处理

	"github.com/Ailoc/nanogrip/internal/bus" // 消息总线
)

// HeartbeatConfig 心跳配置
// 用于配置心跳系统的各种参数
type HeartbeatConfig struct {
	Enabled  bool   `yaml:"enabled"`  // 是否启用心跳
	Interval int    `yaml:"interval"` // 心跳间隔（秒）
	Message  string `yaml:"message"`  // 心跳消息
	Cron     string `yaml:"cron"`     // Cron 表达式（与 Interval 二选一）
	Channel  string `yaml:"channel"`  // 目标通道
	ChatID   string `yaml:"chatId"`   // 目标聊天 ID
}

// Heartbeat 心跳系统
// 允许机器人主动发送消息（定时任务/唤醒）
type Heartbeat struct {
	cfg     *HeartbeatConfig   // 心跳配置
	msgBus  *bus.MessageBus    // 消息总线
	running bool               // 运行状态
	mu      sync.RWMutex       // 读写锁
	ctx     context.Context    // 上下文（用于取消）
	cancel  context.CancelFunc // 取消函数
}

// NewHeartbeat 创建新的心跳系统
// 参数：
//   - cfg: 心跳配置
//   - msgBus: 消息总线
//
// 返回：
//   - *Heartbeat: 新的心跳实例
func NewHeartbeat(cfg *HeartbeatConfig, msgBus *bus.MessageBus) *Heartbeat {
	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())

	return &Heartbeat{
		cfg:     cfg,    // 心跳配置
		msgBus:  msgBus, // 消息总线
		running: false,  // 初始状态为未运行
		ctx:     ctx,    // 上下文
		cancel:  cancel, // 取消函数
	}
}

// Start 启动心跳系统
// 如果未启用配置，直接返回
// 支持两种模式：
//  1. 固定间隔模式：每 Interval 秒发送一次消息
//  2. Cron 模式：按照 Cron 表达式发送消息
func (h *Heartbeat) Start() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 检查是否启用
	if !h.cfg.Enabled {
		log.Println("Heartbeat is disabled")
		return nil
	}

	// 检查是否已经在运行
	if h.running {
		log.Println("Heartbeat is already running")
		return nil
	}

	// 设置为运行状态
	h.running = true

	// 根据配置选择模式
	if h.cfg.Cron != "" {
		// TODO: 实现 Cron 模式
		log.Printf("Heartbeat started with cron: %s", h.cfg.Cron)
	} else if h.cfg.Interval > 0 {
		// 固定间隔模式
		go h.runInterval()
		log.Printf("Heartbeat started with interval: %d seconds", h.cfg.Interval)
	} else {
		log.Println("Heartbeat: no valid schedule configured")
		h.running = false
		return nil
	}

	return nil
}

// runInterval 运行固定间隔模式
// 每隔指定时间发送一次心跳消息
func (h *Heartbeat) runInterval() {
	// 创建定时器
	ticker := time.NewTicker(time.Duration(h.cfg.Interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		// 如果上下文被取消，退出
		case <-h.ctx.Done():
			return
		// 定时触发
		case <-ticker.C:
			h.sendHeartbeat()
		}
	}
}

// sendHeartbeat 发送心跳消息
// 构建出站消息并发布到消息总线
func (h *Heartbeat) sendHeartbeat() {
	// 构建消息
	msg := bus.OutboundMessage{
		Channel: h.cfg.Channel, // 通道
		ChatID:  h.cfg.ChatID,  // 聊天 ID
		Content: h.cfg.Message, // 消息内容
		Metadata: map[string]interface{}{
			"type": "heartbeat", // 消息类型
		},
	}

	// 发布到消息总线
	if err := h.msgBus.PublishOutbound(msg); err != nil {
		log.Printf("Heartbeat: failed to send message: %v", err)
	} else {
		log.Printf("Heartbeat: message sent to %s", h.cfg.Channel)
	}
}

// Stop 停止心跳系统
func (h *Heartbeat) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.running {
		return
	}

	// 取消上下文，停止所有协程
	h.cancel()
	h.running = false

	log.Println("Heartbeat stopped")
}

// IsRunning 返回心跳系统是否在运行
func (h *Heartbeat) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.running
}
