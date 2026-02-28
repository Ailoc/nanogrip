// subagent.go - 后台子代理管理器
//
// 这个文件实现了后台子代理（Subagent）系统，允许主 Agent 将任务委派给后台运行的子代理。
//
// 子代理的特点：
// 1. 独立执行 - 在后台 goroutine 中运行，不阻塞主 Agent
// 2. 专注单一任务 - 每个子代理只负责完成被分配的特定任务
// 3. 简化上下文 - 子代理有自己的系统提示词，不访问主会话历史
// 4. 结果通知 - 完成后通过消息总线发送结果给主 Agent
//
// 使用场景：
// - 长时间运行的任务（如大型文件处理、网页爬取）
// - 并行任务（同时进行多个独立的子任务）
// - 后台监控（持续监控某些条件）
//
// 限制：
// - 子代理不能发送消息给用户（没有 message 工具）
// - 子代理不能再创建其他子代理
// - 子代理不能访问主 Agent 的会话历史
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Ailoc/nanogrip/internal/bus"
	"github.com/Ailoc/nanogrip/internal/providers"
	"github.com/Ailoc/nanogrip/internal/tools"
)

// SubagentManager 管理后台子代理的执行
// 它负责创建、跟踪和取消子代理任务
type SubagentManager struct {
	provider          providers.LLMProvider    // LLM 提供商
	workspace         string                   // 工作空间路径
	bus               *bus.MessageBus          // 消息总线（用于发送结果）
	model             string                   // LLM 模型名称
	temperature       float64                  // 温度参数
	maxTokens         int                      // 最大令牌数
	maxIterations     int                      // 最大迭代次数
	toolRegistry      *tools.ToolRegistry      // 工具注册表
	runningTasks      map[string]*subagentTask // 正在运行的任务映射
	runningTasksMutex sync.Mutex               // 任务映射的互斥锁
}

// subagentTask 表示一个子代理任务
type subagentTask struct {
	ID      string             // 任务 ID（用于跟踪和取消）
	Label   string             // 任务标签（人类可读的描述）
	Task    string             // 任务内容（完整的任务描述）
	Origin  originInfo         // 来源信息（用于发送结果）
	Context context.Context    // 上下文（用于取消）
	Cancel  context.CancelFunc // 取消函数
}

// originInfo 记录任务的来源
type originInfo struct {
	Channel string // 来源频道
	ChatID  string // 聊天 ID
}

// NewSubagentManager 创建一个新的子代理管理器
func NewSubagentManager(
	provider providers.LLMProvider,
	workspace string,
	bus *bus.MessageBus,
	model string,
	temperature float64,
	maxTokens int,
	maxIterations int,
	toolRegistry *tools.ToolRegistry,
) *SubagentManager {
	return &SubagentManager{
		provider:      provider,
		workspace:     workspace,
		bus:           bus,
		model:         model,
		temperature:   temperature,
		maxTokens:     maxTokens,
		maxIterations: maxIterations,
		toolRegistry:  toolRegistry,
		runningTasks:  make(map[string]*subagentTask),
	}
}

// Spawn 创建一个子代理在后台执行任务
// 这个方法会：
// 1. 生成唯一的任务 ID
// 2. 创建子代理任务记录
// 3. 在新的 goroutine 中启动子代理
// 4. 立即返回确认消息（不等待任务完成）
//
// 参数：
//   - task: 任务描述（告诉子代理要做什么）
//   - label: 任务标签（可选，用于显示）
//   - originChannel: 来源频道
//   - originChatID: 来源聊天 ID
//
// 返回：
//   - 确认消息，包含任务 ID
func (s *SubagentManager) Spawn(
	task string,
	label string,
	originChannel string,
	originChatID string,
) string {
	taskID := generateTaskID()
	displayLabel := task
	if len(displayLabel) > 30 {
		displayLabel = displayLabel[:30] + "..."
	}
	if label != "" {
		displayLabel = label
	}

	ctx, cancel := context.WithCancel(context.Background())

	subtask := &subagentTask{
		ID:      taskID,
		Label:   displayLabel,
		Task:    task,
		Origin:  originInfo{Channel: originChannel, ChatID: originChatID},
		Context: ctx,
		Cancel:  cancel,
	}

	s.runningTasksMutex.Lock()
	s.runningTasks[taskID] = subtask
	s.runningTasksMutex.Unlock()

	// 在后台运行子代理
	go s.runSubagent(ctx, taskID, displayLabel, task, originChannel, originChatID)

	log.Printf("Spawned subagent [%s]: %s", taskID, displayLabel)
	return fmt.Sprintf("Subagent [%s] started (id: %s). I'll notify you when it completes.", displayLabel, taskID)
}

// runSubagent 执行子代理任务
// 这是子代理的核心执行逻辑，它运行自己的 Agent 循环：
// 1. 构建子代理专用的系统提示词（简化、专注任务）
// 2. 运行迭代循环（LLM 推理 + 工具调用）
// 3. 处理错误或成功完成
// 4. 通过消息总线发送结果
// 5. 清理任务记录
func (s *SubagentManager) runSubagent(
	ctx context.Context,
	taskID string,
	label string,
	task string,
	originChannel string,
	originChatID string,
) {
	log.Printf("Subagent [%s] starting task: %s", taskID, label)

	// 构建子代理的消息（使用专用的系统提示词）
	systemPrompt := s.buildSubagentPrompt(task)
	messages := []map[string]interface{}{
		{"role": "system", "content": systemPrompt},
		{"role": "user", "content": task},
	}

	var finalResult string

	// 子代理的迭代循环
	for iteration := 0; iteration < s.maxIterations; iteration++ {
		// 将消息转换为提供商格式
		providerMessages := make([]providers.Message, len(messages))
		for i, m := range messages {
			role, _ := m["role"].(string)
			content, _ := m["content"].(string)
			providerMessages[i] = providers.Message{
				Role:    role,
				Content: content,
			}
		}

		// 获取工具定义
		toolDefs := make([]providers.ToolDef, 0)
		for _, t := range s.toolRegistry.GetDefinitions() {
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
		resp, err := s.provider.Chat(ctx, providerMessages, toolDefs, s.model, s.maxTokens, s.temperature)
		if err != nil {
			log.Printf("Subagent [%s] error: %v", taskID, err)
			s.announceResult(taskID, label, task, fmt.Sprintf("Error: %v", err), originChannel, originChatID, "error")
			return
		}

		// 处理工具调用
		if resp.HasToolCalls() {
			// 添加包含工具调用的助手消息
			toolCallDicts := make([]map[string]interface{}, len(resp.ToolCalls))
			for i, tc := range resp.ToolCalls {
				argsStr, _ := json.Marshal(tc.Arguments)
				toolCallDicts[i] = map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]string{
						"name":      tc.Name,
						"arguments": string(argsStr),
					},
				}
			}

			messages = append(messages, map[string]interface{}{
				"role":       "assistant",
				"content":    resp.Content,
				"tool_calls": toolCallDicts,
			})

			// 执行工具
			for _, tc := range resp.ToolCalls {
				result := s.toolRegistry.Execute(ctx, tc.Name, tc.Arguments)
				log.Printf("Subagent [%s] executed %s", taskID, tc.Name)

				messages = append(messages, map[string]interface{}{
					"role":         "tool",
					"tool_call_id": tc.ID,
					"name":         tc.Name,
					"content":      result,
				})
			}
		} else {
			// 没有工具调用，获得最终结果
			finalResult = resp.Content
			break
		}
	}

	if finalResult == "" {
		finalResult = "Task completed but no final response was generated."
	}

	log.Printf("Subagent [%s] completed successfully", taskID)
	s.announceResult(taskID, label, task, finalResult, originChannel, originChatID, "ok")

	// 清理任务记录
	s.runningTasksMutex.Lock()
	delete(s.runningTasks, taskID)
	s.runningTasksMutex.Unlock()
}

// announceResult 宣布子代理的结果
// 这个方法通过消息总线发送子代理的结果给主 Agent
// 消息会自动路由回原始的频道和聊天
func (s *SubagentManager) announceResult(
	taskID string,
	label string,
	task string,
	result string,
	originChannel string,
	originChatID string,
	status string,
) {
	statusText := "completed successfully"
	if status == "error" {
		statusText = "failed"
	}

	// 构建结果通知消息
	// 这个消息会被主 Agent 接收，然后主 Agent 会将其转化为用户友好的响应
	announceContent := fmt.Sprintf(`[Subagent '%s' %s]

Task: %s

Result:
%s

Summarize this naturally for the user. Keep it brief (1-2 sentences). Do not mention technical details like "subagent" or task IDs.`, label, statusText, task, result)

	// 通过消息总线发送结果
	msg := bus.InboundMessage{
		Message: bus.Message{
			Channel:   "system",
			SenderID:  "subagent",
			ChatID:    originChannel + ":" + originChatID,
			Content:   announceContent,
			Timestamp: time.Now(),
		},
	}

	s.bus.PublishInbound(msg)
}

// buildSubagentPrompt 构建子代理的系统提示词
// 子代理的提示词更简洁、更专注：
// - 强调完成特定任务
// - 明确限制（不能发送消息、不能创建子代理）
// - 简化的工作空间信息
func (s *SubagentManager) buildSubagentPrompt(task string) string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")
	tz := time.Now().Format("MST")

	return fmt.Sprintf(`# Subagent

## Current Time
%s (%s)

You are a subagent spawned by the main agent to complete a specific task.

## Rules
1. Stay focused - complete only the assigned task, nothing else
2. Your final response will be reported back to the main agent
3. Do not initiate conversations or take on side tasks
4. Be concise but informative in your findings

## What You Can Do
- Read and write files in the workspace
- Execute shell commands
- Search the web and fetch web pages
- Complete the task thoroughly

## What You Cannot Do
- Send messages directly to users (no message tool available)
- Spawn other subagents
- Access the main agent's conversation history

## Workspace
Your workspace is at: %s
Skills are available at: %s/skills/ (read SKILL.md files as needed)

When you have completed the task, provide a clear summary of your findings or actions.`, now, tz, s.workspace, s.workspace)
}

// GetRunningCount 返回正在运行的子代理数量
func (s *SubagentManager) GetRunningCount() int {
	s.runningTasksMutex.Lock()
	defer s.runningTasksMutex.Unlock()
	return len(s.runningTasks)
}

// CancelTask 取消正在运行的子代理
// 返回 true 表示成功取消，false 表示任务不存在
func (s *SubagentManager) CancelTask(taskID string) bool {
	s.runningTasksMutex.Lock()
	defer s.runningTasksMutex.Unlock()

	if task, ok := s.runningTasks[taskID]; ok {
		task.Cancel()
		delete(s.runningTasks, taskID)
		return true
	}
	return false
}

// StopAll 停止所有正在运行的子代理
// 这会取消所有子代理的上下文并等待它们完成
func (s *SubagentManager) StopAll() {
	s.runningTasksMutex.Lock()
	defer s.runningTasksMutex.Unlock()

	log.Printf("Stopping %d subagents...", len(s.runningTasks))

	for taskID, task := range s.runningTasks {
		task.Cancel()
		delete(s.runningTasks, taskID)
	}

	log.Println("All subagents cancelled")
}

// GetRunningTaskIDs 返回所有正在运行的任务ID列表
func (s *SubagentManager) GetRunningTaskIDs() []string {
	s.runningTasksMutex.Lock()
	defer s.runningTasksMutex.Unlock()

	ids := make([]string, 0, len(s.runningTasks))
	for id := range s.runningTasks {
		ids = append(ids, id)
	}
	return ids
}

// generateTaskID 生成一个短的任务 ID
// 使用当前时间戳的十六进制表示（取前8位）
func generateTaskID() string {
	// 简单的 ID 生成
	return fmt.Sprintf("%x", time.Now().UnixNano())[:8]
}
