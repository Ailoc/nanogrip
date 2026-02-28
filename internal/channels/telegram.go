// Package channels - Telegram频道实现
// telegram.go 实现了Telegram Bot API的集成
// 通过HTTP长轮询（Long Polling）接收消息，通过REST API发送消息
// 支持Markdown到HTML的转换，支持长消息自动分割
package channels

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Ailoc/nanogrip/internal/bus"
	"github.com/Ailoc/nanogrip/internal/config"
)

// TelegramChannel Telegram机器人频道实现
// 主要特性：
// 1. 使用getUpdates API进行长轮询接收消息
// 2. 使用sendMessage API发送消息，支持HTML格式
// 3. 支持用户白名单（AllowFrom）进行访问控制
// 4. 支持Markdown到HTML的自动转换
// 5. 自动分割超长消息（最大4000字符）
// 6. 支持代理配置
// 7. 支持交互式输入处理（当 Shell 命令需要用户输入时）
type TelegramChannel struct {
	*BaseChannel                        // 嵌入基础频道，继承通用功能
	config       *config.TelegramConfig // Telegram配置
	token        string                 // Bot Token，用于API认证
	allowFrom    map[string]bool       // 用户白名单，key为用户ID，value始终为true
	httpClient   *http.Client          // HTTP客户端，用于调用Telegram API
	chatIDs      map[string]int64      // 用户ID到聊天ID的映射，用于回复消息
	mu           sync.RWMutex          // 读写锁，保护chatIDs的并发访问
	updateID     int64                  // 当前已处理的最大update_id，用于增量获取消息
	inputHandler func(channel, chatID, input string) bool // 输入处理回调，用于交互式输入
}

// SetInputHandler 设置输入处理回调
// 这个回调用于处理交互式输入，当 Shell 命令等待用户输入时，用户的消息会通过这个回调处理
func (c *TelegramChannel) SetInputHandler(handler func(channel, chatID, input string) bool) {
	c.inputHandler = handler
}

// NewTelegramChannel 创建一个新的Telegram频道实例
// 参数:
//
//	cfg: Telegram配置对象，包含token、白名单等信息
//	bus: 消息总线，用于发布接收到的消息和订阅待发送消息
//
// 返回: 初始化后的TelegramChannel指针
func NewTelegramChannel(cfg *config.TelegramConfig, bus *bus.MessageBus) *TelegramChannel {
	// 将白名单列表转换为map，提高查找效率
	allowFrom := make(map[string]bool)
	for _, id := range cfg.AllowFrom {
		allowFrom[id] = true
	}

	// 创建 HTTP 客户端，支持代理
	httpClient := &http.Client{
		Timeout: 90 * time.Second, // 长轮询超时
	}

	// 如果配置了代理，创建支持代理的 transport
	if cfg.Proxy != "" {
		proxyURL := cfg.Proxy
		if !strings.HasPrefix(proxyURL, "http://") && !strings.HasPrefix(proxyURL, "https://") && !strings.HasPrefix(proxyURL, "socks5://") {
			proxyURL = "http://" + proxyURL
		}

		transport := &http.Transport{}
		if proxyURL, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
			httpClient.Transport = transport
			log.Printf("Telegram channel using proxy: %s", proxyURL)
		}
	}

	return &TelegramChannel{
		BaseChannel: NewBaseChannel("telegram", cfg, bus),
		config:      cfg,
		token:       cfg.Token,
		allowFrom:   allowFrom,
		httpClient:  httpClient,
		chatIDs:     make(map[string]int64),
	}
}

// Start 启动Telegram机器人服务
// 启动流程：
// 1. 检查Token是否配置
// 2. 设置running状态为true
// 3. 启动消息轮询goroutine，接收Telegram消息
// 参数:
//
//	ctx: 上下文对象，用于控制轮询的生命周期
//
// 返回: 如果Token未配置则返回错误，否则返回nil
func (c *TelegramChannel) Start(ctx context.Context) error {
	if c.token == "" {
		return fmt.Errorf("Telegram bot token not configured")
	}

	c.running = true

	// 启动消息轮询goroutine
	go c.pollUpdates(ctx)
	log.Println("Telegram channel started")

	return nil
}

// Stop 停止Telegram机器人服务
// 设置running标志为false，轮询goroutine会自动退出
// 返回: 始终返回nil
func (c *TelegramChannel) Stop() error {
	c.running = false
	log.Println("Telegram channel stopped")
	return nil
}

// Send 通过Telegram发送消息
// 发送流程：
// 1. 将ChatID字符串转换为int64类型
// 2. 如果有媒体文件，发送媒体并附带说明文字
// 3. 如果没有媒体但有文字内容，发送文字消息
// 4. 如果消息超过4000字符，自动分割为多条消息
// 5. 逐条发送消息
// 参数:
//
//	msg: 出站消息对象，包含接收者ChatID、消息内容和媒体文件
//
// 返回: 发送失败时返回错误
func (c *TelegramChannel) Send(msg bus.OutboundMessage) error {
	// Telegram的chat_id是int64类型，需要转换
	chatID, err := strconv.ParseInt(msg.ChatID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat_id: %w", err)
	}

	// 检查是否有媒体文件需要发送
	if len(msg.Media) > 0 {
		// 发送媒体文件（图片、视频、文档等）
		for _, mediaURL := range msg.Media {
			if err := c.sendMedia(chatID, mediaURL, msg.Content); err != nil {
				return err
			}
		}
		return nil
	}

	// 将Markdown格式转换为Telegram HTML格式
	text := markdownToHTML(msg.Content)

	// 分割超长消息（Telegram消息最大长度为4096字符，这里设置为4000以留出余量）
	parts := splitMessage(text, 4000)
	for _, part := range parts {
		if err := c.sendMessage(chatID, part); err != nil {
			return err
		}
	}

	return nil
}

// sendMessage 发送单条消息到Telegram
// 调用Telegram Bot API的sendMessage方法
// 参数:
//
//	chatID: 目标聊天的ID
//	text: 消息文本，支持HTML格式
//
// 返回: API调用失败时返回错误
func (c *TelegramChannel) sendMessage(chatID int64, text string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", c.token)

	// 构造请求数据
	data := map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML", // 使用HTML解析模式，支持富文本格式
	}

	body, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Telegram API error: %d", resp.StatusCode)
	}

	return nil
}

// sendMedia 发送媒体文件到Telegram
// 支持发送图片、视频、文档等
// 参数:
//
//	chatID: 目标聊天的ID
//	mediaURL: 媒体文件的URL或本地文件路径
//	caption: 媒体的说明文字
//
// 返回: API调用失败时返回错误
func (c *TelegramChannel) sendMedia(chatID int64, mediaURL string, caption string) error {
	// 判断是 HTTP URL 还是本地文件
	isHTTPURL := strings.HasPrefix(mediaURL, "http://") || strings.HasPrefix(mediaURL, "https://")

	if isHTTPURL {
		// HTTP URL - 使用 JSON 方式发送
		return c.sendPhotoByURL(chatID, mediaURL, caption)
	}

	// 本地文件 - 使用 multipart/form-data 上传
	return c.sendPhotoByFile(chatID, mediaURL, caption)
}

// sendPhotoByURL 通过 HTTP URL 发送图片
func (c *TelegramChannel) sendPhotoByURL(chatID int64, photoURL string, caption string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", c.token)

	// 构造请求数据
	data := map[string]interface{}{
		"chat_id": chatID,
		"photo":   photoURL,
	}

	// 添加说明文字（如果有）
	if caption != "" {
		data["caption"] = markdownToHTML(caption)
		data["parse_mode"] = "HTML"
	}

	body, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 读取响应体以获取详细错误信息
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("Telegram API error: %d, response: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// sendPhotoByFile 通过本地文件发送图片
func (c *TelegramChannel) sendPhotoByFile(chatID int64, filePath string, caption string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", c.token)

	// 展开 $HOME 和 ~ 路径
	expandedPath := os.ExpandEnv(filePath)
	if expandedPath != filePath {
		filePath = expandedPath
	}

	// 打开本地文件
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// 创建 multipart 表单
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// 添加 chat_id
	writer.WriteField("chat_id", strconv.FormatInt(chatID, 10))

	// 添加 caption（如果有）
	if caption != "" {
		writer.WriteField("caption", markdownToHTML(caption))
		writer.WriteField("parse_mode", "HTML")
	}

	// 添加图片文件
	part, err := writer.CreateFormFile("photo", file.Name())
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// 关闭 writer 以完成表单
	err = writer.Close()
	if err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	// 创建请求
	req, err := http.NewRequest("POST", apiURL, &buf)
	if err != nil {
		return err
	}

	// 设置 Content-Type 为 multipart/form-data
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 读取响应体以获取详细错误信息
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("Telegram API error: %d, response: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("Successfully sent photo: %s", filePath)
	return nil
}

// pollUpdates 轮询Telegram更新（接收消息）
// 使用长轮询方式持续获取新消息
// 工作流程：
// 1. 循环检查running标志和上下文状态
// 2. 调用getUpdates获取新消息
// 3. 处理每条更新，启动新goroutine处理消息
// 4. 更新updateID，确保不重复处理消息
// 5. 如果出错，使用指数退避重试（1s -> 2s -> 4s -> 8s -> 最多30s）
// 6. 对于EOF等临时错误，使用较短延迟重试
// 参数:
//
//	ctx: 上下文对象，用于优雅关闭
func (c *TelegramChannel) pollUpdates(ctx context.Context) {
	baseDelay := 1 * time.Second
	maxDelay := 30 * time.Second
	delay := baseDelay

	for c.running {
		select {
		case <-ctx.Done():
			return
		default:
			updates, err := c.getUpdates()
			if err != nil {
				// 检查是否是临时错误（EOF、网络断开等）
				errMsg := err.Error()
				isTemporaryError := strings.Contains(errMsg, "unexpected EOF") ||
					strings.Contains(errMsg, "connection reset") ||
					strings.Contains(errMsg, "network is unreachable")

				if isTemporaryError {
					// 临时错误使用较短延迟
					delay = 2 * time.Second
					log.Printf("Telegram polling temporary error: %v (retry in %v)", err, delay)
				} else {
					log.Printf("Telegram polling error: %v (retry in %v)", err, delay)
					// 指数退避，每次重试延迟翻倍，最多30秒
					delay = delay * 2
					if delay > maxDelay {
						delay = maxDelay
					}
				}
				time.Sleep(delay)
				continue
			}

			// 成功后重置延迟
			delay = baseDelay

			// 处理每条更新
			for _, update := range updates {
				if update.UpdateID >= c.updateID {
					c.updateID = update.UpdateID + 1
					// 使用新的goroutine处理消息，避免阻塞轮询
					go c.handleUpdate(update)
				}
			}

			// 短暂休眠，避免过于频繁的请求
			time.Sleep(1 * time.Second)
		}
	}
}

// getUpdates 从Telegram获取消息更新
// 使用Long Polling方式，超时时间为60秒
// 使用offset参数实现增量获取，避免重复接收消息
// 返回: 更新列表和可能的错误
func (c *TelegramChannel) getUpdates() ([]TelegramUpdate, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?timeout=60", c.token)

	// 添加offset参数，只获取大于等于updateID的更新
	if c.updateID > 0 {
		apiURL += fmt.Sprintf("&offset=%d", c.updateID)
	}

	// 创建新的HTTP请求（避免连接重用问题）
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool             `json:"ok"`
		Result []TelegramUpdate `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.OK {
		return nil, fmt.Errorf("Telegram API error")
	}

	return result.Result, nil
}

// handleUpdate 处理接收到的Telegram更新
// 处理流程：
// 1. 检查更新是否包含消息
// 2. 提取消息文本、图片或文档
// 3. 构建发送者ID（用户ID或用户ID|用户名）
// 4. 检查用户是否在白名单中
// 5. 保存chat_id用于后续回复
// 6. 如果有图片，下载并转换为base64
// 7. 将消息发布到消息总线
// 参数:
//
//	update: Telegram更新对象，包含消息等信息
func (c *TelegramChannel) handleUpdate(update TelegramUpdate) {
	if update.Message == nil {
		return
	}

	msg := update.Message

	// 忽略空消息（既没有文本也没有图片说明也没有文档）
	hasText := msg.Text != "" || msg.Caption != ""
	hasPhoto := len(msg.Photo) > 0
	hasDocument := msg.Document != nil

	if !hasText && !hasPhoto && !hasDocument {
		return
	}

	// 构建发送者ID，格式为 "用户ID" 或 "用户ID|用户名"
	senderID := strconv.FormatInt(msg.From.ID, 10)
	if msg.From.Username != "" {
		senderID = fmt.Sprintf("%s|%s", senderID, msg.From.Username)
	}

	// 白名单检查：如果配置了白名单，则只处理白名单中的用户消息
	if len(c.allowFrom) > 0 {
		// 检查完整ID（用户ID|用户名）或仅用户ID
		if !c.allowFrom[senderID] && !c.allowFrom[strconv.FormatInt(msg.From.ID, 10)] {
			log.Printf("Ignoring message from unauthorized user: %s", senderID)
			return
		}
	}

	// 保存chat_id，用于后续回复消息
	c.mu.Lock()
	c.chatIDs[senderID] = msg.Chat.ID
	c.mu.Unlock()

	// 提取消息内容（优先使用text，如果没有则使用caption）
	content := msg.Text
	if content == "" {
		content = msg.Caption
	}

	// 检查是否有输入处理回调，并且消息是纯文本（不是图片或文档）
	// 如果有交互式输入等待，将消息路由到输入处理器
	chatIDStr := strconv.FormatInt(msg.Chat.ID, 10)
	if c.inputHandler != nil && hasText && !hasPhoto && !hasDocument {
		// 调用输入处理回调
		if c.inputHandler("telegram", chatIDStr, content) {
			// 输入已被处理，不发送到消息总线
			log.Printf("Input routed to interaction handler for chat %s", chatIDStr)
			return
		}
	}

	// 处理图片和文档，下载为base64
	mediaList := []string{}

	// 处理图片
	if len(msg.Photo) > 0 {
		// 获取最高分辨率的图片（最后一张）
		photo := msg.Photo[len(msg.Photo)-1]
		base64Data, err := c.downloadFileAsBase64(photo.FileID)
		if err != nil {
			log.Printf("Failed to download photo: %v", err)
		} else {
			// 添加图片的base64数据，格式为 data:image/jpeg;base64,xxx
			mediaList = append(mediaList, base64Data)
		}
	}

	// 处理文档
	if msg.Document != nil {
		base64Data, err := c.downloadFileAsBase64(msg.Document.FileID)
		if err != nil {
			log.Printf("Failed to download document: %v", err)
		} else {
			mediaList = append(mediaList, base64Data)
		}
	}

	// 构建入站消息并发布到消息总线
	inbound := bus.InboundMessage{
		Message: bus.Message{
			ID:       strconv.FormatInt(update.UpdateID, 10),
			Channel:  "telegram",
			SenderID: senderID,
			ChatID:   strconv.FormatInt(msg.Chat.ID, 10),
			Content:  content,
			Media:    mediaList,
		},
	}

	if err := c.bus.PublishInbound(inbound); err != nil {
		log.Printf("Error publishing inbound message: %v", err)
	}
}

// downloadFileAsBase64 下载Telegram文件并转换为base64
// 参数:
//
//	fileID: Telegram文件的file_id
//
// 返回: base64编码的文件数据（带MIME类型前缀），错误信息
func (c *TelegramChannel) downloadFileAsBase64(fileID string) (string, error) {
	// 1. 获取文件信息
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getFile?file_id=%s", c.token, fileID)

	resp, err := c.httpClient.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("failed to get file info: %w", err)
	}
	defer resp.Body.Close()

	var fileResult struct {
		OK bool `json:"ok"`
		Result struct {
			FileID   string `json:"file_id"`
			FilePath string `json:"file_path"`
			FileSize int    `json:"file_size"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&fileResult); err != nil {
		return "", fmt.Errorf("failed to decode file response: %w", err)
	}

	if !fileResult.OK {
		return "", fmt.Errorf("getFile API returned error")
	}

	// 2. 下载文件
	downloadURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", c.token, fileResult.Result.FilePath)
	resp, err = c.httpClient.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	// 3. 读取文件内容
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read file data: %w", err)
	}

	// 4. 转换为base64
	base64Data := base64.StdEncoding.EncodeToString(data)

	// 5. 根据文件扩展名确定MIME类型
	mimeType := "application/octet-stream"
	if strings.HasSuffix(fileResult.Result.FilePath, ".jpg") || strings.HasSuffix(fileResult.Result.FilePath, ".jpeg") {
		mimeType = "image/jpeg"
	} else if strings.HasSuffix(fileResult.Result.FilePath, ".png") {
		mimeType = "image/png"
	} else if strings.HasSuffix(fileResult.Result.FilePath, ".gif") {
		mimeType = "image/gif"
	} else if strings.HasSuffix(fileResult.Result.FilePath, ".pdf") {
		mimeType = "application/pdf"
	}

	// 6. 返回data URL格式
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data), nil
}

// TelegramUpdate 表示Telegram的一次更新
// 可能包含消息、编辑消息、回调查询等不同类型的更新
type TelegramUpdate struct {
	UpdateID int64            `json:"update_id"` // 更新ID，单调递增
	Message  *TelegramMessage `json:"message"`   // 新消息
}

// TelegramMessage 表示Telegram消息
// 包含消息的所有基本信息：发送者、聊天、内容等
type TelegramMessage struct {
	MessageID int64            `json:"message_id"` // 消息ID
	From      *TelegramUser   `json:"from"`       // 发送者信息
	Chat      *TelegramChat   `json:"chat"`       // 聊天信息
	Text      string          `json:"text"`       // 文本消息内容
	Caption   string          `json:"caption"`    // 媒体文件的说明文字
	Photo     []TelegramPhoto `json:"photo"`      // 图片数组（如果消息包含图片）
	Document  *TelegramDocument `json:"document"` // 文档（如果消息包含文件）
}

// TelegramPhoto 表示Telegram图片
type TelegramPhoto struct {
	FileID   string `json:"file_id"`   // 文件唯一ID
	Width    int    `json:"width"`     // 图片宽度
	Height   int    `json:"height"`    // 图片高度
	FileSize int    `json:"file_size"` // 文件大小
}

// TelegramDocument 表示Telegram文档
type TelegramDocument struct {
	FileID   string `json:"file_id"`   // 文件唯一ID
	FileName string `json:"file_name"` // 文件名
	MimeType string `json:"mime_type"` // MIME类型
	FileSize int    `json:"file_size"` // 文件大小
}

// TelegramUser 表示Telegram用户
// 包含用户的基本身份信息
type TelegramUser struct {
	ID        int64  `json:"id"`         // 用户唯一ID
	Username  string `json:"username"`   // 用户名（可选）
	FirstName string `json:"first_name"` // 名字
}

// TelegramChat 表示Telegram聊天
// 可以是私聊、群组或频道
type TelegramChat struct {
	ID   int64  `json:"id"`   // 聊天唯一ID
	Type string `json:"type"` // 聊天类型：private, group, supergroup, channel
}

// markdownToHTML 将Markdown格式转换为Telegram支持的HTML格式
// Telegram不支持标准Markdown，需要转换为特定的HTML标签
// 转换规则：
// - 代码块（```）-> <pre><code>
// - 行内代码（`）-> <code>
// - 粗体（**或__）-> <b>
// - 斜体（_）-> <i>
// - 删除线（~~）-> <s>
// - 链接（[text](url)）-> <a href="url">text</a>
// - 标题和引用被简化处理
// - 列表项的符号替换为圆点
// 参数:
//
//	text: Markdown格式的文本
//
// 返回: HTML格式的文本
func markdownToHTML(text string) string {
	if text == "" {
		return ""
	}

	// 第一步：保护代码块，避免其中的特殊字符被转换
	codeBlocks := []string{}
	re := regexp.MustCompile("```[\\s\\S]*?```")
	text = re.ReplaceAllStringFunc(text, func(m string) string {
		codeBlocks = append(codeBlocks, m)
		return fmt.Sprintf("\x00CB%d\x00", len(codeBlocks)-1)
	})

	// 第二步：保护行内代码
	inlineCodes := []string{}
	re2 := regexp.MustCompile("`[^`]+`")
	text = re2.ReplaceAllStringFunc(text, func(m string) string {
		inlineCodes = append(inlineCodes, m[1:len(m)-1])
		return fmt.Sprintf("\x00IC%d\x00", len(inlineCodes)-1)
	})

	// 移除标题标记（Telegram不支持标题，只保留文本）
	re3 := regexp.MustCompile("^#{1,6}\\s+(.+)$")
	text = re3.ReplaceAllString(text, "$1")

	// 简化引用格式（移除引用符号，只保留文本）
	re4 := regexp.MustCompile("^>\\s*(.*)$")
	text = re4.ReplaceAllString(text, "$1")

	// 转义HTML特殊字符，避免被误解析
	text = strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	).Replace(text)

	// 转换链接格式
	re5 := regexp.MustCompile("\\[([^\\]]+)\\]\\(([^)]+)\\)")
	text = re5.ReplaceAllString(text, "<a href=\"$2\">$1</a>")

	// 转换粗体（**text** 或 __text__）
	re6 := regexp.MustCompile("\\*\\*(.+?)\\*\\*")
	text = re6.ReplaceAllString(text, "<b>$1</b>")
	re7 := regexp.MustCompile("__(.+?)__")
	text = re7.ReplaceAllString(text, "<b>$1</b>")

	// 转换斜体（_text_，注意避免与下划线变量名冲突）
	// 使用普通捕获组，不使用命名捕获组
	re8 := regexp.MustCompile(`(\s)_([^_]+)_(\s)`)
	text = re8.ReplaceAllString(text, "$1<i>$2</i>$3")

	// 转换删除线（~~text~~）
	re9 := regexp.MustCompile("~~(.+?)~~")
	text = re9.ReplaceAllString(text, "<s>$1</s>")

	// 转换列表项符号
	re10 := regexp.MustCompile("^[-\\*]\\s+")
	text = re10.ReplaceAllString(text, "• ")

	// 恢复行内代码（需要转义HTML字符）
	for i, code := range inlineCodes {
		code = strings.NewReplacer(
			"&", "&amp;",
			"<", "&lt;",
			">", "&gt;",
		).Replace(code)
		text = strings.Replace(text, fmt.Sprintf("\x00IC%d\x00", i), "<code>"+code+"</code>", 1)
	}

	// 恢复代码块（需要转义HTML字符并移除```标记）
	for i, code := range codeBlocks {
		code = strings.NewReplacer(
			"&", "&amp;",
			"<", "&lt;",
			">", "&gt;",
		).Replace(code)
		text = strings.Replace(text, fmt.Sprintf("\x00CB%d\x00", i), "<pre><code>"+code[3:len(code)-3]+"</code></pre>", 1)
	}

	return text
}

// splitMessage 将长消息分割为多个部分
// Telegram单条消息最大长度为4096字符，需要分割超长消息
// 分割策略：
// 1. 优先在换行符处分割
// 2. 如果一行过长，则在空格处分割
// 3. 如果都没有，则强制截断
// 参数:
//
//	text: 原始消息文本
//	maxLen: 每部分的最大长度
//
// 返回: 分割后的消息片段列表
func splitMessage(text string, maxLen int) []string {
	// 如果消息长度不超过限制，直接返回
	if len(text) <= maxLen {
		return []string{text}
	}

	var parts []string
	for len(text) > 0 {
		// 如果剩余文本不超过限制，直接添加
		if len(text) <= maxLen {
			parts = append(parts, text)
			break
		}

		// 截取maxLen长度的文本
		cut := text[:maxLen]

		// 尝试在换行符处分割
		pos := strings.LastIndex(cut, "\n")
		if pos == -1 {
			// 如果没有换行符，尝试在空格处分割
			pos = strings.LastIndex(cut, " ")
		}
		if pos == -1 {
			// 如果都没有，强制在maxLen处截断
			pos = maxLen
		}

		// 添加分割后的部分
		parts = append(parts, text[:pos])
		text = text[pos:]
	}
	return parts
}
