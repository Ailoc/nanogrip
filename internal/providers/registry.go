// Package providers 提供了LLM提供商注册表
// 本文件定义了提供商规格和注册表系统
// 包含了所有支持的LLM提供商的配置信息，用于统一管理不同提供商的差异
package providers

import (
	"os"
	"strings"
)

// ProviderSpec 表示一个提供商的规格配置
// 包含了提供商的名称、环境变量、API配置等所有必要信息
type ProviderSpec struct {
	Name                  string          // 提供商的规范名称，如"openai"、"anthropic"等
	Keywords              []string        // 关键词列表，用于模型名称匹配，如["claude", "anthropic"]
	EnvKey                string          // 环境变量名称，用于存储API密钥，如"OPENAI_API_KEY"
	LiteLLMPrefix         string          // LiteLLM格式的提供商前缀，如"openai/"、"anthropic/"
	DefaultAPIBase        string          // 默认的API基础URL，用于网关类型的提供商
	IsGateway             bool            // 是否为网关类型提供商（如OpenRouter、SiliconFlow等聚合服务）
	IsOAuth               bool            // 是否使用OAuth认证（如GitHub Copilot）
	SupportsPromptCaching bool            // 是否支持提示词缓存功能，可以提高性能和降低成本
	SkipPrefixes          []string        // 跳过的前缀列表，用于特殊情况的处理
	EnvExtras             [][]string      // 额外的环境变量配置，格式为[["ENV_NAME", "value"]]
	ModelOverrides        []ModelOverride // 模型特定的参数覆盖配置
	StripModelPrefix      bool            // 是否在发送请求前移除模型名称的前缀
}

// ModelOverride 表示模型特定的参数覆盖
// 用于针对特定模型应用不同的配置参数
type ModelOverride struct {
	Pattern   string                 // 模型名称的匹配模式，可以是正则表达式
	Overrides map[string]interface{} // 要覆盖的参数键值对
}

var (
	// PROVIDERS 是所有支持的LLM提供商的注册表
	// 这个全局变量包含了系统支持的所有提供商配置
	// 在模型解析和环境变量设置时会查询这个注册表
	PROVIDERS = []ProviderSpec{
		{
			// OpenRouter - AI模型聚合网关服务
			Name:                  "openrouter",
			Keywords:              []string{"openrouter"},
			EnvKey:                "OPENROUTER_API_KEY",
			LiteLLMPrefix:         "openrouter/",
			IsGateway:             true,
			DefaultAPIBase:        "https://openrouter.ai/api/v1",
			SupportsPromptCaching: true,
		},
		{
			// Anthropic - Claude系列模型的提供商
			Name:                  "anthropic",
			Keywords:              []string{"anthropic", "claude"},
			EnvKey:                "ANTHROPIC_API_KEY",
			LiteLLMPrefix:         "anthropic/",
			SupportsPromptCaching: true,
		},
		{
			// OpenAI - GPT系列模型的提供商
			Name:          "openai",
			Keywords:      []string{"openai", "gpt"},
			EnvKey:        "OPENAI_API_KEY",
			LiteLLMPrefix: "openai/",
		},
		{
			// DeepSeek - 深度求索的大语言模型
			Name:           "deepseek",
			Keywords:       []string{"deepseek"},
			EnvKey:         "DEEPSEEK_API_KEY",
			LiteLLMPrefix:  "deepseek/",
			IsGateway:      true,
			DefaultAPIBase: "https://api.deepseek.com/v1",
		},
		{
			// Groq - 高性能推理服务提供商
			Name:           "groq",
			Keywords:       []string{"groq"},
			EnvKey:         "GROQ_API_KEY",
			LiteLLMPrefix:  "groq/",
			IsGateway:      true,
			DefaultAPIBase: "https://api.groq.com/openai/v1",
		},
		{
			// Gemini - Google的大语言模型
			Name:          "gemini",
			Keywords:      []string{"gemini", "google"},
			EnvKey:        "GEMINI_API_KEY",
			LiteLLMPrefix: "gemini/",
		},
		{
			// vLLM - 开源的高性能推理引擎
			Name:             "vllm",
			Keywords:         []string{"vllm"},
			EnvKey:           "OPENAI_API_KEY",
			IsGateway:        true,
			StripModelPrefix: true, // vLLM不需要模型前缀
		},
		{
			// Moonshot/Kimi - 月之暗面的大语言模型
			Name:          "moonshot",
			Keywords:      []string{"moonshot", "kimi"},
			EnvKey:        "MOONSHOT_API_KEY",
			LiteLLMPrefix: "moonshot/",
		},
		{
			// MiniMax - 稀宇科技的大语言模型
			Name:          "minimax",
			Keywords:      []string{"minimax"},
			EnvKey:        "MINIMAX_API_KEY",
			LiteLLMPrefix: "minimax/",
		},
		{
			// 智谱AI - GLM系列模型的提供商
			Name:          "zhipu",
			Keywords:      []string{"zhipu", "glm"},
			EnvKey:        "ZHIPU_API_KEY",
			LiteLLMPrefix: "zhipu/",
		},
		{
			// DashScope - 阿里云通义千问系列模型
			Name:          "dashscope",
			Keywords:      []string{"dashscope", "tongyi", "qwen"},
			EnvKey:        "DASHSCOPE_API_KEY",
			LiteLLMPrefix: "dashscope/",
		},
		{
			// SiliconFlow - 硅基流动的AI模型聚合服务
			Name:           "siliconflow",
			Keywords:       []string{"siliconflow", "silicon"},
			EnvKey:         "SILICONFLOW_API_KEY",
			IsGateway:      true,
			DefaultAPIBase: "https://api.siliconflow.cn/v1",
			LiteLLMPrefix:  "siliconflow/",
		},
		{
			// VolcEngine - 火山引擎（字节跳动）豆包系列模型
			Name:          "volcengine",
			Keywords:      []string{"volcengine", "doubao"},
			EnvKey:        "VOLCENGINE_API_KEY",
			LiteLLMPrefix: "volcengine/",
		},
		{
			// AIHubMix - AI模型聚合服务
			Name:           "aihubmix",
			Keywords:       []string{"aihubmix"},
			EnvKey:         "AIHUBMIX_API_KEY",
			IsGateway:      true,
			DefaultAPIBase: "https://api.aihubmix.com/v1",
			LiteLLMPrefix:  "aihubmix/",
		},
		{
			// OpenAI Codex - OpenAI的代码生成模型（OAuth认证）
			Name:          "openai_codex",
			Keywords:      []string{"openai_codex", "codex"},
			IsOAuth:       true,
			LiteLLMPrefix: "openai/",
		},
		{
			// GitHub Copilot - GitHub的AI编程助手（OAuth认证）
			Name:          "github_copilot",
			Keywords:      []string{"github-copilot", "copilot"},
			IsOAuth:       true,
			LiteLLMPrefix: "github/",
		},
		{
			// Custom - 自定义提供商，用于用户配置的自定义API端点
			Name:      "custom",
			Keywords:  []string{"custom"},
			IsGateway: true,
		},
	}
)

// FindByModel 根据模型名称查找对应的提供商规格
// 这个函数通过分析模型名称中的前缀和关键词来匹配提供商
// 匹配策略：
//  1. 优先匹配显式前缀（如"openai/gpt-4"中的"openai"）
//  2. 其次匹配关键词（如模型名中包含"claude"则匹配anthropic）
//
// 参数:
//
//	model: 模型名称，可以包含提供商前缀
//
// 返回:
//
//	匹配的提供商规格，如果未找到则返回nil
func FindByModel(model string) *ProviderSpec {
	modelLower := strings.ToLower(model)
	modelPrefix := ""
	// 提取模型前缀（"/"之前的部分）
	if idx := strings.Index(modelLower, "/"); idx > 0 {
		modelPrefix = modelLower[:idx]
	}

	for i, spec := range PROVIDERS {
		// 首先检查显式前缀
		// 例如："anthropic/claude-3" 会匹配 "anthropic" 提供商
		if modelPrefix != "" {
			// 标准化前缀，将"-"替换为"_"以支持不同的命名风格
			normalizedPrefix := strings.ReplaceAll(modelPrefix, "-", "_")
			if strings.ReplaceAll(spec.Name, "-", "_") == normalizedPrefix {
				return &PROVIDERS[i]
			}
		}

		// 检查关键词匹配
		// 例如："claude-3-opus"会通过关键词"claude"匹配到anthropic提供商
		for _, kw := range spec.Keywords {
			if strings.Contains(modelLower, strings.ReplaceAll(kw, "-", "_")) {
				return &PROVIDERS[i]
			}
		}
	}

	// 未找到匹配的提供商
	return nil
}

// FindByName 根据提供商名称查找规格
// 这个函数用于直接通过提供商名称获取配置
// 参数:
//
//	name: 提供商名称，如"openai"、"anthropic"等
//
// 返回:
//
//	匹配的提供商规格，如果未找到则返回nil
func FindByName(name string) *ProviderSpec {
	// 标准化名称，支持"-"和"_"的互换
	normalized := strings.ReplaceAll(name, "-", "_")
	for i, spec := range PROVIDERS {
		if strings.ReplaceAll(spec.Name, "-", "_") == normalized {
			return &PROVIDERS[i]
		}
	}
	return nil
}

// SetupEnv 为提供商设置环境变量
// 这个函数根据提供商规格配置所需的环境变量
// 某些LLM SDK（如LiteLLM）需要特定的环境变量才能正常工作
// 参数:
//
//	spec: 提供商规格
//	apiKey: API密钥
//	apiBase: API基础URL
func SetupEnv(spec *ProviderSpec, apiKey string, apiBase string) {
	if spec == nil || spec.EnvKey == "" {
		return
	}

	// 设置主要的API密钥环境变量（如果尚未设置）
	if _, exists := os.LookupEnv(spec.EnvKey); !exists {
		os.Setenv(spec.EnvKey, apiKey)
	}

	// 处理额外的环境变量配置
	// EnvExtras允许提供商定义额外需要的环境变量
	// 支持模板变量：{api_key}和{api_base}
	for _, extra := range spec.EnvExtras {
		if len(extra) >= 2 {
			envName := extra[0]
			envVal := strings.ReplaceAll(extra[1], "{api_key}", apiKey)
			envVal = strings.ReplaceAll(envVal, "{api_base}", apiBase)
			// 只设置尚未存在的环境变量，避免覆盖用户的自定义配置
			if _, exists := os.LookupEnv(envName); !exists {
				os.Setenv(envName, envVal)
			}
		}
	}
}
