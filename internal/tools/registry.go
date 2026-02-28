package tools

import (
	"context"
	"fmt"
	"sync"
)

// registry.go - 工具注册表
// 此文件实现了工具的注册、管理和执行功能，提供线程安全的工具注册中心

// ToolRegistry 管理工具的注册和执行
// 提供线程安全的工具注册、查询和执行服务
type ToolRegistry struct {
	mu    sync.RWMutex    // 读写互斥锁，保证并发安全
	tools map[string]Tool // 工具映射表，键为工具名称，值为工具实例
}

// NewToolRegistry 创建一个新的工具注册表
// 返回初始化好的空注册表实例
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

// Register 注册一个工具
// 将工具添加到注册表中，使其可以被代理使用
// 参数:
//
//	tool: 要注册的工具实例
func (r *ToolRegistry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

// Unregister 注销一个工具
// 从注册表中移除指定名称的工具
// 参数:
//
//	name: 要注销的工具名称
func (r *ToolRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

// Get 根据名称获取工具
// 参数:
//
//	name: 工具名称
//
// 返回:
//
//	工具实例，如果不存在则返回nil
func (r *ToolRegistry) Get(name string) Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

// Has 检查工具是否已注册
// 参数:
//
//	name: 工具名称
//
// 返回:
//
//	如果工具已注册返回true，否则返回false
func (r *ToolRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.tools[name]
	return exists
}

// GetDefinitions 返回所有工具的定义（OpenAI格式）
// 将所有已注册工具的schema转换为OpenAI API所需的格式
// 返回:
//
//	包含所有工具定义的切片，每个定义都是一个map
func (r *ToolRegistry) GetDefinitions() []map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	definitions := make([]map[string]interface{}, 0, len(r.tools))
	for _, tool := range r.tools {
		definitions = append(definitions, tool.ToSchema())
	}

	return definitions
}

// Execute 根据名称执行工具
// 这是工具执行的入口函数，负责查找、验证和执行工具
// 参数:
//
//	ctx: 上下文对象，用于控制超时和取消
//	name: 要执行的工具名称
//	params: 工具参数
//
// 返回:
//
//	工具执行结果的字符串表示
func (r *ToolRegistry) Execute(ctx context.Context, name string, params map[string]interface{}) string {
	// 查找工具
	tool := r.Get(name)
	if tool == nil {
		return fmt.Sprintf(`Error: Tool '%s' not found`, name)
	}

	// 验证参数
	if errors := tool.ValidateParams(params); len(errors) > 0 {
		return fmt.Sprintf(`Error: Invalid parameters for tool '%s': %v`, name, errors)
	}

	// 执行工具
	result, err := tool.Execute(ctx, params)
	if err != nil {
		return fmt.Sprintf(`Error executing %s: %v`, name, err)
	}

	return result
}

// ToolNames 返回所有已注册工具的名称列表
// 返回:
//
//	工具名称的字符串切片
func (r *ToolRegistry) ToolNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}

	return names
}

// Len 返回已注册工具的数量
// 返回:
//
//	工具数量
func (r *ToolRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Contains 检查工具是否已注册
// 这是Has方法的别名，提供更直观的命名
// 参数:
//
//	name: 工具名称
//
// 返回:
//
//	如果工具已注册返回true，否则返回false
func (r *ToolRegistry) Contains(name string) bool {
	return r.Has(name)
}
