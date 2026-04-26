// loop.go - 主要的 Agent 循环处理器
//
// 这个文件包含 AgentLoop，它是 nanobot 的核心组件，负责：
// 1. 从消息总线接收用户消息
// 2. 通过 LLM 提供商处理消息（支持工具调用）
// 3. 执行工具调用并收集结果
// 4. 管理会话历史和记忆
// 5. 返回最终响应给用户
//
// 消息处理流程：
// 接收消息 -> 构建上下文 -> LLM 推理 -> 工具调用（循环）-> 生成最终回复 -> 保存会话
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Ailoc/nanogrip/internal/bus"
	"github.com/Ailoc/nanogrip/internal/providers"
	"github.com/Ailoc/nanogrip/internal/session"
	"github.com/Ailoc/nanogrip/internal/tools"
)

// AgentLoop 是主要的 Agent 循环处理器
// 它协调所有核心组件来处理用户消息并生成响应
type AgentLoop struct {
	provider        providers.LLMProvider   // LLM 提供商（OpenAI、Anthropic 等）
	tools           *tools.ToolRegistry     // 工具注册表，包含所有可用的工具
	bus             *bus.MessageBus         // 消息总线，用于接收和发送消息
	sessions        *session.SessionManager // 会话管理器，管理用户会话历史
	contextBuilder  *ContextBuilder         // 上下文构建器，用于构建系统提示词
	memoryStore     *MemoryStore            // 记忆存储，用于长期记忆和历史
	workspace       string                  // 工作空间路径
	model           string                  // LLM 模型名称（如 gpt-4、claude-3-5-sonnet）
	maxTokens       int                     // 最大令牌数
	temperature     float64                 // 温度参数（控制随机性）
	maxIterations   int                     // 最大迭代次数（防止无限循环）
	memoryWindow    int                     // 记忆窗口大小（保留多少条历史消息）
	running         bool                    // 循环是否正在运行
	runningMu       sync.RWMutex            // running 字段的读写锁
	consolidating   map[string]bool         // 正在整理记忆的会话
	consolidatingMu sync.Mutex              // 整理记忆的互斥锁
	messageChan     chan string             // 消息通道（用于工具发送消息）
	toolContextMu   sync.RWMutex            // 保护当前工具上下文
	currentChannel  string                  // 当前处理的通道
	currentChatID   string                  // 当前处理的聊天 ID
	wg              sync.WaitGroup          // 等待所有goroutine结束
	cancelFunc      context.CancelFunc      // 用于取消所有子goroutine
	ctx             context.Context         // 上下文，用于取消操作
	subagents       *SubagentManager        // 子代理管理器
}

// NewAgentLoop 创建一个新的 Agent 循环处理器
// 参数：
//   - provider: LLM 提供商（OpenAI、Anthropic 等）
//   - tools: 工具注册表
//   - bus: 消息总线
//   - sessions: 会话管理器
//   - workspace: 工作空间路径
//   - model: LLM 模型名称
//   - maxTokens: 最大令牌数
//   - temperature: 温度参数
//   - maxIterations: 最大迭代次数
//   - memoryWindow: 记忆窗口大小
func NewAgentLoop(
	provider providers.LLMProvider,
	toolRegistry *tools.ToolRegistry,
	bus *bus.MessageBus,
	sessions *session.SessionManager,
	workspace string,
	model string,
	maxTokens int,
	temperature float64,
	maxIterations int,
	memoryWindow int,
) *AgentLoop {
	// 获取内置技能路径
	// 尝试多个可能的位置
	var builtinSkills string

	// 方案1: 相对于工作区 (/home/minimax/.nanogrip/workspace/../skills = /home/minimax/.nanogrip/skills)
	builtinSkills = filepath.Join(workspace, "..", "skills")
	if _, err := os.Stat(builtinSkills); os.IsNotExist(err) {
		// 方案2: 尝试绝对路径 /workspace/nanogrip/skills
		builtinSkills = "/workspace/nanogrip/skills"
		if _, err := os.Stat(builtinSkills); os.IsNotExist(err) {
			// 方案3: 尝试当前工作目录下的 skills
			builtinSkills = "skills"
		}
	}

	// 创建记忆存储
	memoryStore := NewMemoryStore(workspace)

	// 注册保存记忆工具
	saveMemoryTool := tools.NewSaveMemoryTool(memoryStore)
	toolRegistry.Register(saveMemoryTool)

	// 注册待办事项工具（支持多项目/多任务）
	todoTool := tools.NewTodoTool(workspace)
	toolRegistry.Register(todoTool)

	loop := &AgentLoop{
		provider:       provider,
		tools:          toolRegistry,
		bus:            bus,
		sessions:       sessions,
		contextBuilder: NewContextBuilder(workspace, builtinSkills),
		memoryStore:    memoryStore,
		workspace:      workspace,
		model:          model,
		maxTokens:      maxTokens,
		temperature:    temperature,
		maxIterations:  maxIterations,
		memoryWindow:   memoryWindow,
		consolidating:  make(map[string]bool),
		messageChan:    make(chan string, 100),
	}

	// 设置上下文构建器的记忆上下文
	loop.contextBuilder.SetMemoryStore(memoryStore)

	return loop
}

// Start 启动 Agent 循环处理器
// 它会启动一个后台 goroutine 来持续处理来自消息总线的消息
func (a *AgentLoop) Start(ctx context.Context) error {
	a.runningMu.Lock()
	if a.running {
		a.runningMu.Unlock()
		return fmt.Errorf("agent loop is already running")
	}
	a.running = true
	a.runningMu.Unlock()

	// 创建可取消的上下文
	agentCtx, cancel := context.WithCancel(ctx)
	a.cancelFunc = cancel
	a.ctx = agentCtx

	// 启动消息处理器goroutine并注册到WaitGroup
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.processMessages(agentCtx)
	}()

	log.Println("Agent loop started")
	return nil
}

// Stop 停止 Agent 循环处理器
// 这会取消所有子goroutine并等待它们完成
func (a *AgentLoop) Stop() {
	a.runningMu.Lock()
	if !a.running {
		a.runningMu.Unlock()
		return
	}
	a.running = false
	a.runningMu.Unlock()

	// 取消上下文，通知所有goroutine退出
	if a.cancelFunc != nil {
		a.cancelFunc()
	}

	log.Println("Agent loop stopping, waiting for goroutines...")

	// 等待所有goroutine完成
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All agent goroutines stopped")
	case <-time.After(10 * time.Second):
		log.Println("Warning: Timeout waiting for agent goroutines to stop")
	}
}

// processMessages 处理传入的消息
// 这是一个持续运行的循环，不断从消息总线消费消息并处理
func (a *AgentLoop) processMessages(ctx context.Context) {
	defer log.Println("processMessages goroutine exiting")

	for {
		a.runningMu.RLock()
		running := a.running
		a.runningMu.RUnlock()

		if !running {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
			// 从消息总线消费入站消息
			// 【调试日志】显示正在等待消息
			// log.Printf("[Agent] 等待消息...")
			msg, err := a.bus.ConsumeInbound(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}

			// 【调试日志】显示收到消息
			log.Printf("[Agent] 收到消息: Channel=%s, ChatID=%s, Content=%s", msg.Channel, msg.ChatID, msg.Content)

			// 处理单个消息
			response, err := a.processMessage(ctx, msg)
			if err != nil {
				log.Printf("Error processing message: %v", err)
				response = &bus.OutboundMessage{
					Channel:  msg.Channel,
					ChatID:   msg.ChatID,
					Content:  fmt.Sprintf("Error: %v", err),
					Metadata: msg.Metadata,
				}
			}

			// 发布出站响应
			if response != nil && response.Content != "" {
				a.bus.PublishOutbound(*response)
			}
		}
	}
}

// processMessage 处理单个入站消息
// 这是核心的消息处理逻辑，包括：
// 1. 获取或创建会话
// 2. 更新工具上下文（message、spawn 工具需要知道当前 channel 和 chat_id）
// 3. 处理命令（/new, /help）
// 4. 构建消息上下文
// 5. 运行 Agent 循环进行推理
// 6. 保存会话历史
func (a *AgentLoop) processMessage(ctx context.Context, msg bus.InboundMessage) (*bus.OutboundMessage, error) {
	return a.processMessageWithStream(ctx, msg, nil)
}

func (a *AgentLoop) processMessageWithStream(ctx context.Context, msg bus.InboundMessage, onDelta providers.StreamCallback) (*bus.OutboundMessage, error) {
	// 处理系统消息（子代理公告）
	// chat_id 包含原始的 "channel:chat_id" 用于路由回复
	if msg.Channel == "system" {
		return a.processSystemMessage(ctx, msg)
	}

	// 构建会话键
	key := msg.SessionKey
	if key == "" {
		key = fmt.Sprintf("%s:%s", msg.Channel, msg.ChatID)
	}

	// 获取或创建会话
	sess := a.sessions.GetOrCreate(key)

	// 设置工具上下文（通道、聊天 ID 和交互处理器）
	a.SetToolContext(msg.Channel, msg.ChatID)
	ctx = tools.WithToolContext(ctx, msg.Channel, msg.ChatID)

	// 处理 /new 命令 - 开始新会话
	if msg.Content == "/new" {
		// 创建一个全新的会话，而不是仅仅清空消息
		// 这样可以确保完全重置会话状态
		newSession := session.NewSession(key)
		newSession.CreatedAt = time.Now()
		newSession.UpdatedAt = time.Now()
		a.sessions.Save(newSession)
		a.sessions.Invalidate(key)
		return &bus.OutboundMessage{
			Channel: msg.Channel,
			ChatID:  msg.ChatID,
			Content: "新会话已创建",
		}, nil
	}

	// 处理 /help 命令 - 显示帮助信息
	if msg.Content == "/help" {
		return &bus.OutboundMessage{
			Channel: msg.Channel,
			ChatID:  msg.ChatID,
			Content: "🐈 nanobot commands:\n/new — Start a new conversation\n/help — Show available commands",
		}, nil
	}

	// 构建消息数组（包括系统提示词、历史消息、当前消息）
	messages := a.contextBuilder.BuildMessages(
		sess.GetHistory(a.memoryWindow),
		msg.Content,
		msg.Channel,
		msg.ChatID,
		msg.Media,
	)

	// 运行 Agent 循环进行推理和工具调用
	finalContent, err := a.runAgentLoopWithStream(ctx, messages, onDelta)
	if err != nil {
		return nil, err
	}

	if finalContent == "" {
		finalContent = "I've completed processing but have no response to give."
	}

	// 保存用户消息和助手响应到会话历史
	sess.AddMessage("user", msg.Content, nil)
	sess.AddMessage("assistant", finalContent, nil)
	a.sessions.Save(sess)

	// 检查是否需要记忆整理
	a.ConsolidateIfNeeded(key, sess)

	return &bus.OutboundMessage{
		Channel:  msg.Channel,
		ChatID:   msg.ChatID,
		Content:  finalContent,
		Metadata: msg.Metadata,
	}, nil
}

// runAgentLoop 运行 Agent 迭代循环
// 这是 Agent 的核心推理循环，它会：
// 1. 调用 LLM 获取响应（可能包含工具调用）
// 2. 如果有工具调用，执行工具并将结果添加到消息历史
// 3. 继续下一轮推理，直到 LLM 给出最终文本响应
// 4. 防止无限循环（通过 maxIterations 限制）
//
// 流程：
// LLM 推理 -> 工具调用? -> 是：执行工具 -> 继续推理
//
//	-> 否：返回最终响应
func (a *AgentLoop) runAgentLoop(ctx context.Context, messages []map[string]interface{}) (string, error) {
	return a.runAgentLoopWithStream(ctx, messages, nil)
}

func (a *AgentLoop) runAgentLoopWithStream(ctx context.Context, messages []map[string]interface{}, onDelta providers.StreamCallback) (string, error) {
	iteration := 0
	var finalContent string

	for iteration < a.maxIterations {
		iteration++

		// 将消息转换为提供商格式
		providerMessages := make([]providers.Message, len(messages))
		for i, m := range messages {
			role, _ := m["role"].(string)
			content, _ := m["content"].(string)
			providerMessages[i] = providers.Message{
				Role:    role,
				Content: content,
			}

			// 处理图片（用于视觉模型）
			if imgs, ok := m["images"].([]string); ok && len(imgs) > 0 {
				providerMessages[i].Images = imgs
			}

			// 处理工具响应消息 - 必须包含 tool_call_id
			if role == "tool" {
				if toolCallID, ok := m["tool_call_id"].(string); ok && toolCallID != "" {
					providerMessages[i].ToolCallID = toolCallID
				}
				if name, ok := m["name"].(string); ok && name != "" {
					providerMessages[i].Name = name
				}
			}

			// 添加工具调用（如果存在）
			if tc, ok := m["tool_calls"].([]interface{}); ok {
				for _, tcRaw := range tc {
					tcMap, ok := tcRaw.(map[string]interface{})
					if !ok {
						continue
					}
					funcMap, ok := tcMap["function"].(map[string]interface{})
					if !ok {
						continue
					}
					name, _ := funcMap["name"].(string)
					args, _ := funcMap["arguments"].(string)

					argsMap := make(map[string]interface{})
					_ = json.Unmarshal([]byte(args), &argsMap)
					argsMap["_raw"] = args

					providerMessages[i].Tools = append(providerMessages[i].Tools, providers.ToolCallRequest{
						ID:        tcMap["id"].(string),
						Name:      name,
						Arguments: argsMap,
					})
				}
			}
		}

		// 获取工具定义
		toolDefs := make([]providers.ToolDef, 0)
		for _, t := range a.tools.GetDefinitions() {
			if fn, ok := t["function"].(map[string]interface{}); ok {
				toolDefs = append(toolDefs, providers.ToolDef{
					Type: "function",
					Function: providers.FunctionDef{
						Name:        fn["name"].(string),
						Description: fn["description"].(string),
						Parameters:  fn["parameters"].(map[string]interface{}),
					},
				})
			}
		}
		// 调用 LLM 提供商获取响应
		resp, err := a.chat(ctx, providerMessages, toolDefs, onDelta)
		if err != nil {
			return "", err
		}

		// 检查是否有工具调用
		if resp.HasToolCalls() {
			// 构建工具调用字典 - 与 nanobot 一致
			toolCallDicts := make([]map[string]interface{}, len(resp.ToolCalls))
			for i, tc := range resp.ToolCalls {
				argsStr := providers.ToolArgumentsJSON(tc.Arguments)
				toolCallDicts[i] = map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]string{
						"name":      tc.Name,
						"arguments": argsStr,
					},
				}
			}

			// 【关键修复】与 nanobot 一致：先添加工具调用的助手消息，再执行工具
			// 这确保 LLM 知道它自己调用了哪些工具
			messages = append(messages, map[string]interface{}{
				"role":       "assistant",
				"content":    resp.Content,
				"tool_calls": toolCallDicts,
			})

			// 执行工具调用
			for _, tc := range resp.ToolCalls {
				result := a.tools.Execute(ctx, tc.Name, tc.Arguments)

				// 添加完整的工具结果消息给 LLM
				messages = append(messages, map[string]interface{}{
					"role":         "tool",
					"tool_call_id": tc.ID,
					"name":         tc.Name,
					"content":      result, // 使用完整的 result
				})
			}

			// 【关键修复】不要在这里 break！
			// Plan-Execute 模式需要 LLM 能够连续执行多个工具调用
			// 例如：创建 todo -> 添加 todo -> 执行 -> 更新状态 -> 发送消息
			// 只有当 LLM 没有更多工具调用时，才在下面的 else 分支返回响应
			// 继续循环，让 LLM 可以继续执行更多工具调用
		} else {
			// 没有工具调用，这是最终响应
			finalContent = resp.Content
			break
		}
	}

	return finalContent, nil
}

func (a *AgentLoop) chat(ctx context.Context, messages []providers.Message, toolDefs []providers.ToolDef, onDelta providers.StreamCallback) (*providers.LLMResponse, error) {
	if onDelta != nil {
		if streamingProvider, ok := a.provider.(providers.StreamingLLMProvider); ok {
			emitted := false
			wrappedDelta := func(delta string) {
				if delta == "" {
					return
				}
				emitted = true
				onDelta(delta)
			}

			resp, err := streamingProvider.ChatStream(ctx, messages, toolDefs, a.model, a.maxTokens, a.temperature, wrappedDelta)
			if err == nil {
				return resp, nil
			}
			if emitted {
				return nil, err
			}

			log.Printf("Streaming chat failed before output, falling back to non-streaming chat: %v", err)
		}
	}

	return a.provider.Chat(ctx, messages, toolDefs, a.model, a.maxTokens, a.temperature)
}

// ProcessDirect 直接处理消息（用于 CLI 或 Cron）
// 这个方法用于命令行界面或 cron 任务，不通过消息总线
// 使用当前设置的 currentChannel 和 currentChatID 作为目标
func (a *AgentLoop) ProcessDirect(ctx context.Context, content string) (string, error) {
	return a.ProcessDirectStream(ctx, content, nil)
}

// ProcessDirectStream directly handles a message and streams text deltas when supported.
func (a *AgentLoop) ProcessDirectStream(ctx context.Context, content string, onDelta providers.StreamCallback) (string, error) {
	// 使用当前设置的 Channel 和 ChatID（由 SetToolContext 设置）
	// 如果未设置（如 CLI 模式），使用默认值
	a.toolContextMu.RLock()
	channel := a.currentChannel
	chatID := a.currentChatID
	a.toolContextMu.RUnlock()

	if channel == "" {
		channel = "cli"
	}
	if chatID == "" {
		chatID = "direct"
	}

	return a.ProcessDirectWithContextStream(ctx, channel, chatID, content, onDelta)
}

// ProcessDirectWithContext directly processes a message for a specific chat target.
func (a *AgentLoop) ProcessDirectWithContext(ctx context.Context, channel, chatID, content string) (string, error) {
	return a.ProcessDirectWithContextStream(ctx, channel, chatID, content, nil)
}

// ProcessDirectWithContextStream directly processes a message for a specific chat target and streams text deltas when supported.
func (a *AgentLoop) ProcessDirectWithContextStream(ctx context.Context, channel, chatID, content string, onDelta providers.StreamCallback) (string, error) {
	if channel == "" {
		channel = "cli"
	}
	if chatID == "" {
		chatID = "direct"
	}

	ctx = tools.WithToolContext(ctx, channel, chatID)

	msg := bus.InboundMessage{
		Message: bus.Message{
			Channel:  channel,
			SenderID: "user",
			ChatID:   chatID,
			Content:  content,
		},
	}

	response, err := a.processMessageWithStream(ctx, msg, onDelta)
	if err != nil {
		log.Printf("[AgentLoop] processMessage 返回错误: %v", err)
		return "", err
	}

	if response == nil {
		log.Printf("[AgentLoop] processMessage 返回 nil 响应")
		return "", fmt.Errorf("no response")
	}

	return response.Content, nil
}

// SetMessageChan 设置消息通道（用于消息工具）
// 这允许工具通过通道发送消息给用户
func (a *AgentLoop) SetMessageChan(ch chan string) {
	a.messageChan = ch
}

// SetToolContext 设置工具上下文（通道和聊天 ID）
// 在处理消息前调用，以确保工具知道当前上下文
func (a *AgentLoop) SetToolContext(channel, chatID string) {
	a.toolContextMu.Lock()
	a.currentChannel = channel
	a.currentChatID = chatID
	a.toolContextMu.Unlock()

	// 更新 message 工具的上下文
	if msgTool := a.tools.Get("message"); msgTool != nil {
		if mt, ok := msgTool.(*tools.MessageTool); ok {
			mt.SetContext(channel, chatID)
		}
	}

	// 更新 spawn 工具的上下文
	if spawnTool := a.tools.Get("spawn"); spawnTool != nil {
		if st, ok := spawnTool.(*tools.SpawnTool); ok {
			st.SetContext(channel, chatID)
		}
	}

	// 更新 cron 工具的上下文（用于定时任务）
	if cronTool := a.tools.Get("cron"); cronTool != nil {
		if ct, ok := cronTool.(*tools.CronTool); ok {
			ct.SetContext(channel, chatID)
		}
	}
}

// ConsolidateIfNeeded 检查并执行记忆整理（如果需要）
// 当会话消息数量超过记忆窗口时，触发记忆整理
func (a *AgentLoop) ConsolidateIfNeeded(sessionKey string, sess *session.Session) {
	// 检查是否需要整理
	keepCount := a.memoryWindow / 2
	if keepCount <= 0 {
		keepCount = 10 // 默认保留10条消息
	}

	// 获取当前总消息数
	msgCount := len(sess.Messages)
	if msgCount <= keepCount {
		return
	}

	// 【修复】计算从 LastConsolidated 到当前最新消息的新增数量
	newMessagesSinceLastConsolidate := msgCount - sess.LastConsolidated

	// 如果新增消息数达到 keepCount，触发整理
	if newMessagesSinceLastConsolidate < keepCount {
		log.Printf("[Memory] 新消息数 %d < %d，暂不整理",
			newMessagesSinceLastConsolidate, keepCount)
		return
	}

	log.Printf("[Memory] 新消息数 %d >= %d，触发整理 (LastConsolidated=%d, 总消息=%d)",
		newMessagesSinceLastConsolidate, keepCount, sess.LastConsolidated, msgCount)

	// 检查是否已经在整理
	a.consolidatingMu.Lock()
	if a.consolidating[sessionKey] {
		a.consolidatingMu.Unlock()
		return
	}
	a.consolidating[sessionKey] = true
	a.consolidatingMu.Unlock()

	// 在后台进行整理
	go func() {
		defer func() {
			a.consolidatingMu.Lock()
			delete(a.consolidating, sessionKey)
			a.consolidatingMu.Unlock()
		}()

		a.consolidateMemory(sessionKey, sess, keepCount)
	}()
}

// consolidateMemory 执行记忆整理
// 将旧消息通过 LLM 提炼并保存到 MEMORY.md 和 HISTORY.md
func (a *AgentLoop) consolidateMemory(sessionKey string, sess *session.Session, keepCount int) {
	log.Printf("[Memory] 开始记忆整理: %s", sessionKey)

	// 创建带有更长超时的上下文（记忆整理可能需要更长时间）
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 【修复】整理区间：从 LastConsolidated 到 LastConsolidated + keepCount
	startConsolidate := sess.LastConsolidated
	endConsolidate := startConsolidate + keepCount

	// 确保不超过消息总数
	totalMessages := len(sess.Messages)
	if endConsolidate > totalMessages {
		endConsolidate = totalMessages
	}

	// 没有需要整理的消息
	if startConsolidate >= endConsolidate {
		log.Printf("[Memory] 没有需要整理的消息 (start=%d, end=%d)",
			startConsolidate, endConsolidate)
		return
	}

	// 只整理 [LastConsolidated, LastConsolidated + keepCount) 区间的消息
	oldMessages := sess.Messages[startConsolidate:endConsolidate]
	if len(oldMessages) == 0 {
		return
	}

	log.Printf("[Memory] 整理消息区间 [%d:%d] (%d 条消息) → MEMORY.md",
		startConsolidate, endConsolidate, len(oldMessages))

	// 构建对话文本
	var lines []string
	for _, msg := range oldMessages {
		if msg.Content == "" {
			continue
		}
		timestamp := msg.Timestamp
		if len(timestamp) > 16 {
			timestamp = timestamp[:16]
		}
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", timestamp, msg.Role, msg.Content))
	}

	// 读取当前长期记忆
	currentMemory := a.memoryStore.ReadLongTerm()
	if currentMemory == "" {
		currentMemory = "(empty)"
	}

	// 构建提示词，要求 LLM 整理记忆
	conversationText := strings.Join(lines, "\n")
	prompt := fmt.Sprintf(`Process this conversation and call the save_memory tool with your consolidation.

## Current Long-term Memory
%s

## Conversation to Process
%s

Respond by calling the save_memory tool with:
1. history_entry: A paragraph summarizing key events/decisions (start with [YYYY-MM-DD HH:MM])
2. memory_update: Updated long-term memory (include existing facts plus new ones, or unchanged if nothing new)`, currentMemory, conversationText)

	// 调用 LLM 进行整理（使用较长超时）
	ctx, cancel = context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 构建消息
	messages := []providers.Message{
		{Role: "system", Content: "You are a memory consolidation agent. Call the save_memory tool with your consolidation of the conversation."},
		{Role: "user", Content: prompt},
	}

	// 获取工具定义
	toolDefs := []providers.ToolDef{
		{
			Type: "function",
			Function: providers.FunctionDef{
				Name:        "save_memory",
				Description: "Save the memory consolidation result to persistent storage.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"history_entry": map[string]interface{}{
							"type":        "string",
							"description": "A paragraph (2-5 sentences) summarizing key events/decisions/topics. Start with [YYYY-MM-DD HH:MM]. Include detail useful for grep search.",
						},
						"memory_update": map[string]interface{}{
							"type":        "string",
							"description": "Full updated long-term memory as markdown. Include all existing facts plus new ones. Return unchanged if nothing new.",
						},
					},
					"required": []string{"history_entry", "memory_update"},
				},
			},
		},
	}

	// 调用 LLM
	resp, err := a.provider.Chat(ctx, messages, toolDefs, a.model, 4096, 0.7)
	if err != nil {
		log.Printf("Memory consolidation failed: %v", err)
		return
	}

	// 检查是否有工具调用
	if resp.HasToolCalls() {
		// 执行 save_memory 工具
		for _, tc := range resp.ToolCalls {
			if tc.Name == "save_memory" {
				result := a.tools.Execute(ctx, tc.Name, tc.Arguments)
				log.Printf("Memory consolidation result: %s", result)
			}
		}
	} else {
		log.Printf("Memory consolidation: LLM did not call save_memory, skipping")
	}

	// 【修复】更新 LastConsolidated 到本次整理的结束位置
	sess.LastConsolidated = endConsolidate
	a.sessions.Save(sess)
	log.Printf("[Memory] 整理完成: LastConsolidated=%d -> %d",
		startConsolidate, sess.LastConsolidated)
}

// processSystemMessage 处理系统消息（例如子代理公告）
//
// chat_id 字段包含 "original_channel:original_chat_id" 用于将响应
// 路由到正确的目的地
func (a *AgentLoop) processSystemMessage(ctx context.Context, msg bus.InboundMessage) (*bus.OutboundMessage, error) {
	log.Printf("Processing system message from %s", msg.SenderID)

	// 从 chat_id 解析来源（格式："channel:chat_id"）
	originChannel := "cli"
	originChatID := msg.ChatID
	if idx := strings.Index(msg.ChatID, ":"); idx != -1 {
		originChannel = msg.ChatID[:idx]
		originChatID = msg.ChatID[idx+1:]
	}

	// 使用来源会话获取上下文
	sessionKey := fmt.Sprintf("%s:%s", originChannel, originChatID)
	sess := a.sessions.GetOrCreate(sessionKey)
	ctx = tools.WithToolContext(ctx, originChannel, originChatID)

	// 更新工具上下文
	if msgTool := a.tools.Get("message"); msgTool != nil {
		if mt, ok := msgTool.(*tools.MessageTool); ok {
			mt.SetContext(originChannel, originChatID)
		}
	}
	if spawnTool := a.tools.Get("spawn"); spawnTool != nil {
		if st, ok := spawnTool.(*tools.SpawnTool); ok {
			st.SetContext(originChannel, originChatID)
		}
	}

	// 构建消息（使用公告内容）
	messages := a.contextBuilder.BuildMessages(
		sess.GetHistory(a.memoryWindow),
		msg.Content,
		originChannel,
		originChatID,
		nil,
	)

	// Agent 循环（限制迭代次数用于公告处理）
	iteration := 0
	finalContent := ""

	for iteration < a.maxIterations {
		iteration++

		// 转换消息格式
		providerMessages := make([]providers.Message, len(messages))
		for i, m := range messages {
			role, _ := m["role"].(string)
			content, _ := m["content"].(string)
			providerMessages[i] = providers.Message{
				Role:    role,
				Content: content,
			}

			// 处理图片（用于视觉模型）
			if imgs, ok := m["images"].([]string); ok && len(imgs) > 0 {
				providerMessages[i].Images = imgs
			}

			// 处理工具响应消息 - 必须包含 tool_call_id
			if role == "tool" {
				if toolCallID, ok := m["tool_call_id"].(string); ok && toolCallID != "" {
					providerMessages[i].ToolCallID = toolCallID
				}
				if name, ok := m["name"].(string); ok && name != "" {
					providerMessages[i].Name = name
				}
			}
		}

		// 获取工具定义
		toolDefs := make([]providers.ToolDef, 0)
		for _, t := range a.tools.GetDefinitions() {
			if fn, ok := t["function"].(map[string]interface{}); ok {
				toolDefs = append(toolDefs, providers.ToolDef{
					Type: "function",
					Function: providers.FunctionDef{
						Name:        fn["name"].(string),
						Description: fn["description"].(string),
						Parameters:  fn["parameters"].(map[string]interface{}),
					},
				})
			}
		}
		// 调用 LLM
		resp, err := a.provider.Chat(ctx, providerMessages, toolDefs, a.model, a.maxTokens, a.temperature)
		if err != nil {
			return nil, err
		}

		if resp.HasToolCalls() {
			// 构建工具调用字典
			toolCallDicts := make([]map[string]interface{}, len(resp.ToolCalls))
			for i, tc := range resp.ToolCalls {
				argsStr := providers.ToolArgumentsJSON(tc.Arguments)
				toolCallDicts[i] = map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]string{
						"name":      tc.Name,
						"arguments": argsStr,
					},
				}
			}

			// 先添加工具调用的助手消息
			messages = append(messages, map[string]interface{}{
				"role":       "assistant",
				"content":    resp.Content,
				"tool_calls": toolCallDicts,
			})

			// 执行工具
			for _, tc := range resp.ToolCalls {
				result := a.tools.Execute(ctx, tc.Name, tc.Arguments)
				messages = append(messages, map[string]interface{}{
					"role":         "tool",
					"tool_call_id": tc.ID,
					"name":         tc.Name,
					"content":      result,
				})
			}
		} else {
			finalContent = resp.Content
			break
		}
	}

	if finalContent == "" {
		finalContent = "Background task completed."
	}

	// 保存到会话（在历史中标记为系统消息）
	sess.AddMessage("user", fmt.Sprintf("[System: %s] %s", msg.SenderID, msg.Content), nil)
	sess.AddMessage("assistant", finalContent, nil)
	a.sessions.Save(sess)

	return &bus.OutboundMessage{
		Channel: originChannel,
		ChatID:  originChatID,
		Content: finalContent,
	}, nil
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
