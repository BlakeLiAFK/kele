package tools

import (
	"strings"
	"testing"
)

func TestExtractHTMLContent_Basic(t *testing.T) {
	htmlStr := `<!DOCTYPE html>
<html>
<head>
	<title>测试页面</title>
	<meta name="description" content="这是一个测试页面的描述">
</head>
<body>
	<h1>主标题</h1>
	<p>这是一段正文内容。</p>
	<h2>二级标题</h2>
	<p>另一段内容，包含<a href="https://example.com">链接</a>。</p>
</body>
</html>`

	result := extractHTMLContent(strings.NewReader(htmlStr), "https://test.com")

	// 检查标题
	if !strings.Contains(result, "# 测试页面") {
		t.Errorf("缺少页面标题，结果:\n%s", result)
	}

	// 检查描述
	if !strings.Contains(result, "> 这是一个测试页面的描述") {
		t.Errorf("缺少描述，结果:\n%s", result)
	}

	// 检查 h1 转换为 ##
	if !strings.Contains(result, "## 主标题") {
		t.Errorf("缺少 h1 转换，结果:\n%s", result)
	}

	// 检查 h2 转换为 ###
	if !strings.Contains(result, "### 二级标题") {
		t.Errorf("缺少 h2 转换，结果:\n%s", result)
	}

	// 检查正文
	if !strings.Contains(result, "这是一段正文内容") {
		t.Errorf("缺少正文，结果:\n%s", result)
	}

	// 检查链接格式
	if !strings.Contains(result, "[链接](https://example.com)") {
		t.Errorf("缺少链接格式，结果:\n%s", result)
	}
}

func TestExtractHTMLContent_SkipTags(t *testing.T) {
	htmlStr := `<html><body>
<script>var x = 1;</script>
<style>.foo { color: red; }</style>
<noscript>请启用 JS</noscript>
<nav>导航栏内容</nav>
<footer>底部内容</footer>
<p>保留的正文</p>
</body></html>`

	result := extractHTMLContent(strings.NewReader(htmlStr), "https://test.com")

	if strings.Contains(result, "var x = 1") {
		t.Error("script 内容未被过滤")
	}
	if strings.Contains(result, "color: red") {
		t.Error("style 内容未被过滤")
	}
	if strings.Contains(result, "请启用 JS") {
		t.Error("noscript 内容未被过滤")
	}
	if strings.Contains(result, "导航栏内容") {
		t.Error("nav 内容未被过滤")
	}
	if strings.Contains(result, "底部内容") {
		t.Error("footer 内容未被过滤")
	}
	if !strings.Contains(result, "保留的正文") {
		t.Errorf("正文内容丢失，结果:\n%s", result)
	}
}

func TestExtractHTMLContent_Lists(t *testing.T) {
	htmlStr := `<html><body>
<ul>
	<li>无序项 1</li>
	<li>无序项 2</li>
</ul>
<ol>
	<li>有序项 1</li>
	<li>有序项 2</li>
</ol>
</body></html>`

	result := extractHTMLContent(strings.NewReader(htmlStr), "https://test.com")

	if !strings.Contains(result, "- 无序项 1") {
		t.Errorf("缺少无序列表，结果:\n%s", result)
	}
	if !strings.Contains(result, "- 无序项 2") {
		t.Errorf("缺少无序列表项 2，结果:\n%s", result)
	}
	if !strings.Contains(result, "1. 有序项 1") {
		t.Errorf("缺少有序列表，结果:\n%s", result)
	}
	if !strings.Contains(result, "2. 有序项 2") {
		t.Errorf("缺少有序列表项 2，结果:\n%s", result)
	}
}

func TestExtractHTMLContent_CodeBlock(t *testing.T) {
	htmlStr := `<html><body>
<pre><code>func main() {
    fmt.Println("hello")
}</code></pre>
</body></html>`

	result := extractHTMLContent(strings.NewReader(htmlStr), "https://test.com")

	if !strings.Contains(result, "```") {
		t.Errorf("缺少代码块标记，结果:\n%s", result)
	}
	if !strings.Contains(result, "fmt.Println") {
		t.Errorf("缺少代码内容，结果:\n%s", result)
	}
}

func TestExtractHTMLContent_RelativeLinks(t *testing.T) {
	htmlStr := `<html><body>
<p>查看<a href="/docs/guide">文档</a>了解更多。</p>
</body></html>`

	result := extractHTMLContent(strings.NewReader(htmlStr), "https://example.com/page")

	if !strings.Contains(result, "[文档](https://example.com/docs/guide)") {
		t.Errorf("相对链接未正确解析，结果:\n%s", result)
	}
}

func TestExtractHTMLContent_InlineFormatting(t *testing.T) {
	htmlStr := `<html><body>
<p>这是<strong>加粗</strong>和<em>斜体</em>和<code>代码</code>文本。</p>
</body></html>`

	result := extractHTMLContent(strings.NewReader(htmlStr), "https://test.com")

	if !strings.Contains(result, "**加粗**") {
		t.Errorf("缺少加粗格式，结果:\n%s", result)
	}
	if !strings.Contains(result, "*斜体*") {
		t.Errorf("缺少斜体格式，结果:\n%s", result)
	}
	if !strings.Contains(result, "`代码`") {
		t.Errorf("缺少行内代码格式，结果:\n%s", result)
	}
}

func TestExtractHTMLContent_Table(t *testing.T) {
	htmlStr := `<html><body>
<table>
	<tr><th>名称</th><th>版本</th></tr>
	<tr><td>Go</td><td>1.24</td></tr>
	<tr><td>Rust</td><td>1.82</td></tr>
</table>
</body></html>`

	result := extractHTMLContent(strings.NewReader(htmlStr), "https://test.com")

	if !strings.Contains(result, "名称") || !strings.Contains(result, "版本") {
		t.Errorf("缺少表头，结果:\n%s", result)
	}
	if !strings.Contains(result, "Go") || !strings.Contains(result, "1.24") {
		t.Errorf("缺少表格数据，结果:\n%s", result)
	}
}

func TestProcessResponse_JSON(t *testing.T) {
	tool := NewWebFetchTool(10240)
	body := strings.NewReader(`{"key": "value", "count": 42}`)
	result := tool.processResponse(body, "application/json; charset=utf-8", "https://api.test.com")

	if !strings.Contains(result, `"key"`) || !strings.Contains(result, `"value"`) {
		t.Errorf("JSON 处理异常，结果:\n%s", result)
	}
}

func TestProcessResponse_PlainText(t *testing.T) {
	tool := NewWebFetchTool(10240)
	body := strings.NewReader("这是纯文本内容")
	result := tool.processResponse(body, "text/plain", "https://test.com")

	if result != "这是纯文本内容" {
		t.Errorf("纯文本处理异常，结果: %s", result)
	}
}

func TestProcessResponse_UnsupportedType(t *testing.T) {
	tool := NewWebFetchTool(10240)
	body := strings.NewReader("")
	result := tool.processResponse(body, "image/png", "https://test.com")

	if !strings.Contains(result, "不支持的内容类型") {
		t.Errorf("未正确处理不支持的类型，结果: %s", result)
	}
}

func TestURLSafety(t *testing.T) {
	tool := NewWebFetchTool(10240)

	// 内网地址应被拒绝
	_, err := tool.Execute(map[string]interface{}{
		"url": "http://127.0.0.1/admin",
	})
	if err == nil || !strings.Contains(err.Error(), "内网") {
		t.Errorf("应拒绝内网地址，err: %v", err)
	}

	// localhost 应被拒绝
	_, err = tool.Execute(map[string]interface{}{
		"url": "http://localhost:8080",
	})
	if err == nil || !strings.Contains(err.Error(), "内网") {
		t.Errorf("应拒绝 localhost，err: %v", err)
	}

	// 缺少 url 参数
	_, err = tool.Execute(map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "url") {
		t.Errorf("应报缺少 url，err: %v", err)
	}
}

func TestCleanText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  hello   world  ", "hello world"},
		{"\n\t 多个  空白\t\n", "多个 空白"},
		{"", ""},
		{"正常文本", "正常文本"},
	}

	for _, tc := range tests {
		got := cleanText(tc.input)
		if got != tc.expected {
			t.Errorf("cleanText(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestResolveURL(t *testing.T) {
	tests := []struct {
		base     string
		href     string
		expected string
	}{
		{"https://example.com/page", "/docs", "https://example.com/docs"},
		{"https://example.com/page", "https://other.com/path", "https://other.com/path"},
		{"https://example.com/page", "#section", "#section"},
		{"https://example.com/dir/page", "sub/file", "https://example.com/dir/sub/file"},
	}

	for _, tc := range tests {
		got := resolveURL(tc.base, tc.href)
		if got != tc.expected {
			t.Errorf("resolveURL(%q, %q) = %q, want %q", tc.base, tc.href, got, tc.expected)
		}
	}
}

func TestTruncateUTF8(t *testing.T) {
	// 中文字符每个 3 字节
	s := "你好世界" // 12 bytes
	got := truncateUTF8(s, 6)
	if got != "你好" {
		t.Errorf("truncateUTF8 截断中文异常: got %q, want %q", got, "你好")
	}

	// 不需要截断
	got = truncateUTF8(s, 20)
	if got != s {
		t.Errorf("truncateUTF8 不应截断: got %q", got)
	}
}

func TestIsSkipElement(t *testing.T) {
	skips := []string{"script", "style", "noscript", "nav", "footer", "header", "iframe", "svg", "form"}
	for _, tag := range skips {
		if !isSkipElement(tag) {
			t.Errorf("isSkipElement(%q) should be true", tag)
		}
	}

	keeps := []string{"div", "span", "p", "article", "section", "main"}
	for _, tag := range keeps {
		if isSkipElement(tag) {
			t.Errorf("isSkipElement(%q) should be false", tag)
		}
	}
}

func TestHeadingPrefix(t *testing.T) {
	tests := []struct {
		tag      string
		expected string
	}{
		{"h1", "## "},
		{"h2", "### "},
		{"h3", "#### "},
		{"p", ""},
		{"div", ""},
	}

	for _, tc := range tests {
		got := headingPrefix(tc.tag)
		if got != tc.expected {
			t.Errorf("headingPrefix(%q) = %q, want %q", tc.tag, got, tc.expected)
		}
	}
}

func TestWebFetchTool_Interface(t *testing.T) {
	tool := NewWebFetchTool(10240)

	if tool.Name() != "web_fetch" {
		t.Errorf("Name() = %q, want 'web_fetch'", tool.Name())
	}

	if tool.Description() == "" {
		t.Error("Description() 不应为空")
	}

	params := tool.Parameters()
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Parameters() 缺少 properties")
	}
	if _, ok := props["url"]; !ok {
		t.Error("Parameters() 缺少 url 属性")
	}
}
