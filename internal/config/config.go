package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config 表示 nanogrip 的根配置结构体
// 这个结构体包含了整个系统的所有配置项，包括代理、通道、提供商和工具
type Config struct {
	// Agents 代理配置，包含代理的默认行为设置
	// `yaml:"agents"` 表示此字段对应 YAML 文件中的 "agents" 键
	Agents AgentsConfig `yaml:"agents"`

	// Channels 通道配置，包含各种消息平台的连接设置
	// `yaml:"channels"` 表示此字段对应 YAML 文件中的 "channels" 键
	Channels ChannelsConfig `yaml:"channels"`

	// Providers LLM 提供商配置，包含各种 AI 模型服务商的认证信息
	// `yaml:"providers"` 表示此字段对应 YAML 文件中的 "providers" 键
	Providers ProvidersConfig `yaml:"providers"`

	// Tools 工具配置，包含各种工具的设置（如网络搜索、命令执行等）
	// `yaml:"tools"` 表示此字段对应 YAML 文件中的 "tools" 键
	Tools ToolsConfig `yaml:"tools"`

	// MCPServers MCP 服务器配置
	// 用于连接外部 MCP 服务器以扩展工具能力
	MCPServers map[string]MCPServerConfig `yaml:"mcpServers"`
}

// AgentsConfig 包含代理的配置设置
// 定义了 AI 代理的各种行为参数
type AgentsConfig struct {
	// Defaults 代理的默认配置
	// `yaml:"defaults"` 表示此字段对应 YAML 文件中的 "defaults" 键
	Defaults AgentDefaults `yaml:"defaults"`
}

// AgentDefaults 包含代理的默认配置参数
// 这些参数控制 AI 代理的核心行为和性能特性
type AgentDefaults struct {
	// Workspace 工作空间路径，代理执行任务时使用的目录
	// 例如："~/.nanogrip/workspace"
	// `yaml:"workspace"` 表示此字段对应 YAML 文件中的 "workspace" 键
	Workspace string `yaml:"workspace"`

	// Model 使用的 AI 模型标识符
	// 格式通常为 "提供商/模型名称"，例如："anthropic/claude-opus-4-5"
	// `yaml:"model"` 表示此字段对应 YAML 文件中的 "model" 键
	Model string `yaml:"model"`

	// MaxTokens 单次请求的最大 token 数量
	// 控制生成文本的长度上限，默认值为 8192
	// `yaml:"maxTokens"` 表示此字段对应 YAML 文件中的 "maxTokens" 键
	MaxTokens int `yaml:"maxTokens"`

	// Temperature 温度参数，控制输出的随机性
	// 范围通常为 0.0-1.0，值越高输出越随机，默认值为 0.7
	// `yaml:"temperature"` 表示此字段对应 YAML 文件中的 "temperature" 键
	Temperature float64 `yaml:"temperature"`

	// MaxToolIterations 工具调用的最大迭代次数
	// 限制代理在单次任务中可以调用工具的次数，防止无限循环，默认值为 20
	// `yaml:"maxToolIterations"` 表示此字段对应 YAML 文件中的 "maxToolIterations" 键
	MaxToolIterations int `yaml:"maxToolIterations"`

	// MemoryWindow 记忆窗口大小
	// 定义代理保留多少条历史消息进行上下文记忆，默认值为 50
	// `yaml:"memoryWindow"` 表示此字段对应 YAML 文件中的 "memoryWindow" 键
	MemoryWindow int `yaml:"memoryWindow"`
}

// ChannelsConfig 包含消息通道的配置
// 支持 Telegram 消息平台
type ChannelsConfig struct {
	// Telegram Telegram 消息平台配置
	// `yaml:"telegram"` 表示此字段对应 YAML 文件中的 "telegram" 键
	Telegram TelegramConfig `yaml:"telegram"`
}

// TelegramConfig 包含 Telegram 通道的配置
// 用于连接和控制 Telegram 机器人服务
type TelegramConfig struct {
	// Enabled 是否启用 Telegram 通道
	// `yaml:"enabled"` 表示此字段对应 YAML 文件中的 "enabled" 键
	Enabled bool `yaml:"enabled"`

	// Token Telegram 机器人的 API Token
	// 从 BotFather 获取的认证令牌
	// `yaml:"token"` 表示此字段对应 YAML 文件中的 "token" 键
	Token string `yaml:"token"`

	// AllowFrom 允许交互的用户 ID 白名单列表
	// `yaml:"allowFrom"` 表示此字段对应 YAML 文件中的 "allowFrom" 键
	AllowFrom []string `yaml:"allowFrom"`

	// Proxy 代理服务器地址（可选）
	// 用于在受限网络环境中访问 Telegram API
	// `yaml:"proxy"` 表示此字段对应 YAML 文件中的 "proxy" 键
	Proxy string `yaml:"proxy"`

	// ReplyToMessage 是否以回复消息的方式响应
	// `yaml:"replyToMessage"` 表示此字段对应 YAML 文件中的 "replyToMessage" 键
	ReplyToMessage bool `yaml:"replyToMessage"`
}

// ProvidersConfig 包含各种 LLM（大语言模型）提供商的配置
// 支持多个 AI 服务提供商，每个提供商都有自己的 API 密钥和基础 URL
type ProvidersConfig struct {
	// Custom 自定义提供商配置
	// `yaml:"custom"` 表示此字段对应 YAML 文件中的 "custom" 键
	Custom ProviderConfig `yaml:"custom"`

	// Anthropic Anthropic (Claude) 提供商配置
	// `yaml:"anthropic"` 表示此字段对应 YAML 文件中的 "anthropic" 键
	Anthropic ProviderConfig `yaml:"anthropic"`

	// OpenAI OpenAI (GPT) 提供商配置
	// `yaml:"openai"` 表示此字段对应 YAML 文件中的 "openai" 键
	OpenAI ProviderConfig `yaml:"openai"`

	// OpenRouter OpenRouter 提供商配置
	// `yaml:"openrouter"` 表示此字段对应 YAML 文件中的 "openrouter" 键
	OpenRouter ProviderConfig `yaml:"openrouter"`

	// DeepSeek DeepSeek 提供商配置
	// `yaml:"deepseek"` 表示此字段对应 YAML 文件中的 "deepseek" 键
	DeepSeek ProviderConfig `yaml:"deepseek"`

	// Groq Groq 提供商配置
	// `yaml:"groq"` 表示此字段对应 YAML 文件中的 "groq" 键
	Groq ProviderConfig `yaml:"groq"`

	// Zhipu 智谱 AI 提供商配置
	// `yaml:"zhipu"` 表示此字段对应 YAML 文件中的 "zhipu" 键
	Zhipu ProviderConfig `yaml:"zhipu"`

	// DashScope 阿里云百炼（DashScope）提供商配置
	// `yaml:"dashscope"` 表示此字段对应 YAML 文件中的 "dashscope" 键
	DashScope ProviderConfig `yaml:"dashscope"`

	// VLLM vLLM 提供商配置
	// `yaml:"vllm"` 表示此字段对应 YAML 文件中的 "vllm" 键
	VLLM ProviderConfig `yaml:"vllm"`

	// Gemini Google Gemini 提供商配置
	// `yaml:"gemini"` 表示此字段对应 YAML 文件中的 "gemini" 键
	Gemini ProviderConfig `yaml:"gemini"`

	// MoonShot 月之暗面（MoonShot）提供商配置
	// `yaml:"moonshot"` 表示此字段对应 YAML 文件中的 "moonshot" 键
	MoonShot ProviderConfig `yaml:"moonshot"`

	// MiniMax MiniMax 提供商配置
	// `yaml:"minimax"` 表示此字段对应 YAML 文件中的 "minimax" 键
	MiniMax ProviderConfig `yaml:"minimax"`

	// AiHubMix AiHubMix 提供商配置
	// `yaml:"aihubmix"` 表示此字段对应 YAML 文件中的 "aihubmix" 键
	AiHubMix ProviderConfig `yaml:"aihubmix"`

	// SiliconFlow SiliconFlow 提供商配置
	// `yaml:"siliconflow"` 表示此字段对应 YAML 文件中的 "siliconflow" 键
	SiliconFlow ProviderConfig `yaml:"siliconflow"`

	// VolcEngine 火山引擎（VolcEngine）提供商配置
	// `yaml:"volcengine"` 表示此字段对应 YAML 文件中的 "volcengine" 键
	VolcEngine ProviderConfig `yaml:"volcengine"`

	// OpenAICodex OpenAI Codex 提供商配置
	// `yaml:"openai_codex"` 表示此字段对应 YAML 文件中的 "openai_codex" 键
	OpenAICodex ProviderConfig `yaml:"openai_codex"`

	// GithubCopilot GitHub Copilot 提供商配置
	// `yaml:"github_copilot"` 表示此字段对应 YAML 文件中的 "github_copilot" 键
	GithubCopilot ProviderConfig `yaml:"github_copilot"`
}

// ProviderConfig 包含单个 LLM 提供商的具体配置信息
// 用于配置 API 访问凭证和自定义请求设置
type ProviderConfig struct {
	// APIKey API 密钥
	// 用于身份验证和访问提供商的服务
	// `yaml:"apiKey"` 表示此字段对应 YAML 文件中的 "apiKey" 键
	APIKey string `yaml:"apiKey"`

	// APIBase API 基础 URL
	// 用于自定义 API 端点地址，如使用代理或自托管服务
	// `yaml:"apiBase"` 表示此字段对应 YAML 文件中的 "apiBase" 键
	APIBase string `yaml:"apiBase"`

	// ExtraHeaders 额外的 HTTP 请求头
	// 可以添加自定义的请求头，如特殊的认证信息或元数据
	// `yaml:"extraHeaders"` 表示此字段对应 YAML 文件中的 "extraHeaders" 键
	ExtraHeaders map[string]string `yaml:"extraHeaders"`
}

// ToolsConfig 包含工具的配置信息
// 定义机器人可以使用的各种工具及其设置
type ToolsConfig struct {
	// Web 网络工具配置（如网页搜索）
	// `yaml:"web"` 表示此字段对应 YAML 文件中的 "web" 键
	Web WebToolsConfig `yaml:"web"`

	// Exec 命令执行工具配置
	// `yaml:"exec"` 表示此字段对应 YAML 文件中的 "exec" 键
	Exec ExecToolConfig `yaml:"exec"`

	// RestrictToWorkspace 是否将文件操作限制在工作空间内
	// 为 true 时，机器人只能访问和修改工作空间内的文件
	// `yaml:"restrictToWorkspace"` 表示此字段对应 YAML 文件中的 "restrictToWorkspace" 键
	RestrictToWorkspace bool `yaml:"restrictToWorkspace"`

	// MCPServers MCP（Model Context Protocol）服务器配置
	// 键为服务器名称，值为对应的配置
	// `yaml:"mcpServers"` 表示此字段对应 YAML 文件中的 "mcpServers" 键
	MCPServers map[string]MCPServerConfig `yaml:"mcpServers"`
}

// WebToolsConfig 包含网络工具的配置
// 定义机器人使用的网络相关工具
type WebToolsConfig struct {
	// Search 网络搜索配置
	// `yaml:"search"` 表示此字段对应 YAML 文件中的 "search" 键
	Search WebSearchConfig `yaml:"search"`
}

// WebSearchConfig 包含网络搜索的配置信息
// 用于配置搜索引擎 API 的访问
type WebSearchConfig struct {
	// APIKey 搜索引擎 API 的密钥
	// `yaml:"apiKey"` 表示此字段对应 YAML 文件中的 "apiKey" 键
	APIKey string `yaml:"apiKey"`

	// Provider 搜索提供商 (brave 或 tavily)
	// `yaml:"provider"` 表示此字段对应 YAML 文件中的 "provider" 键
	Provider string `yaml:"provider"`

	// MaxResults 搜索返回的最大结果数
	// `yaml:"maxResults"` 表示此字段对应 YAML 文件中的 "maxResults" 键
	MaxResults int `yaml:"maxResults"`
}

// ExecToolConfig 包含 Shell 命令执行的配置
// 控制机器人执行系统命令的行为
type ExecToolConfig struct {
	// Timeout 命令执行的超时时间（秒）
	// 防止命令执行时间过长导致系统资源占用
	// `yaml:"timeout"` 表示此字段对应 YAML 文件中的 "timeout" 键
	Timeout int `yaml:"timeout"`
}

// MCPServerConfig 包含 MCP 服务器的配置
// MCP（Model Context Protocol）允许机器人连接到外部服务以扩展功能
type MCPServerConfig struct {
	// Command 启动 MCP 服务器的命令
	// `yaml:"command"` 表示此字段对应 YAML 文件中的 "command" 键
	Command string `yaml:"command"`

	// Args 命令行参数列表
	// `yaml:"args"` 表示此字段对应 YAML 文件中的 "args" 键
	Args []string `yaml:"args"`

	// Env 环境变量
	// 键为环境变量名，值为对应的值
	// `yaml:"env"` 表示此字段对应 YAML 文件中的 "env" 键
	Env map[string]string `yaml:"env"`

	// URL MCP 服务器的 URL 地址
	// 用于连接远程 MCP 服务器
	// `yaml:"url"` 表示此字段对应 YAML 文件中的 "url" 键
	URL string `yaml:"url"`

	// Headers HTTP 请求头
	// 用于访问 MCP 服务器时添加的自定义请求头
	// `yaml:"headers"` 表示此字段对应 YAML 文件中的 "headers" 键
	Headers map[string]string `yaml:"headers"`
}

// Load 从指定路径加载配置文件
// 参数:
//
//	path - 配置文件的路径，支持 "~/" 开头的路径（会自动扩展为用户主目录）
//
// 返回:
//
//	*Config - 加载并解析后的配置对象
//	error - 如果读取或解析失败，返回错误信息
//
// 功能说明:
//  1. 自动展开路径中的 "~/" 为用户主目录
//  2. 读取 YAML 配置文件
//  3. 解析 YAML 内容到 Config 结构体
//  4. 为未设置的字段填充默认值
func Load(path string) (*Config, error) {
	// 尝试展开用户主目录
	// 如果路径以 "~/" 开头，将其替换为实际的用户主目录路径
	if len(path) >= 2 && path[0:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, path[2:])
	}

	// 读取配置文件内容
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// 解析 YAML 配置文件
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// 设置默认值
	// 如果配置文件中没有指定某些字段，则使用这些默认值

	// 工作空间默认路径
	if cfg.Agents.Defaults.Workspace == "" {
		cfg.Agents.Defaults.Workspace = "~/.nanogrip/workspace"
	}
	// 默认使用的 AI 模型
	if cfg.Agents.Defaults.Model == "" {
		cfg.Agents.Defaults.Model = "anthropic/claude-opus-4-5"
	}
	// 默认最大 token 数
	if cfg.Agents.Defaults.MaxTokens == 0 {
		cfg.Agents.Defaults.MaxTokens = 8192
	}
	// 默认温度参数
	if cfg.Agents.Defaults.Temperature == 0 {
		cfg.Agents.Defaults.Temperature = 0.7
	}
	// 默认最大工具迭代次数
	if cfg.Agents.Defaults.MaxToolIterations == 0 {
		cfg.Agents.Defaults.MaxToolIterations = 20
	}
	// 默认记忆窗口大小
	if cfg.Agents.Defaults.MemoryWindow == 0 {
		cfg.Agents.Defaults.MemoryWindow = 50
	}

	return &cfg, nil
}

// GetWorkspacePath 返回展开后的工作空间路径
// 这是 Config 结构体的方法，用于获取实际的工作空间目录路径
// 返回:
//
//	string - 完整的工作空间路径
//
// 功能说明:
//
//	如果工作空间路径以 "~/" 开头，会自动展开为用户主目录的完整路径
//	例如："~/.nanogrip/workspace" 会被展开为 "/home/username/.nanogrip/workspace"
func (c *Config) GetWorkspacePath() string {
	// 检查工作空间路径是否以 "~/" 开头
	if len(c.Agents.Defaults.Workspace) >= 2 && c.Agents.Defaults.Workspace[0:2] == "~/" {
		// 获取用户主目录
		home, err := os.UserHomeDir()
		if err != nil {
			// 如果无法获取主目录，返回原始路径
			return c.Agents.Defaults.Workspace
		}
		// 将 "~/" 替换为实际的主目录路径
		return filepath.Join(home, c.Agents.Defaults.Workspace[2:])
	}
	// 如果不是以 "~/" 开头，直接返回原始路径
	return c.Agents.Defaults.Workspace
}
