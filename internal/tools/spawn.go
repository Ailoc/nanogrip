package tools

import (
	"context"
)

// spawn.go - 子代理生成工具
// 此文件实现了生成子代理的工具，允许主代理创建后台运行的子代理来处理长时间任务

// SpawnTool 允许代理生成子代理来执行后台任务
// 用于将长时间运行的任务委派给独立的子代理，避免阻塞主代理
type SpawnTool struct {
	BaseTool
	// spawnFunc 是实际的子代理生成函数
	// 参数: task（任务描述）, label（可读标签）, originChannel（来源频道）, originChatID（来源聊天ID）
	// 返回: 生成结果的描述字符串
	spawnFunc func(task string, label string, originChannel string, originChatID string) string
	channel   string // 当前上下文频道
	chatID    string // 当前上下文聊天ID
}

// NewSpawnTool 创建一个新的子代理生成工具
// 参数:
//
//	spawnFunc: 实际执行子代理生成的函数，由上层提供
//
// 返回:
//
//	配置好的SpawnTool实例
func NewSpawnTool(spawnFunc func(task string, label string, originChannel string, originChatID string) string) *SpawnTool {
	return &SpawnTool{
		BaseTool: NewBaseTool(
			"spawn",
			"Spawn a subagent to run a task in the background. The subagent runs independently and will notify you when complete.\n\n**WHEN TO USE SPAWN:**\n• Tasks taking >2 minutes (large file processing, web scraping, batch operations)\n• Parallel independent tasks (multiple searches, concurrent file operations)\n• Long-running monitoring or polling tasks\n• Tasks where you want to continue working while it completes\n\n**WHEN NOT TO USE:**\n• Quick queries (<30 seconds) - just do them directly\n• Simple file reads/writes - use read/write tools directly\n• Tasks that depend on each other - run sequentially instead\n\n**USAGE:**\n{\"task\": \"your specific task description\", \"label\": \"optional readable name\"}",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"task": map[string]interface{}{
						"type":        "string",
						"description": "The task description for the subagent",
					},
					"label": map[string]interface{}{
						"type":        "string",
						"description": "Optional human-readable label for the task",
					},
				},
				"required": []string{"task"},
			},
		),
		spawnFunc: spawnFunc,
	}
}

// SetContext 设置当前上下文（频道和聊天ID）
// 这允许工具记住当前处理的会话，以便子代理完成时通知正确的位置
// 参数:
//
//	channel: 当前频道
//	chatID: 当前聊天ID
func (t *SpawnTool) SetContext(channel string, chatID string) {
	t.channel = channel
	t.chatID = chatID
}

// Execute 执行子代理生成
// 调用spawnFunc创建一个新的子代理来处理指定任务
// 使用 SetContext 设置的上下文作为默认来源
// 参数:
//
//	ctx: 上下文对象
//	params: 参数map，必须包含"task"，可选"label"
//
// 返回:
//
//	子代理生成结果的描述字符串
func (t *SpawnTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// 获取任务描述
	task, _ := params["task"].(string)
	// 获取可选的标签
	label, _ := params["label"].(string)

	// 验证任务参数
	if task == "" {
		return "Error: task is required", nil
	}

	// 使用上下文默认值（通过 SetContext 设置）
	originChannel := t.channel
	originChatID := t.chatID

	// 如果上下文为空，使用默认值
	if originChannel == "" {
		originChannel = "cli"
	}
	if originChatID == "" {
		originChatID = "direct"
	}

	// 如果提供了生成函数，则执行
	if t.spawnFunc != nil {
		return t.spawnFunc(task, label, originChannel, originChatID), nil
	}

	return "Spawn service not available", nil
}
