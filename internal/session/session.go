// Package session 提供会话管理和持久化功能
//
// 会话系统负责管理对话历史，并使用 JSONL (JSON Lines) 格式将会话持久化到磁盘。
//
// JSONL 格式说明：
//   - 每行是一个独立的 JSON 对象
//   - 第一行是元数据对象，包含会话的基本信息
//   - 后续每行是一条消息
//
// JSONL 格式示例：
//
//	{"_type":"metadata","key":"chat-001","created_at":"2024-01-01T10:00:00Z",...}
//	{"role":"user","content":"Hello","timestamp":"2024-01-01T10:00:01Z"}
//	{"role":"assistant","content":"Hi there","timestamp":"2024-01-01T10:00:02Z"}
//
// JSONL 格式的优势：
//  1. 支持增量写入，不需要重写整个文件
//  2. 每行独立，即使文件损坏也只影响部分数据
//  3. 易于流式处理，适合大型会话历史
//  4. 人类可读，方便调试和分析
//
// 线程安全：
//   - Session 使用 sync.RWMutex 保护消息列表
//   - SessionManager 使用 sync.RWMutex 保护缓存 map
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Message 表示一条聊天消息
// 支持普通消息、工具调用(tool calls)和工具结果
type Message struct {
	Role       string     `json:"role"`                   // 消息角色: "user", "assistant", "system", "tool"
	Content    string     `json:"content"`                // 消息内容
	Timestamp  string     `json:"timestamp"`              // 消息时间戳（RFC3339 格式）
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // 助手调用的工具列表（仅 assistant 角色）
	ToolCallID string     `json:"tool_call_id,omitempty"` // 工具结果对应的调用 ID（仅 tool 角色）
	Name       string     `json:"name,omitempty"`         // 工具名称（仅 tool 角色）
}

// ToolCall 表示消息中的工具调用
// 当助手需要调用工具时，会在消息中包含工具调用信息
type ToolCall struct {
	ID       string       `json:"id"`       // 工具调用的唯一标识符
	Type     string       `json:"type"`     // 调用类型（通常是 "function"）
	Function FunctionCall `json:"function"` // 函数调用详情
}

// FunctionCall 表示函数调用的详细信息
type FunctionCall struct {
	Name      string `json:"name"`      // 函数名称
	Arguments string `json:"arguments"` // 函数参数（JSON 字符串格式）
}

// Session 表示一个对话会话
//
// Session 包含会话的所有消息和元数据，并提供线程安全的访问方法。
// 每个会话都有唯一的 Key，用于标识和持久化。
type Session struct {
	Key              string                 // 会话唯一标识符
	Messages         []Message              // 消息列表
	CreatedAt        time.Time              // 会话创建时间
	UpdatedAt        time.Time              // 会话最后更新时间
	Metadata         map[string]interface{} // 自定义元数据
	LastConsolidated int                    // 最后合并的消息索引（用于上下文压缩）
	mu               sync.RWMutex           // 读写锁，保护 Messages 列表
}

// NewSession 创建一个新的会话
//
// 参数：
//   - key: 会话唯一标识符
//
// 返回：
//   - *Session: 新创建的会话实例
func NewSession(key string) *Session {
	return &Session{
		Key:       key,
		Messages:  make([]Message, 0),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
}

// AddMessage 向会话添加一条新消息
//
// 该方法是线程安全的，使用互斥锁保护消息列表。
// 添加消息时会自动设置时间戳和更新会话的 UpdatedAt 字段。
//
// 参数：
//   - role: 消息角色（"user", "assistant", "system", "tool"）
//   - content: 消息内容
//   - toolCalls: 工具调用列表（可选，仅用于 assistant 角色）
func (s *Session) AddMessage(role, content string, toolCalls []ToolCall) {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg := Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}

	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
}

// GetHistory 获取最近的消息历史，以 LLM API 兼容的格式返回
//
// 该方法会将内部的 Message 结构转换为 map 格式，方便直接传递给 LLM API。
// 如果消息总数超过 maxMessages，只返回最近的 maxMessages 条消息。
//
// 参数：
//   - maxMessages: 要返回的最大消息数
//
// 返回：
//   - []map[string]interface{}: 消息列表，每条消息是一个 map，包含 role、content 等字段
func (s *Session) GetHistory(maxMessages int) []map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	start := 0
	if len(s.Messages) > maxMessages {
		start = len(s.Messages) - maxMessages
	}

	result := make([]map[string]interface{}, 0, len(s.Messages)-start)
	for _, m := range s.Messages[start:] {
		entry := map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		}

		if len(m.ToolCalls) > 0 {
			entry["tool_calls"] = m.ToolCalls
		}
		if m.ToolCallID != "" {
			entry["tool_call_id"] = m.ToolCallID
		}
		if m.Name != "" {
			entry["name"] = m.Name
		}

		result = append(result, entry)
	}

	return result
}

// Clear 清空会话的所有消息
//
// 该方法是线程安全的，会清空消息列表和重置合并索引。
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Messages = nil
	s.LastConsolidated = 0
	s.UpdatedAt = time.Now()
}

// SessionManager 管理对话会话的生命周期和持久化
//
// SessionManager 职责：
//  1. 管理会话缓存（LRU），避免重复加载
//  2. 从磁盘加载会话（JSONL 格式）
//  3. 将会话保存到磁盘（JSONL 格式）
//  4. 提供线程安全的会话访问
//
// LRU 缓存策略：
//   - 最大缓存数量：1000 个会话
//   - 超过限制时，删除最少使用的会话
//   - 访问会话时会更新其使用时间
//
// JSONL 持久化格式：
//   - 每个会话保存为一个 .jsonl 文件
//   - 文件名是会话 key 的安全文件名版本
//   - 第一行是元数据，后续每行是一条消息
type SessionManager struct {
	workspace   string              // 工作区根目录路径
	sessionsDir string              // 会话文件存储目录（workspace/sessions）
	cache       map[string]*Session // 会话缓存，键为会话 key
	cacheMu     sync.RWMutex        // 缓存读写锁
	maxCache    int                 // 最大缓存数量
	accessOrder []string            // 访问顺序，用于 LRU 淘汰
}

// NewSessionManager 创建一个新的会话管理器
//
// 参数：
//   - workspace: 工作区根目录路径
//
// 返回：
//   - *SessionManager: 会话管理器实例
func NewSessionManager(workspace string) *SessionManager {
	return &SessionManager{
		workspace:   workspace,
		sessionsDir: filepath.Join(workspace, "sessions"),
		cache:       make(map[string]*Session),
		maxCache:    1000, // 默认最大缓存 1000 个会话
		accessOrder: make([]string, 0, 100),
	}
}

// EnsureDir 确保目录存在，如果不存在则创建
//
// 参数：
//   - path: 目录路径
//
// 返回：
//   - error: 创建失败时返回错误
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// GetOrCreate 获取或创建会话
//
// LRU 工作流程：
//  1. 检查缓存，如果存在则更新访问顺序并返回
//  2. 尝试从磁盘加载会话（读取 JSONL 文件）
//  3. 如果磁盘上不存在，创建新会话
//  4. 如果缓存已满，淘汰最少使用的会话
//  5. 将会话加入缓存
//
// 该方法是线程安全的。
//
// 参数：
//   - key: 会话唯一标识符
//
// 返回：
//   - *Session: 会话实例
func (sm *SessionManager) GetOrCreate(key string) *Session {
	sm.cacheMu.RLock()
	if session, ok := sm.cache[key]; ok {
		// 更新访问顺序（将 key 移到末尾表示最近使用）
		sm.cacheMu.RUnlock()
		sm.cacheMu.Lock()
		sm.moveToEnd(key)
		sm.cacheMu.Unlock()
		return session
	}
	sm.cacheMu.RUnlock()

	// 尝试从磁盘加载
	session := sm.load(key)
	if session == nil {
		session = NewSession(key)
	}

	sm.cacheMu.Lock()
	// 检查缓存是否已满
	if len(sm.cache) >= sm.maxCache {
		// 淘汰最少使用的会话
		sm.evictOldest()
	}
	sm.cache[key] = session
	sm.accessOrder = append(sm.accessOrder, key)
	sm.cacheMu.Unlock()

	return session
}

// moveToEnd 将 key 移到访问顺序的末尾（表示最近使用）
func (sm *SessionManager) moveToEnd(key string) {
	for i, k := range sm.accessOrder {
		if k == key {
			// 移除当前位置
			sm.accessOrder = append(sm.accessOrder[:i], sm.accessOrder[i+1:]...)
			// 添加到末尾
			sm.accessOrder = append(sm.accessOrder, key)
			break
		}
	}
}

// evictOldest 淘汰最少使用的会话
func (sm *SessionManager) evictOldest() {
	if len(sm.accessOrder) == 0 {
		return
	}
	// 淘汰第一个（最旧）
	oldestKey := sm.accessOrder[0]
	sm.accessOrder = sm.accessOrder[1:]
	delete(sm.cache, oldestKey)
}

// load 从磁盘加载会话
//
// JSONL 文件解析过程：
//  1. 将会话 key 转换为安全文件名
//  2. 打开对应的 .jsonl 文件
//  3. 使用 json.Decoder 逐行解析
//  4. 第一行是元数据对象（_type="metadata"），提取会话基本信息
//  5. 后续每行是一条消息，添加到消息列表
//
// JSONL 格式示例：
//
//	{"_type":"metadata","key":"chat-001","created_at":"2024-01-01T10:00:00Z","metadata":{},"last_consolidated":0}
//	{"role":"user","content":"Hello","timestamp":"2024-01-01T10:00:01Z"}
//	{"role":"assistant","content":"Hi","timestamp":"2024-01-01T10:00:02Z"}
//
// 参数：
//   - key: 会话标识符
//
// 返回：
//   - *Session: 加载的会话，如果文件不存在或解析失败返回 nil
func (sm *SessionManager) load(key string) *Session {
	safeKey := safeFilename(key)
	path := filepath.Join(sm.sessionsDir, safeKey+".jsonl")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var messages []Message
	metadata := make(map[string]interface{})
	var createdAt time.Time
	lastConsolidated := 0

	decoder := json.NewDecoder(file)
	for decoder.More() {
		var data map[string]interface{}
		if err := decoder.Decode(&data); err != nil {
			continue
		}

		if data["_type"] == "metadata" {
			metadata = data["metadata"].(map[string]interface{})
			if createdAtStr, ok := data["created_at"].(string); ok {
				createdAt, _ = time.Parse(time.RFC3339, createdAtStr)
			}
			if lc, ok := data["last_consolidated"].(float64); ok {
				lastConsolidated = int(lc)
			}
		} else {
			var msg Message
			if b, err := json.Marshal(data); err == nil {
				json.Unmarshal(b, &msg)
			}
			messages = append(messages, msg)
		}
	}

	session := NewSession(key)
	session.Messages = messages
	session.Metadata = metadata
	session.LastConsolidated = lastConsolidated
	if !createdAt.IsZero() {
		session.CreatedAt = createdAt
	}

	return session
}

// Save 将会话保存到磁盘
//
// JSONL 文件写入过程：
//  1. 确保会话目录存在
//  2. 创建或覆盖 .jsonl 文件
//  3. 使用 json.Encoder 写入数据：
//     a. 首先写入元数据对象（第一行）
//     b. 然后逐条写入消息（每条消息一行）
//  4. 更新缓存
//
// JSONL 格式优势：
//   - 每行独立：文件损坏只影响部分数据
//   - 易于追加：可以高效地追加新消息（虽然当前实现是完全重写）
//   - 流式处理：可以逐行读取，不需要一次性加载整个文件
//
// 参数：
//   - session: 要保存的会话
//
// 返回：
//   - error: 保存失败时返回错误
func (sm *SessionManager) Save(session *Session) error {
	if err := EnsureDir(sm.sessionsDir); err != nil {
		return err
	}

	safeKey := safeFilename(session.Key)
	path := filepath.Join(sm.sessionsDir, safeKey+".jsonl")

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)

	// Write metadata
	metadata := map[string]interface{}{
		"_type":             "metadata",
		"key":               session.Key,
		"created_at":        session.CreatedAt.Format(time.RFC3339),
		"updated_at":        session.UpdatedAt.Format(time.RFC3339),
		"metadata":          session.Metadata,
		"last_consolidated": session.LastConsolidated,
	}
	if err := encoder.Encode(metadata); err != nil {
		return err
	}

	// Write messages
	session.mu.RLock()
	defer session.mu.RUnlock()
	for _, msg := range session.Messages {
		if err := encoder.Encode(msg); err != nil {
			return err
		}
	}

	sm.cacheMu.Lock()
	sm.cache[session.Key] = session
	sm.cacheMu.Unlock()

	return nil
}

// Invalidate 从缓存中移除会话
//
// 调用此方法后，下次 GetOrCreate 会从磁盘重新加载会话。
// 这在会话被外部修改时很有用。
//
// 参数：
//   - key: 会话标识符
func (sm *SessionManager) Invalidate(key string) {
	sm.cacheMu.Lock()
	delete(sm.cache, key)
	sm.cacheMu.Unlock()
}

// ListSessions 列出所有已保存的会话
//
// 该方法扫描会话目录，读取每个 .jsonl 文件的元数据行，
// 返回所有会话的基本信息（key、创建时间、更新时间、文件路径）。
//
// 返回：
//   - []map[string]interface{}: 会话信息列表，每个会话包含：
//   - key: 会话标识符
//   - created_at: 创建时间
//   - updated_at: 更新时间
//   - path: 文件路径
func (sm *SessionManager) ListSessions() []map[string]interface{} {
	entries, err := os.ReadDir(sm.sessionsDir)
	if err != nil {
		return nil
	}

	sessions := make([]map[string]interface{}, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}

		path := filepath.Join(sm.sessionsDir, entry.Name())
		file, err := os.Open(path)
		if err != nil {
			continue
		}

		var metadata map[string]interface{}
		decoder := json.NewDecoder(file)
		if decoder.More() {
			var data map[string]interface{}
			if err := decoder.Decode(&data); err == nil {
				if data["_type"] == "metadata" {
					metadata = data
				}
			}
		}
		file.Close()

		if metadata != nil {
			key := metadata["key"].(string)
			if key == "" {
				key = entry.Name()[:len(entry.Name())-len(".jsonl")]
			}
			sessions = append(sessions, map[string]interface{}{
				"key":        key,
				"created_at": metadata["created_at"],
				"updated_at": metadata["updated_at"],
				"path":       path,
			})
		}
	}

	return sessions
}

// safeFilename 将字符串转换为安全的文件名
//
// 转换规则：
//  1. 限制长度为 200 个字符
//  2. 保留字母、数字、连字符、下划线和点号
//  3. 其他字符替换为下划线
//
// 这确保会话 key 可以安全地用作文件名，避免文件系统问题。
//
// 参数：
//   - s: 原始字符串
//
// 返回：
//   - string: 安全的文件名
func safeFilename(s string) string {
	s = s[:min(len(s), 200)]
	result := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			result = append(result, r)
		case r >= 'A' && r <= 'Z':
			result = append(result, r)
		case r >= '0' && r <= '9':
			result = append(result, r)
		case r == '-' || r == '_' || r == '.':
			result = append(result, r)
		default:
			result = append(result, '_')
		}
	}
	return string(result)
}

// min 返回两个整数中的较小值
//
// 参数：
//   - a, b: 两个整数
//
// 返回：
//   - int: 较小的整数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
