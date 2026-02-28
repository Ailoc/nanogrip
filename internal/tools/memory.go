package tools

import (
	"context"
	"fmt"
)

// SaveMemoryTool 允许 Agent 保存记忆整理结果
// 这个工具由记忆整理进程调用，用于将提炼的信息保存到 MEMORY.md 和 HISTORY.md
type SaveMemoryTool struct {
	BaseTool
	// memoryStore 用于读取和写入记忆
	memoryStore MemoryStoreInterface
}

// MemoryStoreInterface 定义记忆存储的接口
type MemoryStoreInterface interface {
	ReadLongTerm() string
	WriteLongTerm(content string) error
	AppendHistory(entry string) error
}

// NewSaveMemoryTool 创建一个新的保存记忆工具
func NewSaveMemoryTool(memoryStore MemoryStoreInterface) *SaveMemoryTool {
	return &SaveMemoryTool{
		BaseTool: NewBaseTool(
			"save_memory",
			"Save the memory consolidation result to persistent storage. Call this after processing conversation history to update long-term memory and append to history.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"history_entry": map[string]interface{}{
						"type":        "string",
						"description": "A paragraph (2-5 sentences) summarizing key events/decisions/topics. Start with [YYYY-MM-DD HH:MM]. Include detail useful for grep search.",
					},
					"memory_update": map[string]interface{}{
						"type":        "string",
						"description": "Full updated long-term memory as markdown. Include all existing facts plus new ones. Return unchanged if nothing new.",
					},
				},
				"required": []string{"history_entry", "memory_update"},
			},
		),
		memoryStore: memoryStore,
	}
}

// Execute 执行保存记忆
// 将历史条目追加到 HISTORY.md，并更新 MEMORY.md
func (t *SaveMemoryTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	historyEntry, _ := params["history_entry"].(string)
	memoryUpdate, _ := params["memory_update"].(string)

	if historyEntry == "" && memoryUpdate == "" {
		return "Error: history_entry and memory_update are required", nil
	}

	// 追加历史条目到 HISTORY.md
	if historyEntry != "" {
		if err := t.memoryStore.AppendHistory(historyEntry); err != nil {
			return fmt.Sprintf("Error saving history: %v", err), nil
		}
	}

	// 更新长期记忆 MEMORY.md
	if memoryUpdate != "" {
		currentMemory := t.memoryStore.ReadLongTerm()
		// 只有当新内容与当前内容不同时才更新
		if memoryUpdate != currentMemory {
			if err := t.memoryStore.WriteLongTerm(memoryUpdate); err != nil {
				return fmt.Sprintf("Error saving memory: %v", err), nil
			}
		}
	}

	return "Memory saved successfully", nil
}
