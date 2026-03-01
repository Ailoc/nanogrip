package tools

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Ailoc/nanogrip/internal/cron"
)

// cron.go - 定时任务调度工具
// 此文件实现了定时任务和提醒功能，支持添加、列出和删除定时任务

// CronTool 提供定时调度功能
// 允许代理创建和管理定时任务、提醒和周期性任务
//
// 支持两种模式：
//  1. message 模式：发送固定的消息内容
//  2. agent 模式：触发 Agent 执行命令，可以调用工具、查询数据等
type CronTool struct {
	BaseTool
	cronService *cron.CronService // Cron服务实例，负责实际的任务调度
	channel     string            // 当前会话的频道名称
	chatID      string            // 当前会话的聊天ID
}

// NewCronTool 创建一个新的定时任务工具
// 参数:
//
//	cronService: Cron服务实例，用于管理定时任务
//
// 返回:
//
//	配置好的CronTool实例
func NewCronTool(cronService *cron.CronService) *CronTool {
	return &CronTool{
		BaseTool: NewBaseTool(
			"cron",
			"Schedule reminders and recurring tasks. Actions: add, list, remove.\n\nFor add action:\n- Use 'mode' to specify execution mode: 'message' (send fixed text) or 'agent' (trigger AI command execution)\n- For 'message' mode: use 'message' parameter for the text content\n- For 'agent' mode: use 'command' parameter for the AI command to execute\n- Use 'once_seconds' for one-time reminders (e.g., remind me in 2 minutes)\n- Use 'every_seconds' for recurring tasks (e.g., every 5 minutes)\n- Use 'at' for specific time (e.g., '2026-02-12T10:30:00')\n\nExamples:\n- Message mode: {\"action\":\"add\", \"mode\":\"message\", \"message\":\"Hello\", \"once_seconds\":60}\n- Agent mode: {\"action\":\"add\", \"mode\":\"agent\", \"command\":\"查询今天天气\", \"every_seconds\":3600}",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"add", "list", "remove"},
						"description": "Action to perform",
					},
					"mode": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"message", "agent"},
						"description": "Execution mode: 'message' sends fixed text, 'agent' triggers AI command execution (default: message)",
					},
					"message": map[string]interface{}{
						"type":        "string",
						"description": "Text content to send (for message mode)",
					},
					"command": map[string]interface{}{
						"type":        "string",
						"description": "AI command to execute (for agent mode). The AI will use tools to complete the task.",
					},
					"once_seconds": map[string]interface{}{
						"type":        "integer",
						"description": "Delay in seconds for ONE-TIME reminder (e.g., 120 for 'in 2 minutes'). The job will be deleted after execution.",
					},
					"every_seconds": map[string]interface{}{
						"type":        "integer",
						"description": "Interval in seconds for RECURRING tasks (e.g., 300 for 'every 5 minutes'). The job will repeat indefinitely.",
					},
					"cron_expr": map[string]interface{}{
						"type":        "string",
						"description": "Cron expression like '0 9 * * *' (for scheduled tasks)",
					},
					"at": map[string]interface{}{
						"type":        "string",
						"description": "ISO datetime for one-time execution (e.g., '2026-02-12T10:30:00')",
					},
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "Job ID (for remove)",
					},
				},
				"required": []string{"action"},
			},
		),
		cronService: cronService,
	}
}

// SetContext 设置会话上下文信息
// 用于指定任务完成后消息发送的目标频道和聊天ID
// 参数:
//
//	channel: 频道名称
//	chatID: 聊天ID
func (t *CronTool) SetContext(channel string, chatID string) {
	t.channel = channel
	t.chatID = chatID
}

// Execute 执行定时任务操作
// 根据action参数执行添加、列表或删除操作
// 参数:
//
//	ctx: 上下文对象
//	params: 参数map，必须包含"action"
//
// 返回:
//
//	操作结果的描述字符串
func (t *CronTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// 获取操作类型
	action, _ := params["action"].(string)

	// 检查服务是否可用
	if t.cronService == nil {
		return "Cron service not available", nil
	}

	// 根据操作类型执行相应功能
	switch action {
	case "add":
		return t.addJob(params)
	case "list":
		return t.listJobs()
	case "remove":
		return t.removeJob(params)
	default:
		return "Unknown action: " + action, nil
	}
}

// addJob 添加一个新的定时任务
// 支持两种执行模式：
//  1. message 模式：发送固定的消息内容（兼容旧版）
//  2. agent 模式：触发 Agent 执行命令，可以调用工具、查询数据等
//
// 支持三种调度方式：周期性（every_seconds）、一次性（once_seconds, at）、cron表达式（cron_expr）
// 参数:
//
//	params: 参数map，必须包含"action"，可选"mode"（默认message）
//
// 返回:
//
//	任务创建结果的描述字符串
func (t *CronTool) addJob(params map[string]interface{}) (string, error) {
	// 获取参数
	mode, _ := params["mode"].(string)
	message, _ := params["message"].(string)
	command, _ := params["command"].(string)
	everySeconds, _ := params["every_seconds"].(float64)
	onceSeconds, _ := params["once_seconds"].(float64)
	cronExpr, _ := params["cron_expr"].(string)
	at, _ := params["at"].(string)

	// 默认模式为 message
	if mode == "" {
		mode = "message"
	}

	// 验证会话上下文
	if t.channel == "" || t.chatID == "" {
		return "Error: no session context (channel/chat_id)", nil
	}

	// 根据模式验证参数
	var taskContent string  // 任务内容（用于 Name）
	var triggerAgent bool   // 是否触发 Agent
	var agentCommand string // Agent 命令

	if mode == "agent" {
		// Agent 模式：需要 command 参数
		if command == "" {
			return "Error: 'command' parameter is required for agent mode", nil
		}
		taskContent = command
		triggerAgent = true
		agentCommand = command
		log.Printf("[CronTool] Agent模式任务: %s", command)
	} else {
		// Message 模式：需要 message 参数
		if message == "" {
			return "Error: 'message' parameter is required for message mode", nil
		}
		taskContent = message
		triggerAgent = false
		log.Printf("[CronTool] Message模式任务: %s", message)
	}

	// 构建调度配置
	schedule := cron.Schedule{}
	deleteAfter := false // 默认不删除

	if onceSeconds > 0 {
		// 一次性延迟任务（N秒后执行一次）
		targetTime := time.Now().Add(time.Duration(onceSeconds) * time.Second)
		schedule = cron.Schedule{
			Kind: "at",
			AtMs: targetTime.UnixMilli(),
		}
		deleteAfter = true
		log.Printf("[CronTool] 创建一次性延迟任务: %s, %d秒后执行 (%v)", taskContent, int64(onceSeconds), targetTime)
	} else if everySeconds > 0 {
		// 周期性任务（每隔N秒执行一次）
		schedule = cron.Schedule{
			Kind:    "every",
			EveryMs: int64(everySeconds) * 1000,
		}
		deleteAfter = false
		log.Printf("[CronTool] 创建周期性任务: %s, 每%d秒执行一次", taskContent, int64(everySeconds))
	} else if cronExpr != "" {
		// Cron表达式任务（如：每天9点执行）
		schedule = cron.Schedule{
			Kind:     "cron",
			CronExpr: cronExpr,
		}
		deleteAfter = false
		log.Printf("[CronTool] 创建cron任务: %s, 表达式: %s", taskContent, cronExpr)
	} else if at != "" {
		// 一次性任务（在指定时间执行一次）
		targetTime, err := time.ParseInLocation("2006-01-02T15:04:05", at, time.Local)
		if err != nil {
			targetTime, err = time.ParseInLocation("2006-01-02T15:04", at, time.Local)
		}
		if err != nil {
			return "Error: invalid 'at' datetime format. Use 'YYYY-MM-DDTHH:MM:SS' or 'YYYY-MM-DDTHH:MM'", nil
		}

		log.Printf("[CronTool] 解析时间: 输入='%s', 解析结果=%v, UnixMs=%d",
			at, targetTime, targetTime.UnixMilli())
		schedule = cron.Schedule{
			Kind: "at",
			AtMs: targetTime.UnixMilli(),
		}
		deleteAfter = true
	} else {
		return "Error: either once_seconds, every_seconds, cron_expr, or at is required", nil
	}

	// 创建任务
	job := &cron.Job{
		Name:           taskContent,
		Message:        message, // Message 模式的内容
		Schedule:       schedule,
		Channel:        t.channel,
		To:             t.chatID,
		Deliver:        true,
		DeleteAfterRun: deleteAfter,
		// Agent 模式字段
		TriggerAgent: triggerAgent,
		AgentCommand: agentCommand,
	}

	// 添加到调度器
	t.cronService.AddJob(job)

	modeDesc := "agent"
	if !triggerAgent {
		modeDesc = "message"
	}
	return fmt.Sprintf("Created %s job '%s' (id: %s, type: %s)", modeDesc, job.Name, job.ID, schedule.Kind), nil
}

// listJobs 列出所有已调度的任务
// 返回:
//
//	所有任务的列表字符串，包含任务名称、ID、调度类型、模式和状态
func (t *CronTool) listJobs() (string, error) {
	jobs := t.cronService.ListJobs()

	if len(jobs) == 0 {
		return "No scheduled jobs found. Use 'add' action to create a reminder.", nil
	}

	result := "Scheduled jobs (" + fmt.Sprintf("%d", len(jobs)) + " total):\n"
	for _, job := range jobs {
		// 格式化任务信息
		jobType := ""
		switch job.Schedule.Kind {
		case "at":
			jobType = "one-time"
		case "every":
			jobType = "recurring"
		case "cron":
			jobType = "scheduled"
		}

		// 显示执行模式
		mode := "message"
		if job.TriggerAgent {
			mode = "agent"
		}

		result += fmt.Sprintf("- %s (id: %s, type: %s, mode: %s)\n", job.Name, job.ID, jobType, mode)
	}

	result += "\nTo remove a job, use 'remove' action with the job_id."
	return result, nil
}

// removeJob 删除指定ID的任务
// 参数:
//
//	params: 参数map，必须包含"job_id"
//
// 返回:
//
//	删除结果的描述字符串
func (t *CronTool) removeJob(params map[string]interface{}) (string, error) {
	// 获取任务ID
	jobID, _ := params["job_id"].(string)

	// 验证参数
	if jobID == "" {
		return "Error: job_id is required for remove", nil
	}

	// 执行删除
	if t.cronService.RemoveJob(jobID) {
		return "Removed job " + jobID, nil
	}

	return "Job " + jobID + " not found", nil
}
