package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// web.go - Web搜索工具
// 此文件实现了 WebSearchTool，支持 Tavily 搜索

// WebSearchTool 提供网络搜索功能
// 使用 Tavily Search API
type WebSearchTool struct {
	BaseTool
	apiKey     string // API密钥
	maxResults int    // 最多返回的搜索结果数量
	httpClient *http.Client // HTTP客户端，配置了超时时间
}

// NewWebSearchTool 创建一个新的网络搜索工具
// 参数:
//
//	apiKey: Tavily API 密钥
//	maxResults: 最多返回的搜索结果数量
//
// 返回:
//
//	配置好的WebSearchTool实例
func NewWebSearchTool(apiKey string, provider string, maxResults int) *WebSearchTool {
	return &WebSearchTool{
		BaseTool: NewBaseTool(
			"web_search",
			"Search the web for information using Tavily Search API",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "The search query",
					},
				},
				"required": []string{"query"},
			},
		),
		apiKey:     apiKey,
		maxResults: maxResults,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Execute 执行网络搜索
// 使用 Tavily Search API
// 参数:
//
//	ctx: 上下文对象
//	params: 参数map，必须包含"query"字段
//
// 返回:
//
//	JSON格式的搜索结果数组，或错误信息
func (t *WebSearchTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// 获取搜索查询
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("missing or invalid query parameter")
	}

	// 检查API密钥
	if t.apiKey == "" {
		return "", fmt.Errorf("web search API key not configured")
	}

	// 执行 Tavily 搜索
	return t.searchTavily(ctx, query)
}

// searchTavily 执行 Tavily Search API
func (t *WebSearchTool) searchTavily(ctx context.Context, query string) (string, error) {
	apiURL := "https://api.tavily.com/search"

	// 构建请求体
	requestBody := map[string]interface{}{
		"api_key":     t.apiKey,
		"query":       query,
		"max_results": t.maxResults,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Tavily API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Tavily 响应格式
	var result struct {
		Results []struct {
			Title   string  `json:"title"`
			URL     string  `json:"url"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	output := make([]map[string]string, 0, len(result.Results))
	for _, r := range result.Results {
		output = append(output, map[string]string{
			"title":       r.Title,
			"description": r.Content,
			"url":         r.URL,
		})
	}

	b, _ := json.Marshal(output)
	return string(b), nil
}

// SearchTavilyImage 搜索图片（使用 Tavily）
func SearchTavilyImage(query string) ([]string, error) {
	apiKey := os.Getenv("TAVILY_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("TAVILY_API_KEY not set")
	}

	apiURL := "https://api.tavily.com/search_image"

	requestBody := map[string]interface{}{
		"api_key": apiKey,
		"query":   query,
	}

	bodyBytes, _ := json.Marshal(requestBody)

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Images []string `json:"images"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result.Images, nil
}

// SearchTavilyDeep 深度搜索（使用 Tavily）
func SearchTavilyDeep(query string) (string, error) {
	apiKey := os.Getenv("TAVILY_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("TAVILY_API_KEY not set")
	}

	apiURL := "https://api.tavily.com/search_deep"

	requestBody := map[string]interface{}{
		"api_key": apiKey,
		"query":   query,
	}

	bodyBytes, _ := json.Marshal(requestBody)

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// 逐行读取，提取文本内容
	var content strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, `"content"`) || strings.Contains(line, `"text"`) {
			content.WriteString(line)
			content.WriteString("\n")
		}
	}

	return content.String(), nil
}
