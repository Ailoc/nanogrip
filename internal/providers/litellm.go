// Package providers 提供了LLM提供商的具体实现
// 本文件实现了基于OpenAI兼容API的LiteLLM提供商
// LiteLLM是一个统一的接口，可以调用多个不同的LLM提供商
package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// LiteLLMProvider 是使用OpenAI兼容API的LLM提供商
// 这个提供商实现了统一的接口来调用各种支持OpenAI API格式的模型服务
// 包括OpenAI、Anthropic、DeepSeek、OpenRouter等
type LiteLLMProvider struct {
	apiKey       string            // API密钥，用于身份验证
	apiBase      string            // API基础URL，如果为空则根据模型自动推断
	defaultModel string            // 默认使用的模型名称
	extraHeaders map[string]string // 额外的HTTP请求头，用于特殊配置
	httpClient   *http.Client      // HTTP客户端，配置了超时等参数
}

// NewLiteLLMProvider 创建一个新的LiteLLM提供商实例
// 参数:
//
//	apiKey: API密钥，用于身份验证
//	apiBase: API基础URL，如果为空则根据模型自动推断
//	defaultModel: 默认模型名称
//	extraHeaders: 额外的HTTP请求头
//
// 返回:
//
//	配置好的LiteLLMProvider实例
func NewLiteLLMProvider(apiKey string, apiBase string, defaultModel string, extraHeaders map[string]string) *LiteLLMProvider {
	return &LiteLLMProvider{
		apiKey:       apiKey,
		apiBase:      apiBase,
		defaultModel: defaultModel,
		extraHeaders: extraHeaders,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // 设置120秒超时，适用于大多数LLM请求
		},
	}
}

// Chat 发送聊天完成请求到LLM服务
// 这是LLMProvider接口的核心实现方法
// 参数:
//
//	ctx: 上下文，用于控制请求生命周期
//	messages: 对话消息历史
//	tools: 可用的工具定义列表
//	model: 要使用的模型名称，如果为空则使用默认模型
//	maxTokens: 生成的最大token数
//	temperature: 温度参数，控制生成的随机性
//
// 返回:
//
//	LLM的响应和可能的错误
func (p *LiteLLMProvider) Chat(ctx context.Context, messages []Message, tools []ToolDef, model string, maxTokens int, temperature float64) (*LLMResponse, error) {
	// 如果未指定模型，使用默认模型
	if model == "" {
		model = p.defaultModel
	}

	// 解析模型名称，添加必要的提供商前缀
	model = p.resolveModel(model)

	// 准备请求体，构建符合OpenAI API格式的请求
	reqBody := map[string]interface{}{
		"model":       model,
		"messages":    p.sanitizeMessages(messages),
		"max_tokens":  maxTokens,
		"temperature": temperature,
	}

	// 如果提供了工具定义，添加到请求中
	// tool_choice设置为"auto"表示让模型自动决定是否调用工具
	if len(tools) > 0 {
		reqBody["tools"] = tools
		reqBody["tool_choice"] = "auto"
	}

	// 确定API基础URL
	// 优先级：配置的apiBase > 提供商规格的默认URL > OpenAI默认URL
	apiBase := p.apiBase
	if apiBase == "" {
		spec := FindByModel(model)
		// 如果是网关类型的提供商且有默认API地址，使用它
		if spec != nil && spec.IsGateway && spec.DefaultAPIBase != "" {
			apiBase = spec.DefaultAPIBase
		}
	}

	// 如果仍然没有API地址，使用OpenAI的默认地址作为后备
	if apiBase == "" {
		apiBase = "https://api.openai.com/v1"
	}

	// 构建完整的请求URL
	url := apiBase
	// 检查URL是否已经包含完整路径，避免重复添加
	if !strings.Contains(url, "chat/completions") {
		if !strings.HasSuffix(url, "/") {
			url += "/"
		}
		url += "chat/completions"
	}

	// 将请求体序列化为JSON
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建HTTP请求，绑定上下文以支持取消和超时
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	// 设置授权头，所有OpenAI兼容API都使用Bearer token格式
	// 注意：这里的多个if条件实际上做的是同样的事情，保留是为了未来可能的定制
	if p.apiKey != "" {
		if strings.Contains(apiBase, "openrouter") {
			req.Header.Set("Authorization", "Bearer "+p.apiKey)
		} else if strings.Contains(apiBase, "api.deepseek") {
			req.Header.Set("Authorization", "Bearer "+p.apiKey)
		} else {
			req.Header.Set("Authorization", "Bearer "+p.apiKey)
		}
	}

	// 设置额外的自定义请求头
	for k, v := range p.extraHeaders {
		req.Header.Set(k, v)
	}

	// 发送HTTP请求
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 检查HTTP状态码，非200表示请求失败
	if resp.StatusCode != http.StatusOK {
		// 返回错误信息，但不抛出error，让调用方可以看到错误内容
		return &LLMResponse{
			Content:      fmt.Sprintf("Error: HTTP %d - %s", resp.StatusCode, string(respBody)),
			FinishReason: "error",
		}, nil
	}

	// 解析JSON响应
	var completionResp map[string]interface{}
	if err := json.Unmarshal(respBody, &completionResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// 解析并构造LLMResponse对象
	return p.parseResponse(completionResp)
}

// resolveModel 解析模型名称，根据提供商规格添加或移除前缀
// 这个方法处理不同提供商的命名规则，确保模型名称格式正确
// 参数:
//
//	model: 原始模型名称
//
// 返回:
//
//	解析后的模型名称
func (p *LiteLLMProvider) resolveModel(model string) string {
	// 查找模型对应的提供商规格
	spec := FindByModel(model)
	if spec == nil {
		return model
	}

	// 当明确提供了API基础URL时（自定义提供商），直接使用模型名称
	// 用户直接调用特定的API端点，不需要LiteLLM的前缀
	if p.apiBase != "" {
		// 对于网关模式且明确指定基础URL的情况，如果需要则移除前缀
		if spec.StripModelPrefix {
			if idx := strings.LastIndex(model, "/"); idx > 0 {
				model = model[idx+1:]
			}
		}
		// 使用自定义API地址时不添加前缀，用户知道自己在做什么
		return model
	}

	// 对于标准模式（没有明确的API基础URL）
	// 需要添加LiteLLM的提供商前缀，如"anthropic/"、"openai/"等
	if spec.LiteLLMPrefix != "" && !strings.HasPrefix(model, spec.LiteLLMPrefix) {
		// 检查是否已经有明确的前缀
		if idx := strings.Index(model, "/"); idx > 0 {
			prefix := model[:idx]
			// 如果前缀不匹配提供商名称，添加正确的前缀
			if strings.ToLower(prefix) != spec.Name {
				model = spec.LiteLLMPrefix + model
			}
		} else {
			// 没有前缀，直接添加
			model = spec.LiteLLMPrefix + model
		}
	}

	return model
}

// sanitizeMessages 清理消息，移除非标准字段并转换为API所需格式
// 这个方法将内部的Message结构转换为符合OpenAI API规范的格式
// 参数:
//
//	messages: 原始消息列表
//
// 返回:
//
//	清理并格式化后的消息列表
func (p *LiteLLMProvider) sanitizeMessages(messages []Message) []map[string]interface{} {
	result := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		clean := make(map[string]interface{})
		// 基本字段：角色
		clean["role"] = msg.Role

		// 处理内容：如果有图片，构建多部分内容
		if len(msg.Images) > 0 {
			// 构建多部分内容：文本 + 图片
			contentParts := make([]interface{}, 0)

			// 添加文本部分（如果存在）
			if msg.Content != "" {
				contentParts = append(contentParts, map[string]interface{}{
					"type": "text",
					"text": msg.Content,
				})
			}

			// 添加图片部分
			for _, img := range msg.Images {
				var imageContent map[string]interface{}
				if strings.HasPrefix(img, "data:") {
					// base64 数据 URL - 提取 MIME 类型
					imageContent = map[string]interface{}{
						"type":      "image_url",
						"image_url": map[string]interface{}{"url": img},
					}
				} else {
					// 普通 URL
					imageContent = map[string]interface{}{
						"type":      "image_url",
						"image_url": map[string]interface{}{"url": img},
					}
				}
				contentParts = append(contentParts, imageContent)
			}

			clean["content"] = contentParts
		} else {
			// 没有图片，使用普通文本内容
			clean["content"] = msg.Content
		}

		// 如果消息包含工具调用（通常是assistant角色的消息）
		if len(msg.Tools) > 0 {
			toolCalls := make([]map[string]interface{}, len(msg.Tools))
			for j, tc := range msg.Tools {
				// 构造符合OpenAI格式的tool_call对象
				toolCalls[j] = map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]string{
						"name":      tc.Name,
						"arguments": tc.Arguments["_raw"].(string), // 使用原始JSON字符串
					},
				}
			}
			clean["tool_calls"] = toolCalls
		}

		// 如果是工具响应消息，需要包含tool_call_id
		if msg.ToolCallID != "" {
			clean["tool_call_id"] = msg.ToolCallID
		}

		// 如果有名称字段（用于工具响应）
		if msg.Name != "" {
			clean["name"] = msg.Name
		}

		result[i] = clean
	}
	return result
}

// parseResponse 解析API响应，提取各种信息构造LLMResponse对象
// 这个方法处理OpenAI格式的API响应，提取内容、工具调用、使用统计等
// 参数:
//
//	resp: API返回的JSON响应（已解析为map）
//
// 返回:
//
//	结构化的LLMResponse对象和可能的错误
func (p *LiteLLMProvider) parseResponse(resp map[string]interface{}) (*LLMResponse, error) {
	// 提取choices数组，这是OpenAI API响应的标准格式
	choices, ok := resp["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return &LLMResponse{
			Content:      "No response from model",
			FinishReason: "error",
		}, nil
	}

	// 获取第一个choice（通常只有一个）
	choice := choices[0].(map[string]interface{})
	message := choice["message"].(map[string]interface{})

	// 提取消息内容
	content, _ := message["content"].(string)
	// 提取推理内容（某些模型如DeepSeek支持）
	reasoningContent, _ := message["reasoning_content"].(string)

	// 移除内容中的thinking标签块
	content = stripThink(content)

	// 获取完成原因（stop表示正常结束，tool_calls表示需要调用工具）
	finishReason, _ := choice["finish_reason"].(string)

	// 解析工具调用
	// 如果LLM决定调用工具，响应中会包含tool_calls数组
	var toolCalls []ToolCallRequest
	if tc, ok := message["tool_calls"].([]interface{}); ok {
		toolCalls = make([]ToolCallRequest, len(tc))
		for i, tcRaw := range tc {
			tcMap := tcRaw.(map[string]interface{})
			funcMap := tcMap["function"].(map[string]interface{})

			name, _ := funcMap["name"].(string)
			argsStr, _ := funcMap["arguments"].(string)

			// 解析参数JSON字符串为map
			args := make(map[string]interface{})
			if argsStr != "" {
				json.Unmarshal([]byte(argsStr), &args)
			}
			// 保留原始JSON字符串，某些情况下需要
			args["_raw"] = argsStr

			toolCalls[i] = ToolCallRequest{
				ID:        tcMap["id"].(string),
				Name:      name,
				Arguments: args,
			}
		}
	}

	// 解析token使用统计
	// 这对于计费和监控很重要
	usage := make(map[string]int)
	if usageRaw, ok := resp["usage"].(map[string]interface{}); ok {
		if v, ok := usageRaw["prompt_tokens"].(float64); ok {
			usage["prompt_tokens"] = int(v)
		}
		if v, ok := usageRaw["completion_tokens"].(float64); ok {
			usage["completion_tokens"] = int(v)
		}
		if v, ok := usageRaw["total_tokens"].(float64); ok {
			usage["total_tokens"] = int(v)
		}
	}

	// 构造并返回完整的响应对象
	return &LLMResponse{
		Content:          content,
		ToolCalls:        toolCalls,
		FinishReason:     finishReason,
		Usage:            usage,
		ReasoningContent: reasoningContent,
	}, nil
}

// stripThink 从内容中移除thinking块
// 某些模型（如DeepSeek）会在响应中包含<think>标签来显示思考过程
// 这个方法移除这些标签，只保留最终的回答内容
// 参数:
//
//	text: 原始文本内容
//
// 返回:
//
//	移除thinking块后的内容
func stripThink(text string) string {
	re := regexp.MustCompile(`<think>[\s\S]*?</think>`)
	return re.ReplaceAllString(text, "")
}

// GetDefaultModel 返回默认模型名称
// 实现LLMProvider接口的方法
// 返回:
//
//	配置的默认模型名称
func (p *LiteLLMProvider) GetDefaultModel() string {
	return p.defaultModel
}

// SetupProviderEnv 为提供商设置环境变量
// 这是一个便捷函数，用于根据模型名称自动配置环境变量
// 某些LLM SDK需要特定的环境变量来工作
// 参数:
//
//	apiKey: API密钥
//	apiBase: API基础URL
//	model: 模型名称，用于识别提供商
func SetupProviderEnv(apiKey string, apiBase string, model string) {
	spec := FindByModel(model)
	if spec != nil {
		SetupEnv(spec, apiKey, apiBase)
	}
}
