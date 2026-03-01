// Package cron 提供基于时间堆的高效任务调度服务
//
// 该服务使用最小堆(min heap)数据结构来高效管理定时任务。
// 核心优化：
//  1. 使用最小堆按 NextRun 时间排序，O(1) 获取下一个任务
//  2. 智能唤醒机制：计算到下一个任务的等待时间，而非固定轮询
//  3. 支持系统级触发器：at 命令、systemd timer
//
// 性能对比：
//   - 传统轮询：每秒检查，CPU 占用高
//   - 本实现：计算精确等待时间，CPU 占用接近零
//
// 堆数据结构说明：
//   - 最小堆是一个完全二叉树，父节点总是小于等于子节点
//   - 堆顶（索引 0）始终是最早要执行的任务
//   - 插入和删除操作的时间复杂度都是 O(log n)
//   - 查看堆顶元素是 O(1) 操作
//
// 调度策略：
//  1. 所有任务按照 NextRun 时间组织在最小堆中
//  2. 计算到下一个任务的精确等待时间，使用 time.Sleep
//  3. 唤醒后执行到期的任务
//  4. 执行后的任务重新计算下次运行时间，重新插入堆中
//  5. 一次性任务（DeleteAfterRun=true）执行后直接删除
//
// 支持的调度类型：
//   - "every": 固定间隔执行（如每 5 秒）
//   - "cron": Cron 表达式（如 "0 9 * * *"）
//   - "at": 指定时间点执行一次
//   - "system": 系统级触发（使用 at 命令）
//
// 线程安全：
//   - 使用 sync.RWMutex 保护任务 map 和堆
//   - 任务执行在独立的 goroutine 中进行，不阻塞调度循环
package cron

import (
	"container/heap"
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Ailoc/nanogrip/internal/bus"
	"github.com/robfig/cron/v3"
)

// AgentExecutor 定义 Agent 执行器接口
// 用于在定时任务到期时触发 Agent 执行命令
type AgentExecutor interface {
	// ProcessDirect 直接处理命令并返回结果
	ProcessDirect(ctx context.Context, message string) (string, error)

	// SetToolContext 设置工具上下文（用于 Cron 任务执行时设置正确的 Channel 和 ChatID）
	SetToolContext(channel, chatID string)
}

// Job 表示一个定时任务
type Job struct {
	ID             string    // 任务唯一标识符
	Name           string    // 任务名称
	Message        string    // 要发送的消息内容（兼容旧模式）
	Schedule       Schedule  // 调度配置
	Channel        string    // 目标频道
	To             string    // 接收者
	Deliver        bool      // 是否立即发送
	DeleteAfterRun bool      // 执行后是否删除（一次性任务）
	CreatedAt      time.Time // 任务创建时间
	NextRun        time.Time // 下次执行时间（堆排序的关键字段）

	// Agent 模式支持（方案4）
	TriggerAgent  bool   // 是否触发 Agent 执行（true=执行命令, false=发送固定消息）
	AgentCommand  string // Agent 要执行的命令内容
}

// Schedule 表示任务的调度配置
type Schedule struct {
	Kind     string // 调度类型: "every"（固定间隔）, "cron"（cron表达式）, "at"（指定时间）
	EveryMs  int64  // "every" 类型的间隔时间（毫秒）
	CronExpr string // "cron" 类型的表达式（如 "0 9 * * *"）
	TZ       string // cron 表达式的时区
	AtMs     int64  // "at" 类型的执行时间戳（毫秒）
}

// CronService 管理定时任务的调度和执行
//
// 核心设计：
//  1. 使用最小堆维护任务队列，按 NextRun 时间排序
//  2. 使用 map 存储所有任务，支持快速查找和删除
//  3. 运行独立的 goroutine 进行任务调度
//  4. 任务执行通过回调函数 runner 进行
//  5. 支持 Agent 模式：可通过 AgentExecutor 触发 AI 执行复杂任务
type CronService struct {
	jobs     map[string]*Job // 任务 map，键为任务 ID，用于快速查找
	heap     *jobHeap        // 最小堆，按 NextRun 时间排序任务
	mu       sync.RWMutex    // 读写锁，保护 jobs 和 heap
	runner   func(job *Job)  // 任务执行回调函数（兼容旧版，优先使用 agentExecutor）

	// Agent 模式支持
	agentExecutor AgentExecutor  // Agent 执行器，用于触发 AI 命令执行
	messageBus    *bus.MessageBus // 消息总线，用于发送消息结果（使用具体类型以匹配接口）

	stopChan chan struct{}   // 停止信号通道
	stopOnce sync.Once       // 确保 Stop 只执行一次
	wg       sync.WaitGroup  // 等待组，用于跟踪调度 goroutine
}

// 移除之前的 MessageBus 接口定义，直接使用 bus.MessageBus

// jobHeapItem 是堆中的元素
// 包装 Job 并维护在堆中的索引，用于高效的更新和删除操作
type jobHeapItem struct {
	job   *Job // 指向任务的指针
	index int  // 元素在堆数组中的索引（用于 heap.Fix 等操作）
}

// jobHeap 实现 heap.Interface 接口，创建一个基于 NextRun 的最小堆
//
// 堆的工作原理：
//   - Len: 返回堆中元素数量
//   - Less: 定义堆的排序规则（NextRun 早的任务排在前面）
//   - Swap: 交换两个元素位置，并更新它们的 index
//   - Push: 向堆中添加元素
//   - Pop: 从堆中移除并返回最后一个元素（由 heap 包调用）
//
// 堆维护特性：
//   - 堆顶（索引0）始终是 NextRun 最早的任务
//   - 父节点的 NextRun 总是早于或等于子节点
//   - 这样可以在 O(1) 时间内获取下一个要执行的任务
type jobHeap []*jobHeapItem

// Len 返回堆中元素数量
func (h jobHeap) Len() int { return len(h) }

// Less 定义堆的排序规则
// i < j 当且仅当 job[i].NextRun 早于 job[j].NextRun
// 这使得最早要执行的任务总是在堆顶
func (h jobHeap) Less(i, j int) bool { return h[i].job.NextRun.Before(h[j].job.NextRun) }

// Swap 交换堆中两个元素的位置
// 同时更新两个元素的 index 字段，保持索引的正确性
func (h jobHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i // 更新交换后的索引
	h[j].index = j
}

// Push 向堆中添加一个新元素
// 该方法由 heap.Push 调用，会将元素添加到数组末尾，然后由堆算法上浮到正确位置
func (h *jobHeap) Push(x interface{}) {
	item := x.(*jobHeapItem)
	item.index = len(*h) // 设置元素在堆中的索引
	*h = append(*h, item)
}

// Pop 从堆中移除并返回最后一个元素
// 该方法由 heap.Pop 调用，实际上会先将堆顶元素移到末尾，然后调用此方法移除
func (h *jobHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]  // 取出最后一个元素
	old[n-1] = nil    // 避免内存泄漏
	*h = old[0 : n-1] // 缩小切片
	return item
}

// NewCronService 创建一个新的定时任务服务
//
// 参数：
//   - runner: 任务执行回调函数（可选，用于兼容旧版）
//            如果设置了 AgentExecutor，runner 将被忽略
//
// 返回：
//   - *CronService: 任务服务实例
func NewCronService(runner func(job *Job)) *CronService {
	return &CronService{
		jobs:     make(map[string]*Job),
		heap:     &jobHeap{},
		runner:   runner,
		stopChan: make(chan struct{}),
	}
}

// SetAgentExecutor 设置 Agent 执行器
// 用于在 Agent 模式下触发 AI 命令执行
func (c *CronService) SetAgentExecutor(executor AgentExecutor) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.agentExecutor = executor
}

// SetMessageBus 设置消息总线
// 用于发送 Agent 执行结果到通信通道
func (c *CronService) SetMessageBus(msgBus *bus.MessageBus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messageBus = msgBus
}

// Start 启动定时任务服务
//
// 启动一个后台 goroutine 运行任务调度循环。
// 该方法立即返回，调度在后台进行。
func (c *CronService) Start() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.runLoop()
	}()
}

// Stop 停止定时任务服务
//
// 关闭停止信号通道，导致调度循环退出。
// 已经在执行的任务不会被中断。
// 等待调度循环 goroutine 完成。
func (c *CronService) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopChan)

		// 等待调度循环完成
		done := make(chan struct{})
		go func() {
			c.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			log.Println("Cron service stopped")
		case <-time.After(5 * time.Second):
			log.Println("Warning: Cron service stop timeout")
		}
	})
}

// AddJob 添加一个新的定时任务
//
// 工作流程：
//  1. 生成唯一的任务 ID（基于当前时间戳）
//  2. 设置任务创建时间
//  3. 计算首次执行时间（NextRun）
//  4. 将任务添加到 jobs map
//  5. 将任务包装成 jobHeapItem 并插入最小堆
//
// 堆操作说明：
//   - heap.Push 会将任务添加到堆中，并自动维护堆的性质
//   - 新任务会根据 NextRun 时间自动排序到正确位置
//   - 时间复杂度: O(log n)
//
// 参数：
//   - job: 要添加的任务
//
// 返回：
//   - *Job: 添加后的任务（已设置 ID 和时间信息）
func (c *CronService) AddJob(job *Job) *Job {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	job.ID = fmt.Sprintf("job_%d", now.UnixNano())
	job.CreatedAt = now
	job.NextRun = c.calculateNextRun(job.Schedule)

	// 【性能优化】简化日志输出
	timeUntilRun := job.NextRun.Sub(now)
	log.Printf("[Cron] ✓ 添加任务: %s (Kind=%s), 执行时间: %v, 距离: %v",
		job.Name, job.Schedule.Kind, job.NextRun.Format("15:04:05"), timeUntilRun.Round(time.Second))

	c.jobs[job.ID] = job
	heap.Push(c.heap, &jobHeapItem{job: job, index: len(*c.heap)}) // 插入堆，O(log n)

	return job
}

// executeJob 执行任务（支持 Agent 模式和 Message 模式）
func (c *CronService) executeJob(job *Job) {
	// 记录任务信息以便调试
	log.Printf("[Cron] 📋 任务详情: ID=%s, Name=%s, Channel=%q, ChatID=%q, TriggerAgent=%v",
		job.ID, job.Name, job.Channel, job.To, job.TriggerAgent)

	// 优先使用 Agent 模式
	if job.TriggerAgent {
		c.executeAgentJob(job)
		return
	}

	// 兼容旧版：使用 runner 回调
	if c.runner != nil {
		log.Printf("[Cron] 📨 使用 runner 回调发送消息")
		c.runner(job)
	} else {
		log.Printf("[Cron] ⚠ 警告：runner 为 nil，任务 %s 未执行", job.Name)
	}
}

// executeAgentJob 执行 Agent 模式的任务
func (c *CronService) executeAgentJob(job *Job) {
	// 【关键调试】在函数入口就输出日志
	log.Printf("[Cron] 🔵 executeAgentJob 开始执行: 任务=%s", job.Name)
	// 强制刷新日志
	// os.Stdout.Sync()

	log.Printf("[Cron] 🔵 准备获取锁...")
	c.mu.RLock()
	log.Printf("[Cron] 🔵 已获取锁")
	executor := c.agentExecutor
	msgBus := c.messageBus
	c.mu.RUnlock()
	// 强制刷新日志
	// os.Stdout.Sync()

	log.Printf("[Cron] 🔵 获取锁后: executor=%v, msgBus=%v", executor != nil, msgBus != nil)

	if executor == nil {
		log.Printf("[Cron] ⚠ 警告：AgentExecutor 未设置，任务 %s 无法执行", job.Name)
		return
	}

	if msgBus == nil {
		log.Printf("[Cron] ⚠ 警告：MessageBus 未设置，任务 %s 无法发送结果", job.Name)
		return
	}

	log.Printf("[Cron] 🤖 触发 Agent 执行前: 命令=%s, Channel=%s, ChatID=%s", job.AgentCommand, job.Channel, job.To)

	// 验证任务字段
	if job.Channel == "" {
		log.Printf("[Cron] ⚠ 警告：任务的 Channel 为空！")
		return
	}
	if job.To == "" {
		log.Printf("[Cron] ⚠ 警告：任务的 ChatID 为空！")
		return
	}

	// 【关键修复】在执行 Agent 前，设置工具上下文
	// 这样 Agent 调用 cron 工具添加子任务时，会使用正确的 Channel 和 ChatID
	executor.SetToolContext(job.Channel, job.To)
	log.Printf("[Cron] ✓ 已设置工具上下文: Channel=%s, ChatID=%s", job.Channel, job.To)

	// 调用 Agent 执行命令
	log.Printf("[Cron] 🔄 准备调用 ProcessDirect...")
	ctx := context.Background()
	response, err := executor.ProcessDirect(ctx, job.AgentCommand)
	log.Printf("[Cron] 🔄 ProcessDirect 返回: response长度=%d, err=%v", len(response), err)

	if err != nil {
		log.Printf("[Cron] ❌ Agent 执行失败: %v", err)
		// 发送错误消息
		c.sendResult(msgBus, job, fmt.Sprintf("❌ 任务执行失败: %v", err))
		return
	}

	log.Printf("[Cron] ✓ Agent 执行成功，响应长度: %d 字符", len(response))

	// 发送 Agent 的响应结果
	c.sendResult(msgBus, job, response)

	log.Printf("[Cron] ✅ executeAgentJob 执行完成")
}

// sendResult 发送任务执行结果到通信通道
func (c *CronService) sendResult(msgBus *bus.MessageBus, job *Job, content string) {
	log.Printf("[Cron] sendResult 被调用: jobName=%s, channel=%s, chatID=%s", job.Name, job.Channel, job.To)

	// 验证必要的字段
	if job.Channel == "" {
		log.Printf("[Cron] ⚠ 警告：任务 %s 的 Channel 为空，无法发送消息", job.Name)
		return
	}
	if job.To == "" {
		log.Printf("[Cron] ⚠ 警告：任务 %s 的 ChatID 为空，无法发送消息", job.Name)
		return
	}

	// 使用 bus 包定义的 OutboundMessage 结构
	msg := bus.OutboundMessage{
		Channel:  job.Channel,
		ChatID:   job.To,
		Content:  content,
		Metadata: map[string]interface{}{
			"from_cron": true, // 标记消息来自 cron
		},
	}

	// 安全地截取内容前 50 字符用于日志显示
	contentPreview := content
	if len(contentPreview) > 50 {
		contentPreview = contentPreview[:50] + "..."
	}
	log.Printf("[Cron] 📤 准备发送消息: Channel=%s, ChatID=%s, Content=%s",
		job.Channel, job.To, contentPreview)

	if err := msgBus.PublishOutbound(msg); err != nil {
		log.Printf("[Cron] ❌ 发送消息失败: %v", err)
	} else {
		log.Printf("[Cron] ✓ 消息已发送到 %s (%s)", job.Channel, job.To)
	}
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RemoveJob 删除一个任务
//
// 工作流程：
//  1. 从 jobs map 中删除任务（O(1)）
//  2. 从堆中删除对应的任务项（O(n) 查找 + O(log n) 删除）
//
// 注意：由于堆中需要通过遍历找到要删除元素的索引，
// 删除操作的时间复杂度是 O(n)。对于大量任务场景，
// 如果频繁删除，可以考虑维护一个 id -> index 的映射。
//
// 参数：
//   - id: 任务 ID
//
// 返回：
//   - bool: 如果任务存在并被删除返回 true，否则返回 false
func (c *CronService) RemoveJob(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查任务是否存在，不存在返回false
	if _, ok := c.jobs[id]; !ok {
		return false
	}

	// 从 map 中删除
	delete(c.jobs, id)

	// 从堆中删除对应的任务项
	// 需要遍历堆找到匹配的索引
	for i := 0; i < len(*c.heap); i++ {
		item := (*c.heap)[i]
		if item.job.ID == id {
			// 使用 heap.Remove 从堆中删除
			// 时间复杂度：O(log n)
			heap.Remove(c.heap, i)
			log.Printf("[Cron] ✓ 任务已删除: %s", id)
			return true
		}
	}

	// 如果在堆中未找到，可能已经被执行并清理
	log.Printf("[Cron] ⚠ 警告：任务 %s 在 map 中存在但不在堆中", id)
	return true
}

// ListJobs 列出所有任务
//
// 返回：
//   - []*Job: 所有任务的列表
func (c *CronService) ListJobs() []*Job {
	c.mu.RLock()
	defer c.mu.RUnlock()

	jobs := make([]*Job, 0, len(c.jobs))
	for _, job := range c.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

// calculateNextRun 计算任务的下次执行时间
//
// 根据调度类型计算：
//   - "every": 当前时间 + 固定间隔
//   - "at": 指定的时间戳
//   - "cron": 根据 cron 表达式计算（使用 robfig/cron 库）
//
// 参数：
//   - schedule: 调度配置
//
// 返回：
//   - time.Time: 下次执行时间
func (c *CronService) calculateNextRun(schedule Schedule) time.Time {
	now := time.Now()

	switch schedule.Kind {
	case "every":
		return now.Add(time.Duration(schedule.EveryMs) * time.Millisecond)
	case "at":
		// 关键修复：time.UnixMilli 返回 UTC 时间，需要转换为本地时间
		// 否则与 time.Now() 比较时会出现时区不匹配
		return time.UnixMilli(schedule.AtMs).In(now.Location())
	case "cron":
		// 使用 robfig/cron 库解析 cron 表达式
		// 支持 5 字段格式：分 时 日 月 周
		return c.calculateCronNextRun(schedule, now)
	default:
		return now
	}
}

// calculateCronNextRun 使用 cron 表达式计算下次执行时间
//
// 支持标准 cron 表达式格式：
//   - 5 字段：分 时 日 月 周 (如 "0 9 * * *" 表示每天 9 点)
//   - 支持特殊字符：* / , -
//
// 时区处理：
//   - 如果指定了 TZ 字段，使用该时区计算
//   - 否则使用本地时区
func (c *CronService) calculateCronNextRun(schedule Schedule, now time.Time) time.Time {
	// 创建 cron parser，使用 5 字段格式（分 时 日 月 周）
	// 注意：不包含秒字段，与标准 Linux cron 一致
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

	// 解析 cron 表达式
	sched, err := parser.Parse(schedule.CronExpr)
	if err != nil {
		log.Printf("[Cron] ⚠ 警告：解析 cron 表达式失败: %s, 错误: %v", schedule.CronExpr, err)
		// 解析失败时，默认 1 小时后执行
		return now.Add(1 * time.Hour)
	}

	// 处理时区
	baseTime := now
	if schedule.TZ != "" {
		// 如果指定了时区，转换为该时区
		loc, err := time.LoadLocation(schedule.TZ)
		if err != nil {
			log.Printf("[Cron] ⚠ 警告：加载时区失败: %s, 错误: %v，使用本地时区", schedule.TZ, err)
		} else {
			baseTime = now.In(loc)
		}
	}

	// 计算下次执行时间
	nextTime := sched.Next(baseTime)

	log.Printf("[Cron] ✓ Cron 表达式解析成功: %s -> 下次执行: %v",
		schedule.CronExpr, nextTime.Format("2006-01-02 15:04:05"))

	return nextTime
}

// runLoop 运行任务调度循环（智能唤醒版本）
//
// 核心优化：使用精确等待时间，而非固定轮询
//
// 工作流程：
//  1. 获取堆顶任务的 NextRun 时间
//  2. 计算到下一个任务的等待时间（duration = NextRun - now）
//  3. 如果没有任务，阻塞等待（可被 stopChan 中断）
//  4. 如果等待时间 > 0，使用 time.Sleep 精确等待
//  5. 如果等待时间 <= 0，立即执行（任务已到期）
//  6. 执行完成后，返回步骤 1 继续
//
// 性能优势：
//   - 传统轮询（1秒）：每秒唤醒，即使没有任务需要执行
//   - 本实现：只在有任务到期时才唤醒，CPU 占用接近零
//   - 例如：下一个任务在 1 小时后，当前线程睡眠 1 小时
func (c *CronService) runLoop() {
	for {
		c.mu.RLock()
		hasJobs := c.heap.Len() > 0
		c.mu.RUnlock()

		if !hasJobs {
			// 没有任务时，睡眠较长时间（1分钟）以响应新任务
			// 这样可以避免频繁唤醒，同时保持对新任务的响应性
			select {
			case <-c.stopChan:
				return
			case <-time.After(60 * time.Second):
				// 唤醒后继续循环，检查是否有新任务
				continue
			}
		}

		// 获取下一个任务的等待时间
		waitDuration := c.getNextWaitDuration()

		if waitDuration > 0 {
			// 【性能优化】分批等待以避免错过新添加的任务
			// 最长等待 10 秒，确保新任务最多延迟 10 秒就被检查
			maxWait := waitDuration
			if maxWait > 10*time.Second {
				maxWait = 10 * time.Second
			}

			select {
			case <-c.stopChan:
				return
			case <-time.After(maxWait):
				// 等待时间到，检查并执行任务
				c.checkAndRun()
			}
		} else {
			// 等待时间 <= 0，立即检查任务
			c.checkAndRun()
		}
	}
}

// getNextWaitDuration 计算到下一个任务的精确等待时间
//
// 返回值：
//   - time.Duration: 等待时间
//   - 如果没有任务，返回最大值（阻塞直到有新任务）
func (c *CronService) getNextWaitDuration() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.heap.Len() == 0 {
		// 没有任务，返回较长等待时间
		return 60 * time.Second
	}

	// 获取堆顶任务的下次执行时间
	nextRun := (*c.heap)[0].job.NextRun
	now := time.Now()
	duration := nextRun.Sub(now)

	if nextRun.Before(now) || nextRun.Equal(now) {
		// 任务已到期或刚好到期，立即执行（返回 0）
		return 0
	}

	// 返回精确的等待时间
	return duration
}

// checkAndRun 检查并执行所有到期的任务
//
// 使用堆的优势：
//  1. 堆顶（索引0）总是下一个要执行的任务
//  2. 只需检查堆顶，如果堆顶未到期，所有任务都未到期
//  3. 执行一个任务后，从堆中弹出，下一个任务自动成为新的堆顶
//
// 工作流程：
//  1. 循环检查堆顶任务
//  2. 如果堆顶任务的 NextRun 晚于当前时间，退出循环
//  3. 从堆中弹出该任务（heap.Pop，时间复杂度 O(log n)）
//  4. 在新的 goroutine 中执行任务（不阻塞调度循环）
//  5. 如果是一次性任务，从 map 中删除
//  6. 如果是周期性任务，重新计算 NextRun 并插入堆
//
// 堆操作的时间复杂度：
//   - 查看堆顶: O(1)
//   - 弹出堆顶: O(log n)
//   - 插入任务: O(log n)
//
// 因此，即使有成千上万个任务，调度效率仍然很高。
func (c *CronService) checkAndRun() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	processedCount := 0

	// 持续检查堆顶，处理所有到期的任务
	for c.heap.Len() > 0 {
		item := (*c.heap)[0] // 查看堆顶任务（最早要执行的任务）

		if item.job.NextRun.After(now) {
			// 堆顶任务未到期，说明所有任务都未到期，退出
			break
		}

		// 从堆中弹出到期任务（O(log n)）
		heap.Pop(c.heap)

		// 【性能优化】只在有任务执行时才输出日志，避免频繁 I/O
		modeDesc := "message"
		if item.job.TriggerAgent {
			modeDesc = "agent"
		}
		log.Printf("[Cron] ✓ 执行任务: %s (Kind=%s, Mode=%s)", item.job.Name, item.job.Schedule.Kind, modeDesc)
		processedCount++

		// 在独立 goroutine 中执行任务，避免阻塞调度循环
		jobCopy := *item.job // 复制任务，避免并发问题
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[Cron] ❌ 任务执行 panic: %v", r)
				}
			}()
			log.Printf("[Cron] 🔄 Goroutine 开始执行任务: %s", jobCopy.Name)
			c.executeJob(&jobCopy)
			log.Printf("[Cron] 🔄 Goroutine 完成执行任务: %s", jobCopy.Name)
		}()

		// 处理任务后续：删除或重新调度
		// 【关键修复】确保 "at" 类型任务只执行一次，即使 DeleteAfterRun 标志错误
		if item.job.DeleteAfterRun || item.job.Schedule.Kind == "at" {
			// 一次性任务（包括 "at" 类型），从 map 中删除
			// 注意：由于已经在锁内，直接删除，不需要调用 RemoveJob（RemoveJob 会尝试获取锁）
			delete(c.jobs, item.job.ID)
			log.Printf("[Cron] ✓ 一次性任务已从 map 删除: %s", item.job.Name)
		} else {
			// 周期性任务，重新计算下次执行时间
			item.job.NextRun = c.calculateNextRun(item.job.Schedule)
			// 重新插入堆中（O(log n)）
			heap.Push(c.heap, item)
			log.Printf("[Cron] ✓ 周期性任务已重新调度: %s, 下次: %v", item.job.Name, item.job.NextRun)
		}
	}
}
