package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// base.go - 工具接口定义和基础实现
// 此文件定义了代理工具的核心接口和基础结构，提供所有工具的通用功能

// Tool 是代理工具的接口定义
// 所有工具必须实现此接口才能被注册和使用
type Tool interface {
	// Name 返回工具的名称标识符
	Name() string
	// Description 返回工具的功能描述
	Description() string
	// Parameters 返回工具的参数定义（JSON Schema格式）
	Parameters() map[string]interface{}
	// ToSchema 将工具转换为OpenAI函数调用格式的schema
	ToSchema() map[string]interface{}
	// ValidateParams 验证传入的参数是否符合要求，返回错误列表
	ValidateParams(params map[string]interface{}) []string
	// Execute 执行工具的核心逻辑，返回执行结果或错误
	Execute(ctx context.Context, params map[string]interface{}) (string, error)
}

type toolContextKey struct{}

// ToolContext carries the default chat target for context-aware tools.
type ToolContext struct {
	Channel string
	ChatID  string
}

// WithToolContext attaches the current chat target to a context.
func WithToolContext(ctx context.Context, channel, chatID string) context.Context {
	return context.WithValue(ctx, toolContextKey{}, ToolContext{Channel: channel, ChatID: chatID})
}

// ToolContextFrom returns the current chat target stored in ctx.
func ToolContextFrom(ctx context.Context) (ToolContext, bool) {
	toolCtx, ok := ctx.Value(toolContextKey{}).(ToolContext)
	return toolCtx, ok
}

// BaseTool 提供工具的通用功能实现
// 其他具体工具可以通过嵌入此结构体来复用基础功能
type BaseTool struct {
	name        string                 // 工具名称
	description string                 // 工具描述
	parameters  map[string]interface{} // 参数定义（JSON Schema格式）
}

// NewBaseTool 创建一个新的基础工具实例
// 参数:
//
//	name: 工具名称标识符
//	description: 工具功能描述
//	parameters: 参数定义（JSON Schema格式）
func NewBaseTool(name string, description string, parameters map[string]interface{}) BaseTool {
	return BaseTool{
		name:        name,
		description: description,
		parameters:  parameters,
	}
}

// Name 返回工具名称
func (t *BaseTool) Name() string {
	return t.name
}

// Description 返回工具描述
func (t *BaseTool) Description() string {
	return t.description
}

// Parameters 返回工具参数定义
func (t *BaseTool) Parameters() map[string]interface{} {
	return t.parameters
}

// ToSchema 将工具转换为OpenAI函数调用格式的schema
// 返回包含type和function字段的map，符合OpenAI API要求
func (t *BaseTool) ToSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        t.Name(),
			"description": t.Description(),
			"parameters":  t.Parameters(),
		},
	}
}

// ValidateParams 验证工具参数的有效性
// 检查所有必需参数是否存在，返回错误信息列表
// 参数:
//
//	params: 待验证的参数map
//
// 返回:
//
//	错误信息的字符串切片，如果验证通过则为空切片
func (t *BaseTool) ValidateParams(params map[string]interface{}) []string {
	var errors []string

	// 检查每个必需参数是否存在
	for _, name := range RequiredParameterNames(t.parameters) {
		if _, exists := params[name]; !exists {
			errors = append(errors, fmt.Sprintf("missing required parameter: %s", name))
		}
	}

	return errors
}

// RequiredParameterNames extracts required parameter names from a JSON schema.
func RequiredParameterNames(schema map[string]interface{}) []string {
	if schema == nil {
		return nil
	}

	switch required := schema["required"].(type) {
	case []string:
		return required
	case []interface{}:
		names := make([]string, 0, len(required))
		for _, item := range required {
			if name, ok := item.(string); ok {
				names = append(names, name)
			}
		}
		return names
	default:
		return nil
	}
}

// ToolResult 表示工具执行的结果
// 用于统一封装工具执行的成功/失败状态和输出信息
type ToolResult struct {
	Success bool   // 执行是否成功
	Output  string // 输出内容
	Error   error  // 错误信息（如果有）
}

// NewToolResult 创建一个新的工具结果实例
// 参数:
//
//	success: 是否成功
//	output: 输出内容
//	err: 错误信息
func NewToolResult(success bool, output string, err error) ToolResult {
	return ToolResult{
		Success: success,
		Output:  output,
		Error:   err,
	}
}

// String 返回结果的字符串表示
// 如果有错误则返回错误信息，否则返回输出内容
func (r ToolResult) String() string {
	if r.Error != nil {
		return fmt.Sprintf("Error: %v", r.Error)
	}
	return r.Output
}

// JSONString 将任意值转换为JSON字符串
// 这是一个辅助函数，用于将结果序列化为JSON格式
// 参数:
//
//	v: 要序列化的值
//
// 返回:
//
//	JSON字符串，如果序列化失败则返回包含错误信息的JSON
func JSONString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error": "%v"}`, err)
	}
	return string(b)
}
