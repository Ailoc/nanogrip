package providers

import (
	"context"
	"fmt"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicProvider uses Anthropic's official Go SDK.
type AnthropicProvider struct {
	client       anthropic.Client
	defaultModel string
}

func NewAnthropicProvider(apiKey string, apiBase string, defaultModel string) *AnthropicProvider {
	options := []option.RequestOption{
		option.WithRequestTimeout(120 * time.Second),
	}
	if apiKey != "" {
		options = append(options, option.WithAPIKey(apiKey))
	}
	if apiBase != "" {
		options = append(options, option.WithBaseURL(apiBase))
	}

	return &AnthropicProvider{
		client:       anthropic.NewClient(options...),
		defaultModel: defaultModel,
	}
}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []Message, tools []ToolDef, model string, maxTokens int, temperature float64) (*LLMResponse, error) {
	params, err := p.messageParams(messages, tools, model, maxTokens, temperature)
	if err != nil {
		return nil, err
	}

	message, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic message failed: %w", err)
	}

	return parseAnthropicResponse(message), nil
}

func (p *AnthropicProvider) ChatStream(ctx context.Context, messages []Message, tools []ToolDef, model string, maxTokens int, temperature float64, onDelta StreamCallback) (*LLMResponse, error) {
	params, err := p.messageParams(messages, tools, model, maxTokens, temperature)
	if err != nil {
		return nil, err
	}

	stream := p.client.Messages.NewStreaming(ctx, params)
	defer stream.Close()

	message := anthropic.Message{}
	for stream.Next() {
		event := stream.Current()
		if err := message.Accumulate(event); err != nil {
			return nil, fmt.Errorf("anthropic stream accumulation failed: %w", err)
		}

		if onDelta != nil {
			if delta := anthropicTextDelta(event); delta != "" {
				onDelta(delta)
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("anthropic message stream failed: %w", err)
	}

	return parseAnthropicResponse(&message), nil
}

func (p *AnthropicProvider) messageParams(messages []Message, tools []ToolDef, model string, maxTokens int, temperature float64) (anthropic.MessageNewParams, error) {
	apiModel, err := normalizeModelForProvider(ProviderAnthropic, model, p.defaultModel)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}
	if maxTokens <= 0 {
		maxTokens = 8192
	}

	anthropicMessages, systemPrompt, err := toAnthropicMessages(messages)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}

	params := anthropic.MessageNewParams{
		Model:       anthropic.Model(apiModel),
		MaxTokens:   int64(maxTokens),
		Messages:    anthropicMessages,
		System:      systemPrompt,
		Temperature: anthropic.Float(temperature),
	}
	if len(tools) > 0 {
		params.Tools = toAnthropicTools(tools)
		params.ToolChoice = anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{},
		}
	}

	return params, nil
}

func (p *AnthropicProvider) GetDefaultModel() string {
	return p.defaultModel
}

func anthropicTextDelta(event anthropic.MessageStreamEventUnion) string {
	contentDelta, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent)
	if !ok {
		return ""
	}
	textDelta, ok := contentDelta.Delta.AsAny().(anthropic.TextDelta)
	if !ok {
		return ""
	}
	return textDelta.Text
}

func toAnthropicMessages(messages []Message) ([]anthropic.MessageParam, []anthropic.TextBlockParam, error) {
	result := make([]anthropic.MessageParam, 0, len(messages))
	systemPrompt := make([]anthropic.TextBlockParam, 0)

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			if msg.Content != "" {
				systemPrompt = append(systemPrompt, anthropic.TextBlockParam{Text: msg.Content})
			}
		case "assistant":
			result = append(result, anthropic.NewAssistantMessage(anthropicAssistantBlocks(msg)...))
		case "tool":
			result = append(result, anthropic.NewUserMessage(anthropicToolResultBlock(msg)))
		default:
			blocks, err := anthropicUserBlocks(msg)
			if err != nil {
				return nil, nil, err
			}
			result = append(result, anthropic.NewUserMessage(blocks...))
		}
	}

	return result, systemPrompt, nil
}

func anthropicUserBlocks(msg Message) ([]anthropic.ContentBlockParamUnion, error) {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.Images)+1)
	if msg.Content != "" {
		blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
	}
	for _, image := range msg.Images {
		block, err := anthropicImageBlock(image)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	if len(blocks) == 0 {
		blocks = append(blocks, anthropic.NewTextBlock(""))
	}
	return blocks, nil
}

func anthropicAssistantBlocks(msg Message) []anthropic.ContentBlockParamUnion {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.Tools)+1)
	if msg.Content != "" {
		blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
	}
	for _, toolCall := range msg.Tools {
		blocks = append(blocks, anthropic.ContentBlockParamUnion{
			OfToolUse: &anthropic.ToolUseBlockParam{
				ID:    toolCall.ID,
				Name:  toolCall.Name,
				Input: cleanToolArguments(toolCall.Arguments),
			},
		})
	}
	if len(blocks) == 0 {
		blocks = append(blocks, anthropic.NewTextBlock(""))
	}
	return blocks
}

func anthropicToolResultBlock(msg Message) anthropic.ContentBlockParamUnion {
	return anthropic.ContentBlockParamUnion{
		OfToolResult: &anthropic.ToolResultBlockParam{
			ToolUseID: msg.ToolCallID,
			Content: []anthropic.ToolResultBlockParamContentUnion{
				{
					OfText: &anthropic.TextBlockParam{Text: msg.Content},
				},
			},
		},
	}
}

func anthropicImageBlock(image string) (anthropic.ContentBlockParamUnion, error) {
	if strings.HasPrefix(image, "data:") {
		mediaType, data, ok := parseImageDataURL(image)
		if !ok {
			return anthropic.ContentBlockParamUnion{}, fmt.Errorf("invalid image data URL")
		}
		return anthropic.NewImageBlockBase64(mediaType, data), nil
	}

	return anthropic.NewImageBlock(anthropic.URLImageSourceParam{URL: image}), nil
}

func parseImageDataURL(value string) (string, string, bool) {
	header, data, ok := strings.Cut(value, ",")
	if !ok || !strings.HasPrefix(header, "data:") || data == "" {
		return "", "", false
	}

	metadata := strings.TrimPrefix(header, "data:")
	parts := strings.Split(metadata, ";")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", false
	}

	hasBase64 := false
	for _, part := range parts[1:] {
		if part == "base64" {
			hasBase64 = true
			break
		}
	}
	if !hasBase64 {
		return "", "", false
	}

	return parts[0], data, true
}

func toAnthropicTools(tools []ToolDef) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		result = append(result, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        tool.Function.Name,
				Description: anthropic.String(tool.Function.Description),
				InputSchema: anthropicInputSchema(tool.Function.Parameters),
			},
		})
	}
	return result
}

func anthropicInputSchema(parameters map[string]interface{}) anthropic.ToolInputSchemaParam {
	schema := anthropic.ToolInputSchemaParam{}
	if len(parameters) == 0 {
		schema.Properties = map[string]interface{}{}
		return schema
	}

	extraFields := make(map[string]interface{})
	for key, value := range parameters {
		switch key {
		case "properties":
			schema.Properties = value
		case "required":
			schema.Required = toStringSlice(value)
		case "type":
		default:
			extraFields[key] = value
		}
	}
	if len(extraFields) > 0 {
		schema.ExtraFields = extraFields
	}
	if schema.Properties == nil {
		schema.Properties = map[string]interface{}{}
	}
	return schema
}

func parseAnthropicResponse(message *anthropic.Message) *LLMResponse {
	if message == nil {
		return &LLMResponse{
			Content:      "No response from Anthropic",
			FinishReason: "error",
		}
	}

	var contentParts []string
	var reasoningParts []string
	toolCalls := make([]ToolCallRequest, 0)

	for _, block := range message.Content {
		switch block.Type {
		case "text":
			contentParts = append(contentParts, block.Text)
		case "thinking":
			reasoningParts = append(reasoningParts, block.Thinking)
		case "tool_use":
			toolCalls = append(toolCalls, ToolCallRequest{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: parseToolArgumentsJSON(block.Input),
			})
		}
	}

	inputTokens := int(message.Usage.InputTokens + message.Usage.CacheCreationInputTokens + message.Usage.CacheReadInputTokens)
	outputTokens := int(message.Usage.OutputTokens)

	return &LLMResponse{
		Content:          strings.Join(contentParts, ""),
		ToolCalls:        toolCalls,
		FinishReason:     string(message.StopReason),
		Usage:            map[string]int{"prompt_tokens": inputTokens, "completion_tokens": outputTokens, "total_tokens": inputTokens + outputTokens},
		ReasoningContent: strings.Join(reasoningParts, ""),
	}
}
