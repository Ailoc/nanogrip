package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

// OpenAIProvider uses OpenAI's official Go SDK.
type OpenAIProvider struct {
	client       openai.Client
	defaultModel string
}

func NewOpenAIProvider(apiKey string, apiBase string, defaultModel string) *OpenAIProvider {
	options := []option.RequestOption{
		option.WithRequestTimeout(120 * time.Second),
	}
	if apiKey != "" {
		options = append(options, option.WithAPIKey(apiKey))
	}
	if apiBase != "" {
		options = append(options, option.WithBaseURL(apiBase))
	}

	return &OpenAIProvider{
		client:       openai.NewClient(options...),
		defaultModel: defaultModel,
	}
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message, tools []ToolDef, model string, maxTokens int, temperature float64) (*LLMResponse, error) {
	params, err := p.chatCompletionParams(messages, tools, model, maxTokens, temperature)
	if err != nil {
		return nil, err
	}

	completion, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai chat completion failed: %w", err)
	}

	return parseOpenAIResponse(completion), nil
}

func (p *OpenAIProvider) ChatStream(ctx context.Context, messages []Message, tools []ToolDef, model string, maxTokens int, temperature float64, onDelta StreamCallback) (*LLMResponse, error) {
	params, err := p.chatCompletionParams(messages, tools, model, maxTokens, temperature)
	if err != nil {
		return nil, err
	}

	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()

	acc := openai.ChatCompletionAccumulator{}
	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		if onDelta != nil && len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta.Content
			if delta == "" {
				delta = chunk.Choices[0].Delta.Refusal
			}
			if delta != "" {
				onDelta(delta)
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("openai chat completion stream failed: %w", err)
	}

	return parseOpenAIResponse(&acc.ChatCompletion), nil
}

func (p *OpenAIProvider) chatCompletionParams(messages []Message, tools []ToolDef, model string, maxTokens int, temperature float64) (openai.ChatCompletionNewParams, error) {
	apiModel, err := normalizeModelForProvider(ProviderOpenAI, model, p.defaultModel)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}

	openAIMessages, err := toOpenAIMessages(messages)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}

	params := openai.ChatCompletionNewParams{
		Model:       shared.ChatModel(apiModel),
		Messages:    openAIMessages,
		Temperature: openai.Float(temperature),
	}
	if maxTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(maxTokens))
	}
	if len(tools) > 0 {
		params.Tools = toOpenAITools(tools)
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.String("auto"),
		}
	}

	return params, nil
}

func (p *OpenAIProvider) GetDefaultModel() string {
	return p.defaultModel
}

func toOpenAIMessages(messages []Message) ([]openai.ChatCompletionMessageParamUnion, error) {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			result = append(result, openai.SystemMessage(msg.Content))
		case "assistant":
			assistant := openai.ChatCompletionAssistantMessageParam{}
			assistant.Content.OfString = openai.String(msg.Content)
			if len(msg.Tools) > 0 {
				assistant.ToolCalls = make([]openai.ChatCompletionMessageToolCallParam, 0, len(msg.Tools))
				for _, toolCall := range msg.Tools {
					assistant.ToolCalls = append(assistant.ToolCalls, openai.ChatCompletionMessageToolCallParam{
						ID: toolCall.ID,
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      toolCall.Name,
							Arguments: ToolArgumentsJSON(toolCall.Arguments),
						},
					})
				}
			}
			result = append(result, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant})
		case "tool":
			result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
		default:
			result = append(result, openaiUserMessage(msg))
		}
	}
	return result, nil
}

func openaiUserMessage(msg Message) openai.ChatCompletionMessageParamUnion {
	if len(msg.Images) == 0 {
		return openai.UserMessage(msg.Content)
	}

	parts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(msg.Images)+1)
	if msg.Content != "" {
		parts = append(parts, openai.TextContentPart(msg.Content))
	}
	for _, image := range msg.Images {
		parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
			URL: image,
		}))
	}
	return openai.UserMessage(parts)
}

func toOpenAITools(tools []ToolDef) []openai.ChatCompletionToolParam {
	result := make([]openai.ChatCompletionToolParam, 0, len(tools))
	for _, tool := range tools {
		result = append(result, openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        tool.Function.Name,
				Description: openai.String(tool.Function.Description),
				Parameters:  shared.FunctionParameters(tool.Function.Parameters),
			},
		})
	}
	return result
}

func parseOpenAIResponse(completion *openai.ChatCompletion) *LLMResponse {
	if completion == nil || len(completion.Choices) == 0 {
		return &LLMResponse{
			Content:      "No response from OpenAI",
			FinishReason: "error",
		}
	}

	choice := completion.Choices[0]
	message := choice.Message
	content := message.Content
	if content == "" && message.Refusal != "" {
		content = message.Refusal
	}

	toolCalls := make([]ToolCallRequest, 0, len(message.ToolCalls))
	for _, toolCall := range message.ToolCalls {
		toolCalls = append(toolCalls, ToolCallRequest{
			ID:        toolCall.ID,
			Name:      toolCall.Function.Name,
			Arguments: parseToolArgumentsRaw(toolCall.Function.Arguments),
		})
	}

	usage := map[string]int{
		"prompt_tokens":     int(completion.Usage.PromptTokens),
		"completion_tokens": int(completion.Usage.CompletionTokens),
		"total_tokens":      int(completion.Usage.TotalTokens),
	}

	return &LLMResponse{
		Content:      content,
		ToolCalls:    toolCalls,
		FinishReason: choice.FinishReason,
		Usage:        usage,
	}
}
