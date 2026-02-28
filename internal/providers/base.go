// Package providers 提供了LLM提供商的核心接口和类型定义
// 这个包定义了与各种大语言模型提供商交互所需的基础数据结构和接口
// 包括消息格式、工具调用、响应处理等核心功能
package providers

import (
	"context"
	"encoding/json"
)

// ToolCallRequest 表示来自LLM的工具调用请求
// 当LLM需要调用外部工具或函数时，会返回这种结构的请求
type ToolCallRequest struct {
	ID        string                 // 工具调用的唯一标识符，用于追踪和响应
	Name      string                 // 要调用的工具或函数名称
	Arguments map[string]interface{} // 工具调用的参数，以键值对形式存储
}

// LLMResponse 表示来自LLM提供商的响应
// 包含了模型生成的文本内容、工具调用、使用统计等完整信息
type LLMResponse struct {
	Content          string            // 模型生成的主要文本内容
	ToolCalls        []ToolCallRequest // 模型请求的工具调用列表，可能为空
	FinishReason     string            // 完成原因，如"stop"、"length"、"tool_calls"等
	Usage            map[string]int    // token使用统计，包括prompt_tokens、completion_tokens等
	ReasoningContent string            // 推理内容（某些模型如DeepSeek支持），用于存储模型的思考过程
}

// HasToolCalls 检查响应中是否包含工具调用
// 这是一个便捷方法，用于快速判断是否需要处理工具调用
func (r *LLMResponse) HasToolCalls() bool {
	return len(r.ToolCalls) > 0
}

// Message 表示聊天消息
// 这是与LLM交互的基本单位，支持不同的消息角色和工具调用
type Message struct {
	Role       string            `json:"role"`                   // 消息角色：user(用户)、assistant(助手)、system(系统)、tool(工具)
	Content    string            `json:"content"`                // 消息的文本内容
	Images     []string          `json:"images,omitempty"`       // 图片URL或base64数据（用于视觉模型）
	Tools      []ToolCallRequest `json:"tool_calls,omitempty"`   // 当角色为assistant时，包含的工具调用请求
	ToolCallID string            `json:"tool_call_id,omitempty"` // 当角色为tool时，对应的工具调用ID
	Name       string            `json:"name,omitempty"`         // 工具名称或函数名称（用于工具响应消息）
}

// LLMProvider 是LLM提供商的核心接口
// 所有具体的LLM提供商实现都必须实现这个接口
type LLMProvider interface {
	// Chat 发送聊天完成请求到LLM提供商
	// ctx: 上下文，用于控制请求的生命周期和取消
	// messages: 消息历史，包含对话上下文
	// tools: 可用的工具定义列表，LLM可以选择调用这些工具
	// model: 要使用的模型名称
	// maxTokens: 生成的最大token数量
	// temperature: 温度参数，控制生成的随机性（0-1之间，越高越随机）
	// 返回: LLM的响应和可能的错误
	Chat(ctx context.Context, messages []Message, tools []ToolDef, model string, maxTokens int, temperature float64) (*LLMResponse, error)

	// GetDefaultModel 返回提供商的默认模型名称
	// 当用户未指定模型时，使用这个默认值
	GetDefaultModel() string
}

// ToolDef 表示工具定义
// 用于向LLM描述可用的工具或函数，遵循OpenAI的工具调用格式
type ToolDef struct {
	Type     string      `json:"type"`     // 工具类型，通常为"function"
	Function FunctionDef `json:"function"` // 函数的详细定义
}

// FunctionDef 表示函数定义
// 包含函数的名称、描述和参数schema，用于让LLM理解如何使用这个工具
type FunctionDef struct {
	Name        string                 `json:"name"`        // 函数名称，应该是描述性的标识符
	Description string                 `json:"description"` // 函数的文字描述，帮助LLM理解函数的用途
	Parameters  map[string]interface{} `json:"parameters"`  // 参数schema，通常是JSON Schema格式
}

// ParseToolArguments 从JSON字符串解析工具参数
// 这是一个辅助函数，用于将LLM返回的JSON格式参数字符串解析为map
// 参数:
//
//	args: JSON格式的参数字符串
//
// 返回:
//
//	解析后的参数map和可能的错误
func ParseToolArguments(args string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := json.Unmarshal([]byte(args), &result)
	return result, err
}
