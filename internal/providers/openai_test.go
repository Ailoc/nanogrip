package providers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToOpenAIMessagesKeepsEmptyAssistantContent(t *testing.T) {
	messages, err := toOpenAIMessages([]Message{
		{
			Role:    "assistant",
			Content: "",
			Tools: []ToolCallRequest{
				{
					ID:        "call_123",
					Name:      "save_memory",
					Arguments: map[string]interface{}{"memory_update": "test"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}

	payload := string(data)
	if !strings.Contains(payload, `"content":""`) {
		t.Fatalf("expected empty assistant content to be serialized, got %s", payload)
	}
	if !strings.Contains(payload, `"tool_calls"`) {
		t.Fatalf("expected tool calls to be serialized, got %s", payload)
	}
}
