// Package mcp 提供了 MCP (Model Context Protocol) 客户端实现
// 基于 mark3labs/mcp-go 库实现
// 支持两种 MCP 服务器类型：命令启动型和 URL 连接型
package mcp

import (
	"context" // context 用于控制协程生命周期
	"fmt"     // fmt 用于格式化输出
	"log"     // log 用于日志记录
	"sync"    // sync 用于同步原语

	"github.com/Ailoc/nanogrip/internal/tools"        // 工具接口定义
	"github.com/mark3labs/mcp-go/client"           // mcp-go 客户端
	"github.com/mark3labs/mcp-go/client/transport" // mcp-go 传输层
	"github.com/mark3labs/mcp-go/mcp"              // mcp-go 核心类型
)

// MCPClient MCP 客户端
// 基于 mcp-go 库实现，用于连接到 MCP 服务器并获取可用的工具列表
type MCPClient struct {
	name      string         // 服务器名称
	config    *MCPConfig     // MCP 服务器配置
	mcpClient *client.Client // mcp-go 客户端实例
	tools     []tools.Tool   // 从 MCP 服务器获取的工具列表
	mu        sync.RWMutex   // 读写锁
	running   bool           // 运行状态
}

// MCPConfig MCP 服务器配置
// 定义如何连接到 MCP 服务器
type MCPConfig struct {
	// Command 启动 MCP 服务器的命令（命令型）
	// 例如：["npx", "-y", "@modelcontextprotocol/server-filesystem", "/path/to/dir"]
	Command string `yaml:"command"`

	// Args 命令行参数列表
	// 与 Command 配合使用
	Args []string `yaml:"args"`

	// Env 环境变量
	// 启动命令时设置的环境变量
	Env map[string]string `yaml:"env"`

	// URL MCP 服务器的 HTTP URL（URL 型）
	// 直接连接到已运行的 MCP 服务器
	URL string `yaml:"url"`

	// Headers HTTP 请求头
	// 访问 MCP 服务器时使用的自定义请求头
	Headers map[string]string `yaml:"headers"`
}

// NewMCPClient 创建新的 MCP 客户端
// 参数：
//   - name: 服务器名称
//   - config: MCP 服务器配置
//
// 返回：
//   - *MCPClient: 新的 MCP 客户端实例
func NewMCPClient(name string, config *MCPConfig) *MCPClient {
	return &MCPClient{
		name:    name,
		config:  config,
		tools:   []tools.Tool{},
		running: false,
	}
}

// Start 启动 MCP 客户端
// 根据配置类型（命令型或 URL 型）连接到 MCP 服务器
func (m *MCPClient) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return nil // 已经运行
	}

	// 检查配置类型
	if m.config.URL != "" {
		// URL 型：连接到 HTTP SSE 服务器
		return m.startHTTP()
	} else if m.config.Command != "" {
		// 命令型：启动本地进程
		return m.startStdio()
	}

	return fmt.Errorf("MCP client %s: 未配置 URL 或 Command", m.name)
}

// startHTTP 启动 HTTP 类型的 MCP 客户端
// 使用 SSE (Server-Sent Events) 连接到 MCP 服务器
func (m *MCPClient) startHTTP() error {
	log.Printf("MCP client %s: 连接到 SSE 服务器 %s", m.name, m.config.URL)

	// 创建 SSE 传输选项
	var opts []transport.ClientOption
	if len(m.config.Headers) > 0 {
		opts = append(opts, transport.WithHeaders(m.config.Headers))
	}

	// 创建 SSE MCP 客户端
	mcpClient, err := client.NewSSEMCPClient(m.config.URL, opts...)
	if err != nil {
		return fmt.Errorf("创建 SSE 客户端失败: %w", err)
	}

	// 启动连接
	ctx := context.Background()
	if err := mcpClient.Start(ctx); err != nil {
		return fmt.Errorf("启动客户端失败: %w", err)
	}

	// 初始化连接
	_, err = mcpClient.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "nanobot-go",
				Version: "1.0.0",
			},
		},
	})
	if err != nil {
		mcpClient.Close()
		return fmt.Errorf("初始化失败: %w", err)
	}

	m.mcpClient = mcpClient

	// 获取工具列表
	if err := m.fetchTools(); err != nil {
		log.Printf("MCP client %s: 获取工具列表失败: %v", m.name, err)
		return err
	}

	m.running = true
	log.Printf("MCP client %s: 已启动 (HTTP)", m.name)
	return nil
}

// startStdio 启动命令型的 MCP 客户端
// 使用 stdio 传输连接到 MCP 服务器
func (m *MCPClient) startStdio() error {
	log.Printf("MCP client %s: 启动命令 %s %v", m.name, m.config.Command, m.config.Args)

	// 构建环境变量
	var env []string
	for k, v := range m.config.Env {
		env = append(env, k+"="+v)
	}

	// 创建 stdio 传输
	stdioTransport := transport.NewStdio(m.config.Command, env, m.config.Args...)

	// 创建 MCP 客户端
	mcpClient := client.NewClient(stdioTransport)

	// 启动连接
	ctx := context.Background()
	if err := mcpClient.Start(ctx); err != nil {
		return fmt.Errorf("启动客户端失败: %w", err)
	}

	// 初始化连接
	_, err := mcpClient.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "nanobot-go",
				Version: "1.0.0",
			},
		},
	})
	if err != nil {
		mcpClient.Close()
		return fmt.Errorf("初始化失败: %w", err)
	}

	m.mcpClient = mcpClient

	// 获取工具列表
	if err := m.fetchTools(); err != nil {
		log.Printf("MCP client %s: 获取工具列表失败: %v", m.name, err)
		return err
	}

	m.running = true
	log.Printf("MCP client %s: 已启动 (Stdio)", m.name)
	return nil
}

// fetchTools 从 MCP 服务器获取工具列表
func (m *MCPClient) fetchTools() error {
	ctx := context.Background()

	// 调用 ListTools
	result, err := m.mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return fmt.Errorf("调用 ListTools 失败: %w", err)
	}

	// 转换为工具
	m.tools = make([]tools.Tool, len(result.Tools))
	for i, t := range result.Tools {
		m.tools[i] = m.newTool(t)
	}

	log.Printf("MCP client %s: 获取到 %d 个工具", m.name, len(m.tools))
	return nil
}

// newTool 将 MCP 工具转换为 nanobot 工具
func (m *MCPClient) newTool(mcpTool mcp.Tool) tools.Tool {
	// 解析输入模式 - ToolInputSchema 基于 ToolArgumentsSchema
	// 包含 Type, Properties, Required 等字段
	inputSchema := mcpTool.InputSchema
	params := map[string]interface{}{
		"type": inputSchema.Type,
	}
	if len(inputSchema.Properties) > 0 {
		params["properties"] = inputSchema.Properties
	}
	if len(inputSchema.Required) > 0 {
		params["required"] = inputSchema.Required
	}

	// 返回封装后的工具
	return &mcpToolWrapper{
		name:        mcpTool.Name,
		description: mcpTool.Description,
		parameters:  params,
		mcpClient:   m,
	}
}

// mcpToolWrapper MCP 工具包装器
// 将 MCP 工具适配为 nanobot 工具接口
type mcpToolWrapper struct {
	name        string
	description string
	parameters  map[string]interface{}
	mcpClient   *MCPClient
}

// Name 返回工具名称
func (t *mcpToolWrapper) Name() string {
	return t.name
}

// Description 返回工具描述
func (t *mcpToolWrapper) Description() string {
	return t.description
}

// Parameters 返回工具参数定义
func (t *mcpToolWrapper) Parameters() map[string]interface{} {
	return t.parameters
}

// ToSchema 将工具转换为 OpenAI 函数调用格式的 schema
func (t *mcpToolWrapper) ToSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        t.Name(),
			"description": t.Description(),
			"parameters":  t.Parameters(),
		},
	}
}

// ValidateParams 验证参数有效性
func (t *mcpToolWrapper) ValidateParams(params map[string]interface{}) []string {
	// MCP 工具参数验证较宽松，基本只检查必需参数
	var errors []string
	if t.parameters == nil {
		return errors
	}

	// 获取必需参数列表
	required, ok := t.parameters["required"].([]interface{})
	if !ok {
		return errors
	}

	// 检查每个必需参数是否存在
	for _, req := range required {
		reqStr, ok := req.(string)
		if !ok {
			continue
		}
		if _, exists := params[reqStr]; !exists {
			errors = append(errors, fmt.Sprintf("missing required parameter: %s", reqStr))
		}
	}

	return errors
}

// Execute 执行 MCP 工具
// 参数：
//   - ctx: 上下文
//   - params: 工具参数
//
// 返回：
//   - string: 执行结果
//   - error: 执行错误
func (t *mcpToolWrapper) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// 调用 mcp-go 客户端的 CallTool
	// CallToolRequest 使用 Params 字段，包含 Name 和 Arguments
	result, err := t.mcpClient.mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      t.name,
			Arguments: params,
		},
	})
	if err != nil {
		return "", fmt.Errorf("调用工具失败: %w", err)
	}

	// 提取文本内容
	var output string
	for _, content := range result.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			output += textContent.Text
		}
	}

	return output, nil
}

// Stop 停止 MCP 客户端
func (m *MCPClient) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	// 关闭 mcp-go 客户端
	if m.mcpClient != nil {
		m.mcpClient.Close()
	}

	m.running = false
	log.Printf("MCP client %s 已停止", m.name)
}

// GetTools 获取工具列表
func (m *MCPClient) GetTools() []tools.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tools
}

// IsRunning 返回客户端是否在运行
func (m *MCPClient) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// MCPManager MCP 管理器
// 管理所有 MCP 客户端连接
type MCPManager struct {
	clients map[string]*MCPClient // MCP 客户端映射
	mu      sync.RWMutex          // 读写锁
}

// NewMCPManager 创建新的 MCP 管理器
func NewMCPManager() *MCPManager {
	return &MCPManager{
		clients: make(map[string]*MCPClient),
	}
}

// StartAll 启动所有 MCP 服务器
// 参数：
//   - configs: MCP 服务器配置映射
//
// 返回：
//   - error: 启动错误
func (m *MCPManager) StartAll(configs map[string]MCPConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, config := range configs {
		// 创建客户端
		mcpClient := NewMCPClient(name, &config)

		// 启动连接
		if err := mcpClient.Start(); err != nil {
			log.Printf("MCP 服务器 %s 启动失败: %v", name, err)
			continue
		}

		// 注册客户端
		m.clients[name] = mcpClient
		log.Printf("MCP 服务器 %s 已启动", name)
	}

	return nil
}

// StopAll 停止所有 MCP 服务器
func (m *MCPManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, mcpClient := range m.clients {
		mcpClient.Stop()
		log.Printf("MCP 服务器 %s 已停止", name)
	}

	m.clients = make(map[string]*MCPClient)
}

// GetTools 获取所有 MCP 工具
func (m *MCPManager) GetTools() []tools.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var allTools []tools.Tool
	for _, mcpClient := range m.clients {
		allTools = append(allTools, mcpClient.GetTools()...)
	}

	return allTools
}

// GetClient 获取指定的 MCP 客户端
func (m *MCPManager) GetClient(name string) *MCPClient {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clients[name]
}
