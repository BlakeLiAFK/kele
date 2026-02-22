package tools

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// WebFetchTool 网页内容提取工具
type WebFetchTool struct {
	maxOutputSize int
}

// NewWebFetchTool 创建网页提取工具
func NewWebFetchTool(maxOutputSize int) *WebFetchTool {
	return &WebFetchTool{maxOutputSize: maxOutputSize}
}

func (t *WebFetchTool) Name() string { return "web_fetch" }

func (t *WebFetchTool) Description() string {
	return "抓取网页并提取可读正文内容。将 HTML 转换为结构化文本（Markdown 风格），适合阅读理解。仅用于网页内容提取，API 调用请使用 http 工具。"
}

func (t *WebFetchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "要抓取的网页 URL（必须是 http 或 https）",
			},
		},
		"required": []string{"url"},
	}
}

func (t *WebFetchTool) Execute(args map[string]interface{}) (string, error) {
	rawURL, _ := args["url"].(string)
	if rawURL == "" {
		return "", fmt.Errorf("缺少 url 参数")
	}

	// 安全检查
	if err := checkURLSafety(rawURL); err != nil {
		return "", err
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

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}

	// 模拟浏览器请求，提高兼容性
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Kele/0.4.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d %s", resp.StatusCode, resp.Status)
	}

	// 限制读取大小（原始数据最多 1MB）
	maxRead := int64(1024 * 1024)
	limitReader := io.LimitReader(resp.Body, maxRead)

	contentType := resp.Header.Get("Content-Type")
	result := t.processResponse(limitReader, contentType, rawURL)

	// 输出截断
	maxOutput := t.maxOutputSize
	if maxOutput <= 0 {
		maxOutput = 10240
	}
	if len(result) > maxOutput {
		result = truncateUTF8(result, maxOutput) +
			fmt.Sprintf("\n\n... [内容被截断，超过 %d 字节]", maxOutput)
	}

	return result, nil
}

// processResponse 根据 Content-Type 分发处理
func (t *WebFetchTool) processResponse(body io.Reader, contentType, rawURL string) string {
	ct := strings.ToLower(contentType)

	switch {
	case strings.Contains(ct, "text/html"), strings.Contains(ct, "application/xhtml"):
		return extractHTMLContent(body, rawURL)

	case strings.Contains(ct, "application/json"):
		data, err := io.ReadAll(body)
		if err != nil {
			return fmt.Sprintf("[读取 JSON 失败: %v]", err)
		}
		return string(data)

	case strings.Contains(ct, "text/"):
		data, err := io.ReadAll(body)
		if err != nil {
			return fmt.Sprintf("[读取文本失败: %v]", err)
		}
		return string(data)

	default:
		return fmt.Sprintf("[不支持的内容类型: %s，无法提取文本内容]", contentType)
	}
}

// extractHTMLContent 从 HTML 提取结构化文本
func extractHTMLContent(body io.Reader, baseURL string) string {
	doc, err := html.Parse(body)
	if err != nil {
		return fmt.Sprintf("[HTML 解析失败: %v]", err)
	}

	var title string
	var description string
	var buf strings.Builder

	// 提取 meta 信息
	extractMeta(doc, &title, &description)

	// 输出标题和描述
	if title != "" {
		buf.WriteString("# ")
		buf.WriteString(title)
		buf.WriteString("\n\n")
	}
	if description != "" {
		buf.WriteString("> ")
		buf.WriteString(description)
		buf.WriteString("\n\n")
	}

	// 提取正文
	extractText(doc, baseURL, &buf)

	return strings.TrimSpace(buf.String())
}

// extractMeta 提取 title 和 meta description
func extractMeta(n *html.Node, title, description *string) {
	if n.Type == html.ElementNode {
		if n.DataAtom == atom.Title && *title == "" {
			*title = cleanText(textContent(n))
		}
		if n.DataAtom == atom.Meta {
			var name, content string
			for _, a := range n.Attr {
				switch strings.ToLower(a.Key) {
				case "name":
					name = strings.ToLower(a.Val)
				case "content":
					content = a.Val
				}
			}
			if name == "description" && *description == "" {
				*description = cleanText(content)
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractMeta(c, title, description)
	}
}

// textContent 递归获取节点纯文本
func textContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(textContent(c))
	}
	return sb.String()
}

// extractText 递归提取正文内容
func extractText(n *html.Node, baseURL string, buf *strings.Builder) {
	if n.Type == html.ElementNode {
		// 跳过不需要的标签
		if isSkipElement(n.Data) {
			return
		}

		tag := n.Data

		// 处理标题
		if prefix := headingPrefix(tag); prefix != "" {
			buf.WriteString("\n")
			buf.WriteString(prefix)
			buf.WriteString(cleanText(textContent(n)))
			buf.WriteString("\n\n")
			return
		}

		// 处理段落
		if tag == "p" {
			text := cleanText(extractInlineContent(n, baseURL))
			if text != "" {
				buf.WriteString(text)
				buf.WriteString("\n\n")
			}
			return
		}

		// 处理链接（独立块级）
		if tag == "a" {
			href := getAttr(n, "href")
			text := cleanText(textContent(n))
			if text != "" && href != "" {
				absURL := resolveURL(baseURL, href)
				buf.WriteString(fmt.Sprintf("[%s](%s)", text, absURL))
			} else if text != "" {
				buf.WriteString(text)
			}
			return
		}

		// 处理列表
		if tag == "ul" || tag == "ol" {
			buf.WriteString("\n")
			idx := 0
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "li" {
					idx++
					text := cleanText(extractInlineContent(c, baseURL))
					if text != "" {
						if tag == "ol" {
							buf.WriteString(fmt.Sprintf("%d. %s\n", idx, text))
						} else {
							buf.WriteString(fmt.Sprintf("- %s\n", text))
						}
					}
				}
			}
			buf.WriteString("\n")
			return
		}

		// 处理代码块
		if tag == "pre" {
			code := cleanText(textContent(n))
			if code != "" {
				buf.WriteString("\n```\n")
				buf.WriteString(code)
				buf.WriteString("\n```\n\n")
			}
			return
		}

		// 处理换行
		if tag == "br" {
			buf.WriteString("\n")
			return
		}

		// 处理水平线
		if tag == "hr" {
			buf.WriteString("\n---\n\n")
			return
		}

		// 处理表格 - 简化为文本
		if tag == "table" {
			extractTableText(n, baseURL, buf)
			return
		}
	}

	// 文本节点
	if n.Type == html.TextNode {
		text := cleanText(n.Data)
		if text != "" {
			buf.WriteString(text)
			buf.WriteString(" ")
		}
		return
	}

	// 递归子节点
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractText(c, baseURL, buf)
	}
}

// extractInlineContent 提取内联内容（保留链接格式）
func extractInlineContent(n *html.Node, baseURL string) string {
	var sb strings.Builder
	extractInline(n, baseURL, &sb)
	return sb.String()
}

func extractInline(n *html.Node, baseURL string, buf *strings.Builder) {
	if n.Type == html.TextNode {
		buf.WriteString(n.Data)
		return
	}

	if n.Type == html.ElementNode {
		if isSkipElement(n.Data) {
			return
		}

		// 链接
		if n.Data == "a" {
			href := getAttr(n, "href")
			text := cleanText(textContent(n))
			if text != "" && href != "" {
				absURL := resolveURL(baseURL, href)
				buf.WriteString(fmt.Sprintf("[%s](%s)", text, absURL))
			} else if text != "" {
				buf.WriteString(text)
			}
			return
		}

		// 行内代码
		if n.Data == "code" {
			text := textContent(n)
			if text != "" {
				buf.WriteString("`")
				buf.WriteString(text)
				buf.WriteString("`")
			}
			return
		}

		// 强调
		if n.Data == "strong" || n.Data == "b" {
			text := textContent(n)
			if text != "" {
				buf.WriteString("**")
				buf.WriteString(text)
				buf.WriteString("**")
			}
			return
		}

		if n.Data == "em" || n.Data == "i" {
			text := textContent(n)
			if text != "" {
				buf.WriteString("*")
				buf.WriteString(text)
				buf.WriteString("*")
			}
			return
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractInline(c, baseURL, buf)
	}
}

// extractTableText 简化提取表格内容
func extractTableText(n *html.Node, baseURL string, buf *strings.Builder) {
	buf.WriteString("\n")
	var rows [][]string
	collectTableRows(n, baseURL, &rows)
	for _, row := range rows {
		buf.WriteString("| ")
		buf.WriteString(strings.Join(row, " | "))
		buf.WriteString(" |\n")
	}
	buf.WriteString("\n")
}

func collectTableRows(n *html.Node, baseURL string, rows *[][]string) {
	if n.Type == html.ElementNode && (n.Data == "tr") {
		var cells []string
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
				cells = append(cells, cleanText(extractInlineContent(c, baseURL)))
			}
		}
		if len(cells) > 0 {
			*rows = append(*rows, cells)
		}
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectTableRows(c, baseURL, rows)
	}
}

// isSkipElement 判断是否为需要跳过的标签
func isSkipElement(tag string) bool {
	switch tag {
	case "script", "style", "noscript", "nav", "footer", "header",
		"iframe", "svg", "form", "button", "input", "select", "textarea":
		return true
	}
	return false
}

// headingPrefix 返回标题前缀
func headingPrefix(tag string) string {
	switch tag {
	case "h1":
		return "## "
	case "h2":
		return "### "
	case "h3":
		return "#### "
	case "h4":
		return "##### "
	case "h5":
		return "###### "
	case "h6":
		return "###### "
	}
	return ""
}

// getAttr 获取节点属性值
func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// resolveURL 解析相对 URL
func resolveURL(baseURL, href string) string {
	if href == "" || strings.HasPrefix(href, "#") {
		return href
	}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

// cleanText 清理文本（合并空白）
func cleanText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// 合并连续空白为单个空格
	var sb strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace {
				sb.WriteRune(' ')
				prevSpace = true
			}
		} else {
			sb.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(sb.String())
}

// truncateUTF8 按字节截断但保证 UTF-8 完整性
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// 从 maxBytes 位置往回找到完整的 UTF-8 字符边界
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}
