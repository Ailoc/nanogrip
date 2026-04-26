package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// web.go - Web搜索工具
// 此文件实现了 WebSearchTool，支持 Tavily 搜索

// WebSearchTool 提供网络搜索功能
// 使用 Tavily Search API
type WebSearchTool struct {
	BaseTool
	apiKey     string       // API密钥
	provider   string       // 搜索提供商
	maxResults int          // 最多返回的搜索结果数量
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
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = "tavily"
	}
	if maxResults <= 0 {
		maxResults = 5
	}

	return &WebSearchTool{
		BaseTool: NewBaseTool(
			"web_search",
			"Search the web for current information",
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
		provider:   provider,
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

	switch t.provider {
	case "tavily":
		return t.searchTavily(ctx, query)
	case "brave":
		return t.searchBrave(ctx, query)
	default:
		return "", fmt.Errorf("unsupported web search provider: %s", t.provider)
	}
}

// searchBrave 执行 Brave Search API
func (t *WebSearchTool) searchBrave(ctx context.Context, query string) (string, error) {
	apiURL := "https://api.search.brave.com/res/v1/web/search"
	requestURL, err := url.Parse(apiURL)
	if err != nil {
		return "", err
	}
	values := requestURL.Query()
	values.Set("q", query)
	values.Set("count", fmt.Sprintf("%d", t.maxResults))
	requestURL.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", requestURL.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", t.apiKey)

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
		return "", fmt.Errorf("Brave API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	output := make([]map[string]string, 0, len(result.Web.Results))
	for _, r := range result.Web.Results {
		output = append(output, map[string]string{
			"title":       r.Title,
			"description": r.Description,
			"url":         r.URL,
		})
	}

	b, _ := json.Marshal(output)
	return string(b), nil
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

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(bodyBytes))
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
