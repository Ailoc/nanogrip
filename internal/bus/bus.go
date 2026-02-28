// Package bus 实现了一个异步消息总线,用于解耦系统组件之间的通信。
//
// 消息总线的核心作用:
//  1. 解耦组件: 通过消息总线,各个通道适配器(Channel Adapter)和智能体(Agent)
//     之间无需直接依赖,降低了系统的耦合度。
//  2. 异步处理: 采用基于 Go channel 的缓冲队列,实现了生产者-消费者模式,
//     允许消息的异步发送和接收。
//  3. 双向通信: 提供了 inbound(入站)和 outbound(出站)两个独立的消息通道,
//     分别处理从通道到智能体的消息和从智能体到通道的消息。
//  4. 流量控制: 通过缓冲区大小控制消息队列长度,防止系统过载。
package bus

import (
	"context"
	"sync"
	"time"
)

// Message 表示一个聊天消息的基础结构。
// 这是所有消息类型的通用数据结构,包含了消息的完整信息。
type Message struct {
	ID         string                 // 消息的唯一标识符
	Channel    string                 // 消息来源的通道名称(如 "telegram", "wechat" 等)
	SenderID   string                 // 发送者的用户 ID
	ChatID     string                 // 聊天会话的 ID(可能是群组 ID 或私聊 ID)
	Content    string                 // 消息的文本内容
	Media      []string               // 媒体文件的 URL 列表(图片、视频、文件等)
	Metadata   map[string]interface{} // 附加的元数据,用于存储通道特定的扩展信息
	Timestamp  time.Time              // 消息的时间戳
	SessionKey string                 // 会话密钥,用于关联和追踪会话上下文
}

// InboundMessage 表示从外部通道进入系统的入站消息。
// 这种消息由各个通道适配器(如 Telegram、微信等)接收后发布到消息总线,
// 然后由智能体消费并处理。入站消息流向: 通道 -> MessageBus -> 智能体
type InboundMessage struct {
	Message // 嵌入 Message 结构体,继承所有字段
}

// OutboundMessage 表示从系统发送到外部通道的出站消息。
// 这种消息由智能体处理完成后发布到消息总线,然后由相应的通道适配器
// 消费并发送给最终用户。出站消息流向: 智能体 -> MessageBus -> 通道
type OutboundMessage struct {
	Channel  string                 // 目标通道名称,指定消息应该发送到哪个通道
	ChatID   string                 // 目标聊天会话 ID
	Content  string                 // 要发送的文本内容
	Media    []string               // 要发送的媒体文件 URL 列表
	Metadata map[string]interface{} // 附加的元数据,可用于通道特定的发送选项
}

// MessageBus 是一个异步消息总线,用于实现通道和智能体之间的解耦通信。
//
// 工作原理:
// - 使用两个独立的 Go channel 作为消息队列(inbound 和 outbound)
// - 每个 channel 都有缓冲区,允许一定数量的消息排队等待处理
// - 采用发布-订阅模式:生产者发布消息,消费者订阅并消费消息
// - 支持优雅关闭:通过 context 控制生命周期,确保资源正确释放
//
// 线程安全:
// - Go channel 本身是线程安全的,支持多个 goroutine 并发读写
// - context 用于协调多个 goroutine 的生命周期
// - sync.WaitGroup 用于等待所有后台任务完成
type MessageBus struct {
	inbound  chan InboundMessage  // 入站消息通道:存储从通道适配器到智能体的消息
	outbound chan OutboundMessage // 出站消息通道:存储从智能体到通道适配器的消息
	ctx      context.Context      // 上下文对象,用于控制消息总线的生命周期
	cancel   context.CancelFunc   // 取消函数,用于触发消息总线的关闭流程
	wg       sync.WaitGroup       // 等待组,用于等待所有后台 goroutine 完成
}

// New 创建并返回一个新的 MessageBus 实例。
//
// 参数:
//
//	bufferSize - 每个消息通道的缓冲区大小。缓冲区决定了在消费者处理之前,
//	             可以排队等待的最大消息数量。较大的缓冲区可以提高吞吐量,
//	             但会占用更多内存;较小的缓冲区可能导致发布者阻塞。
//
// 返回:
//
//	*MessageBus - 初始化完成的消息总线实例
//
// 缓冲区的作用:
// - 当生产者的速度快于消费者时,缓冲区可以临时存储消息,避免生产者阻塞
// - 如果缓冲区满了,PublishInbound 和 PublishOutbound 会返回 ErrBusFull 错误
// - 缓冲区大小的选择需要根据实际的消息吞吐量和延迟要求来平衡
func New(bufferSize int) *MessageBus {
	ctx, cancel := context.WithCancel(context.Background())
	return &MessageBus{
		inbound:  make(chan InboundMessage, bufferSize),  // 创建带缓冲的入站消息通道
		outbound: make(chan OutboundMessage, bufferSize), // 创建带缓冲的出站消息通道
		ctx:      ctx,                                    // 设置上下文
		cancel:   cancel,                                 // 保存取消函数
	}
}

// PublishInbound 发布一条入站消息到消息总线。
// 这个方法由通道适配器调用,将从外部接收的消息发送到智能体。
//
// 参数:
//
//	msg - 要发布的入站消息
//
// 返回:
//
//	error - 如果发布成功返回 nil;如果缓冲区已满返回 ErrBusFull;
//	        如果消息总线已关闭返回 context 错误
//
// 发布机制(使用 select 语句实现非阻塞发布):
// 1. case b.inbound <- msg: 尝试将消息发送到入站通道
//   - 如果通道有空间,消息立即被放入缓冲区,返回 nil
//   - 如果通道已满且没有消费者在读取,会尝试下一个 case
//
// 2. case <-b.ctx.Done(): 检查消息总线是否已关闭
//   - 如果已关闭,返回 context 的错误信息
//
// 3. default: 如果上述两个 case 都无法执行(即通道满且总线未关闭)
//   - 立即返回 ErrBusFull 错误,避免阻塞调用者
//
// 注意事项:
// - 这是一个非阻塞操作,不会因为缓冲区满而永久等待
// - 调用者应该处理 ErrBusFull 错误,可以选择重试或丢弃消息
func (b *MessageBus) PublishInbound(msg InboundMessage) error {
	select {
	case b.inbound <- msg:
		return nil
	case <-b.ctx.Done():
		return b.ctx.Err()
	default:
		return ErrBusFull
	}
}

// ConsumeInbound 从消息总线消费下一条入站消息。
// 这个方法由智能体调用,从通道适配器接收需要处理的消息。
//
// 参数:
//
//	ctx - 上下文对象,用于控制消费操作的超时和取消
//
// 返回:
//
//	InboundMessage - 消费到的入站消息
//	error - 如果成功消费返回 nil;如果 context 被取消或超时返回相应错误
//
// 消费机制(使用 select 语句实现可取消的阻塞消费):
// 1. case msg := <-b.inbound: 从入站通道接收消息
//   - 如果通道中有消息,立即读取并返回
//   - 如果通道为空,会阻塞等待,直到有新消息到达或 context 被取消
//
// 2. case <-ctx.Done(): 等待 context 取消信号
//   - 如果 context 被取消(超时或主动取消),返回空消息和 context 错误
//   - 这允许调用者设置超时或在需要时中断等待
//
// 使用场景:
// - 智能体在循环中调用此方法,等待并处理来自各个通道的消息
// - 可以通过传入带超时的 context 来避免无限期等待
// - 多个 goroutine 可以同时消费,消息会被平均分配(竞争消费模式)
func (b *MessageBus) ConsumeInbound(ctx context.Context) (InboundMessage, error) {
	select {
	case msg := <-b.inbound:
		return msg, nil
	case <-ctx.Done():
		return InboundMessage{}, ctx.Err()
	}
}

// PublishOutbound 发布一条出站消息到消息总线。
// 这个方法由智能体调用,将处理完成的响应发送给通道适配器。
//
// 参数:
//
//	msg - 要发布的出站消息
//
// 返回:
//
//	error - 如果发布成功返回 nil;如果缓冲区已满返回 ErrBusFull;
//	        如果消息总线已关闭返回 context 错误
//
// 发布机制(与 PublishInbound 相同的非阻塞模式):
// 1. case b.outbound <- msg: 尝试将消息发送到出站通道
//   - 如果通道有空间,消息立即被放入缓冲区,返回 nil
//   - 如果通道已满,会尝试下一个 case
//
// 2. case <-b.ctx.Done(): 检查消息总线是否已关闭
//   - 如果已关闭,返回 context 的错误信息
//
// 3. default: 如果通道满且总线未关闭
//   - 立即返回 ErrBusFull 错误,避免阻塞调用者
//
// 使用场景:
// - 智能体处理完入站消息后,通过此方法发送响应
// - 响应会被放入出站队列,等待相应的通道适配器消费并发送
func (b *MessageBus) PublishOutbound(msg OutboundMessage) error {
	select {
	case b.outbound <- msg:
		return nil
	case <-b.ctx.Done():
		return b.ctx.Err()
	default:
		return ErrBusFull
	}
}

// ConsumeOutbound 从消息总线消费下一条出站消息。
// 这个方法由通道适配器调用,接收智能体发送的响应消息并发送给最终用户。
//
// 参数:
//
//	ctx - 上下文对象,用于控制消费操作的超时和取消
//
// 返回:
//
//	OutboundMessage - 消费到的出站消息
//	error - 如果成功消费返回 nil;如果 context 被取消或超时返回相应错误
//
// 消费机制(与 ConsumeInbound 相同的可取消阻塞模式):
// 1. case msg := <-b.outbound: 从出站通道接收消息
//   - 如果通道中有消息,立即读取并返回
//   - 如果通道为空,会阻塞等待,直到有新消息到达或 context 被取消
//
// 2. case <-ctx.Done(): 等待 context 取消信号
//   - 如果 context 被取消,返回空消息和 context 错误
//
// 使用场景:
// - 各个通道适配器在循环中调用此方法,等待并发送出站消息
// - 每个通道适配器只处理发送给自己的消息(通过 Channel 字段识别)
// - 多个通道适配器可以同时消费,根据 Channel 字段过滤自己的消息
func (b *MessageBus) ConsumeOutbound(ctx context.Context) (OutboundMessage, error) {
	select {
	case msg := <-b.outbound:
		return msg, nil
	case <-ctx.Done():
		return OutboundMessage{}, ctx.Err()
	}
}

// InboundSize 返回当前入站消息队列中待处理的消息数量。
//
// 返回:
//
//	int - 入站通道缓冲区中的消息数量
//
// 用途:
// - 监控消息积压情况,如果返回值接近缓冲区大小,说明智能体处理速度较慢
// - 用于系统健康检查和性能指标收集
// - 可以根据队列长度动态调整处理策略(如增加处理 goroutine 数量)
//
// 注意:
// - len() 函数对 channel 的调用是原子操作,线程安全
// - 返回值只是瞬时快照,调用后队列长度可能立即发生变化
func (b *MessageBus) InboundSize() int {
	return len(b.inbound)
}

// OutboundSize 返回当前出站消息队列中待发送的消息数量。
//
// 返回:
//
//	int - 出站通道缓冲区中的消息数量
//
// 用途:
// - 监控消息发送积压情况,如果返回值较大,说明通道适配器发送速度较慢
// - 用于系统健康检查和性能指标收集
// - 可以根据队列长度判断特定通道是否出现发送瓶颈
//
// 注意:
// - len() 函数对 channel 的调用是原子操作,线程安全
// - 返回值只是瞬时快照,调用后队列长度可能立即发生变化
func (b *MessageBus) OutboundSize() int {
	return len(b.outbound)
}

// Close 优雅地关闭消息总线,释放所有资源。
//
// 关闭流程:
// 1. 调用 cancel() 函数,触发 context 取消
//   - 这会导致所有阻塞在 PublishInbound/PublishOutbound 的调用收到取消信号
//   - 所有正在进行的 ConsumeInbound/ConsumeOutbound 调用也会收到取消信号
//
// 2. 等待所有后台 goroutine 完成(通过 sync.WaitGroup)
//   - 确保所有正在处理的任务都已完成
//
// 3. 关闭 inbound 和 outbound 通道
//   - 关闭 channel 后,任何尝试发送到这些 channel 的操作都会 panic
//   - 但从已关闭的 channel 接收消息是安全的,会返回零值和 false
//
// 注意事项:
// - Close() 应该只被调用一次,通常在程序退出时调用
// - 调用 Close() 后不应再使用此 MessageBus 实例
// - 关闭操作是幂等的,多次调用不会造成问题(但第二次调用会 panic,因为 channel 已关闭)
// - 确保在关闭前停止所有生产者和消费者的创建
func (b *MessageBus) Close() {
	b.cancel()        // 触发 context 取消,通知所有正在等待的操作
	b.wg.Wait()       // 等待所有后台 goroutine 完成
	close(b.inbound)  // 关闭入站通道
	close(b.outbound) // 关闭出站通道
}

// ErrBusFull 是当消息总线的缓冲区已满时返回的错误。
//
// 何时返回此错误:
// - 当调用 PublishInbound 时,如果入站通道的缓冲区已满
// - 当调用 PublishOutbound 时,如果出站通道的缓冲区已满
//
// 错误处理建议:
// 1. 重试策略: 可以在短暂延迟后重试发布操作
// 2. 丢弃消息: 对于非关键消息,可以选择丢弃并记录日志
// 3. 背压处理: 将错误传播到上游,让消息源减缓发送速度
// 4. 增加缓冲区: 如果频繁出现此错误,考虑增加 bufferSize 参数
// 5. 性能优化: 检查消费者的处理速度,优化处理逻辑
//
// 这是一个全局变量,所有 MessageBus 实例共享同一个错误对象。
var ErrBusFull = &BusError{"message bus is full"}

// BusError 是消息总线相关错误的类型。
// 实现了 error 接口,可以直接作为错误返回。
type BusError struct {
	msg string // 错误消息内容
}

// Error 实现 error 接口的 Error() 方法,返回错误消息字符串。
func (e *BusError) Error() string {
	return e.msg
}
