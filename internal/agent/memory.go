// memory.go - 两层记忆系统
//
// 这个文件实现了 nanobot 的两层记忆架构：
//
// 第一层：MEMORY.md（长期记忆）
// - 存储重要的、结构化的信息
// - 由 Agent 主动维护和更新
// - 包含用户偏好、重要事实、项目信息等
// - 始终加载到系统提示词中
//
// 第二层：每日笔记（memory/YYYY-MM-DD.md）
// - 记录当天的对话和事件
// - 按日期组织，便于回顾
// - 可通过工具按需访问
//
// 记忆整理机制：
// - 当会话历史超过窗口大小时，旧消息可以被整理到 MEMORY.md
// - 使用 LLM 提炼重要信息，避免丢失关键上下文
package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ailoc/nanogrip/internal/providers"
)

// MemoryStore 提供两层记忆：MEMORY.md（长期）+ 每日笔记
// 这是 Agent 的"记忆系统"，允许它记住重要信息并回顾历史
type MemoryStore struct {
	memoryDir   string // 记忆目录路径
	memoryFile  string // MEMORY.md 文件路径（长期记忆）
	historyFile string // HISTORY.md 文件路径（历史日志）
}

// NewMemoryStore 创建一个新的记忆存储
// 它会自动创建记忆目录（如果不存在）
func NewMemoryStore(workspace string) *MemoryStore {
	memoryDir := filepath.Join(workspace, "memory")
	memoryFile := filepath.Join(memoryDir, "MEMORY.md")
	historyFile := filepath.Join(memoryDir, "HISTORY.md")

	// 确保目录存在
	os.MkdirAll(memoryDir, 0755)

	return &MemoryStore{
		memoryDir:   memoryDir,
		memoryFile:  memoryFile,
		historyFile: historyFile,
	}
}

// GetTodayFile 获取今天的笔记文件路径
// 格式：memory/YYYY-MM-DD.md
func (m *MemoryStore) GetTodayFile() string {
	today := time.Now().Format("2006-01-02")
	return filepath.Join(m.memoryDir, today+".md")
}

// ReadToday 读取今天的笔记
func (m *MemoryStore) ReadToday() string {
	todayFile := m.GetTodayFile()
	data, err := os.ReadFile(todayFile)
	if err != nil {
		return ""
	}
	return string(data)
}

// AppendToday 追加内容到今天的笔记
// 如果文件不存在，会创建文件并添加日期标题
func (m *MemoryStore) AppendToday(content string) error {
	todayFile := m.GetTodayFile()

	var newContent string
	existingData, err := os.ReadFile(todayFile)
	if err != nil {
		// 文件不存在，创建新文件
		header := "# " + time.Now().Format("2006-01-02") + "\n\n"
		newContent = header + content
	} else {
		// 文件存在，追加内容
		existing := string(existingData)
		existing = strings.TrimRight(existing, "\n")
		newContent = existing + "\n\n" + content
	}

	return os.WriteFile(todayFile, []byte(newContent), 0644)
}

// ReadLongTerm 读取长期记忆（MEMORY.md）
// 返回 MEMORY.md 的完整内容，如果文件不存在则返回空字符串
func (m *MemoryStore) ReadLongTerm() string {
	data, err := os.ReadFile(m.memoryFile)
	if err != nil {
		return ""
	}
	return string(data)
}

// WriteLongTerm 写入长期记忆（MEMORY.md）
// 这会覆盖整个 MEMORY.md 文件的内容
func (m *MemoryStore) WriteLongTerm(content string) error {
	return os.WriteFile(m.memoryFile, []byte(content), 0644)
}

// AppendHistory 追加条目到历史文件（HISTORY.md）
// 历史文件是一个只追加的日志，记录所有对话和事件
func (m *MemoryStore) AppendHistory(entry string) error {
	f, err := os.OpenFile(m.historyFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// 确保条目以双换行符结尾（便于 grep 搜索）
	entry = strings.TrimRight(entry, "\n")
	if !strings.HasSuffix(entry, "\n\n") {
		entry = entry + "\n\n"
	}
	_, err = f.WriteString(entry)
	return err
}

// GetRecentMemories 获取最近 N 天的笔记
// 参数：
//   - days: 回溯的天数
//
// 返回：
//   - 合并的笔记内容
func (m *MemoryStore) GetRecentMemories(days int) string {
	memories := []string{}
	today := time.Now()

	for i := 0; i < days; i++ {
		date := today.AddDate(0, 0, -i)
		dateStr := date.Format("2006-01-02")
		filePath := filepath.Join(m.memoryDir, dateStr+".md")

		if data, err := os.ReadFile(filePath); err == nil {
			memories = append(memories, string(data))
		}
	}

	return strings.Join(memories, "\n\n---\n\n")
}

// ListMemoryFiles 列出所有记忆文件（按日期排序，最新的在前）
func (m *MemoryStore) ListMemoryFiles() []string {
	entries, err := os.ReadDir(m.memoryDir)
	if err != nil {
		return []string{}
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			files = append(files, entry.Name())
		}
	}

	// 按日期排序（倒序）
	for i := 0; i < len(files)-1; i++ {
		for j := i + 1; j < len(files); j++ {
			if files[j] > files[i] {
				files[i], files[j] = files[j], files[i]
			}
		}
	}

	return files
}

// GetMemoryContext 返回系统提示词中的记忆上下文
// 包含长期记忆和今天的笔记
func (m *MemoryStore) GetMemoryContext() string {
	var parts []string

	// 长期记忆
	longTerm := m.ReadLongTerm()
	if longTerm != "" {
		parts = append(parts, "## Long-term Memory\n"+longTerm)
	}

	// 今天的笔记
	today := m.ReadToday()
	if today != "" {
		parts = append(parts, "## Today's Notes\n"+today)
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "\n\n")
}

// SessionMessage 表示会话历史中的单个消息
type SessionMessage struct {
	Role      string   // 角色（user、assistant、tool）
	Content   string   // 消息内容
	Timestamp string   // 时间戳
	ToolsUsed []string // 使用的工具列表
}

// ConsolidateOldMessages 将旧消息整理到记忆中
// 当会话历史过长时，这个方法会：
// 1. 识别超出记忆窗口的旧消息
// 2. 使用 LLM 提炼这些消息中的重要信息
// 3. 将提炼的信息更新到 MEMORY.md
// 4. 返回精简后的消息列表
//
// 参数：
//   - messages: 会话消息列表
//   - provider: LLM 提供商（用于提炼信息）
//   - model: 使用的 LLM 模型
//   - memoryWindow: 记忆窗口大小
//   - lastConsolidated: 上次整理到的位置
//
// 整理策略：
// - 保留最近的 memoryWindow/2 条消息
// - 将更旧的消息通过 LLM 提炼成摘要
// - 更新 MEMORY.md 而不是保留原始对话
func (m *MemoryStore) ConsolidateOldMessages(
	messages []SessionMessage,
	provider providers.LLMProvider,
	model string,
	memoryWindow int,
	lastConsolidated int,
) ([]SessionMessage, error) {
	keepCount := memoryWindow / 2
	if len(messages) <= keepCount {
		return messages, nil
	}

	// 获取需要整理的消息
	if lastConsolidated >= len(messages)-keepCount {
		return messages, nil
	}

	oldMessages := messages[lastConsolidated : len(messages)-keepCount]
	if len(oldMessages) == 0 {
		return messages, nil
	}

	// 构建对话文本
	var lines []string
	for _, msg := range oldMessages {
		if msg.Content == "" {
			continue
		}
		toolsUsed := ""
		if len(msg.ToolsUsed) > 0 {
			toolsUsed = " [tools: " + strings.Join(msg.ToolsUsed, ", ") + "]"
		}
		timestamp := msg.Timestamp
		if len(timestamp) > 16 {
			timestamp = timestamp[:16]
		}
		lines = append(lines, fmt.Sprintf("[%s] %s%s: %s", timestamp, msg.Role, toolsUsed, msg.Content))
	}

	// 读取当前长期记忆
	currentMemory := m.ReadLongTerm()
	if currentMemory == "" {
		currentMemory = "(empty)"
	}

	// 构建提示词，要求 LLM 整理记忆
	prompt := fmt.Sprintf(`Process this conversation and call the save_memory tool with your consolidation.

## Current Long-term Memory
%s

## Conversation to Process
%s`, currentMemory, strings.Join(lines, "\n"))

	// 注意：完整实现需要调用 LLM 并执行 save_memory 工具
	_ = prompt
	fmt.Printf("Would consolidate %d messages\n", len(oldMessages))

	return messages, nil
}
