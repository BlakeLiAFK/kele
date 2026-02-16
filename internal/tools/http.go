package tools

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPTool HTTP 请求工具
type HTTPTool struct {
	maxOutputSize int
}

// NewHTTPTool 创建 HTTP 工具
func NewHTTPTool(maxOutputSize int) *HTTPTool {
	return &HTTPTool{maxOutputSize: maxOutputSize}
}

func (t *HTTPTool) Name() string { return "http" }

func (t *HTTPTool) Description() string {
	return "发起 HTTP 请求获取外部信息。支持 GET/POST/PUT/DELETE 方法。禁止访问内网地址。"
}

func (t *HTTPTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "请求 URL（必须是 http 或 https）",
			},
			"method": map[string]interface{}{
				"type":        "string",
				"description": "HTTP 方法（GET/POST/PUT/DELETE），默认 GET",
				"enum":        []string{"GET", "POST", "PUT", "DELETE"},
			},
			"headers": map[string]interface{}{
				"type":        "object",
				"description": "请求头（可选）",
			},
			"body": map[string]interface{}{
				"type":        "string",
				"description": "请求体（POST/PUT 时使用）",
			},
		},
		"required": []string{"url"},
	}
}

func (t *HTTPTool) Execute(args map[string]interface{}) (string, error) {
	rawURL, _ := args["url"].(string)
	if rawURL == "" {
		return "", fmt.Errorf("缺少 url 参数")
	}

	method, _ := args["method"].(string)
	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(method)

	// 安全检查：禁止访问内网地址
	if err := checkURLSafety(rawURL); err != nil {
		return "", err
	}

	var bodyReader io.Reader
	if body, ok := args["body"].(string); ok && body != "" {
		bodyReader = strings.NewReader(body)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("重定向次数过多")
			}
			return nil
		},
	}

	req, err := http.NewRequest(method, rawURL, bodyReader)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置默认 User-Agent
	req.Header.Set("User-Agent", "Kele/0.2.0")

	// 设置自定义 headers
	if headers, ok := args["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}
	}

	// 如果有 body 且没设置 Content-Type，默认 JSON
	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应（限制大小）
	maxSize := int64(t.maxOutputSize)
	if maxSize <= 0 {
		maxSize = 10240
	}
	limitReader := io.LimitReader(resp.Body, maxSize+1)
	respBody, err := io.ReadAll(limitReader)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	result := fmt.Sprintf("HTTP %d %s\n", resp.StatusCode, resp.Status)

	// 添加关键响应头
	for _, h := range []string{"Content-Type", "Content-Length"} {
		if v := resp.Header.Get(h); v != "" {
			result += fmt.Sprintf("%s: %s\n", h, v)
		}
	}
	result += "\n"

	body := string(respBody)
	if int64(len(respBody)) > maxSize {
		body = body[:maxSize] + fmt.Sprintf("\n\n... [响应被截断，超过 %d 字节]", maxSize)
	}
	result += body

	return result, nil
}

// checkURLSafety 检查 URL 是否安全（禁止内网访问）
func checkURLSafety(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("无效 URL: %v", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("仅支持 http/https 协议")
	}

	host := parsed.Hostname()

	// 检查是否为私有地址
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("禁止访问内网地址: %s", host)
		}
	}

	// 检查常见内网域名
	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".local") || strings.HasSuffix(lowerHost, ".internal") {
		return fmt.Errorf("禁止访问内网地址: %s", host)
	}

	return nil
}
