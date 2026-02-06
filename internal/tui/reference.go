package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Reference @ 引用
type Reference struct {
	Raw     string // 原始文本，如 @main.go
	Path    string // 解析后的路径
	Type    string // file / dir / glob
	Content string // 读取到的内容
	Error   error  // 读取错误
}

// parseReferences 从输入中解析所有 @ 引用
func parseReferences(input string) (string, []Reference) {
	// 匹配 @ 开头的路径：支持字母、数字、点、斜杠、星号、下划线、连字符
	re := regexp.MustCompile(`@([\w./*\-]+[\w./*\-]*)`)
	matches := re.FindAllStringSubmatchIndex(input, -1)

	if len(matches) == 0 {
		return input, nil
	}

	var refs []Reference
	seen := make(map[string]bool)

	for _, match := range matches {
		raw := input[match[0]:match[1]]
		path := input[match[2]:match[3]]

		// 去重
		if seen[path] {
			continue
		}
		seen[path] = true

		ref := resolveReference(path)
		ref.Raw = raw
		refs = append(refs, ref)
	}

	// 构建纯文本（去掉 @ 引用部分）
	cleanInput := re.ReplaceAllString(input, "")
	cleanInput = strings.TrimSpace(cleanInput)

	return cleanInput, refs
}

// resolveReference 解析单个引用
func resolveReference(path string) Reference {
	ref := Reference{Path: path}

	// 判断类型
	if strings.Contains(path, "*") {
		ref.Type = "glob"
		ref.Content, ref.Error = readGlob(path)
	} else {
		info, err := os.Stat(path)
		if err != nil {
			ref.Type = "file"
			ref.Error = fmt.Errorf("路径不存在: %s", path)
			return ref
		}

		if info.IsDir() {
			ref.Type = "dir"
			ref.Content, ref.Error = readDir(path)
		} else {
			ref.Type = "file"
			ref.Content, ref.Error = readFile(path)
		}
	}

	return ref
}

// readFile 读取文件内容
func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	content := string(data)

	// 限制大小：超过 10000 字符截断
	const maxSize = 10000
	if len(content) > maxSize {
		content = content[:maxSize] + fmt.Sprintf("\n\n... [文件截断，共 %d 字节]", len(data))
	}

	return content, nil
}

// readDir 读取目录结构
func readDir(path string) (string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("目录: %s/\n\n", path))

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue // 跳过隐藏文件
		}
		if entry.IsDir() {
			b.WriteString(fmt.Sprintf("  📁 %s/\n", entry.Name()))
		} else {
			info, _ := entry.Info()
			size := ""
			if info != nil {
				size = formatSize(info.Size())
			}
			b.WriteString(fmt.Sprintf("  📄 %s  %s\n", entry.Name(), size))
		}
	}

	return b.String(), nil
}

// readGlob 按 glob 模式读取文件
func readGlob(pattern string) (string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("没有匹配的文件: %s", pattern)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("匹配 %s (%d 个文件):\n\n", pattern, len(matches)))

	// 限制文件数量
	const maxFiles = 10
	for i, match := range matches {
		if i >= maxFiles {
			b.WriteString(fmt.Sprintf("\n... 还有 %d 个文件未显示\n", len(matches)-maxFiles))
			break
		}

		content, err := readFile(match)
		if err != nil {
			b.WriteString(fmt.Sprintf("--- %s [读取失败: %v] ---\n\n", match, err))
			continue
		}

		b.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", match, content))
	}

	return b.String(), nil
}

// buildContextMessage 构建包含引用内容的上下文消息
func buildContextMessage(userText string, refs []Reference) string {
	if len(refs) == 0 {
		return userText
	}

	var b strings.Builder

	// 先写引用的上下文
	b.WriteString("<context>\n")
	for _, ref := range refs {
		if ref.Error != nil {
			continue
		}
		b.WriteString(fmt.Sprintf("<file path=\"%s\">\n", ref.Path))
		b.WriteString(ref.Content)
		b.WriteString("\n</file>\n\n")
	}
	b.WriteString("</context>\n\n")

	// 再写用户的实际问题
	if userText != "" {
		b.WriteString(userText)
	}

	return b.String()
}

// formatRefSummary 格式化引用摘要（在 TUI 中显示）
func formatRefSummary(refs []Reference) string {
	if len(refs) == 0 {
		return ""
	}

	var parts []string
	for _, ref := range refs {
		if ref.Error != nil {
			parts = append(parts, fmt.Sprintf("❌ %s (%v)", ref.Path, ref.Error))
		} else {
			icon := "📄"
			if ref.Type == "dir" {
				icon = "📁"
			} else if ref.Type == "glob" {
				icon = "📦"
			}
			size := len(ref.Content)
			parts = append(parts, fmt.Sprintf("%s %s (%s)", icon, ref.Path, formatSize(int64(size))))
		}
	}

	return "📎 " + strings.Join(parts, " | ")
}

// completeFilePath @ 引用的文件路径补全
func completeFilePath(partial string) (string, []string) {
	if partial == "" {
		// 列出当前目录
		entries, err := os.ReadDir(".")
		if err != nil {
			return "", nil
		}
		var candidates []string
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			name := e.Name()
			if e.IsDir() {
				name += "/"
			}
			candidates = append(candidates, name)
		}
		return "", candidates
	}

	// 解析目录和前缀
	dir := filepath.Dir(partial)
	base := filepath.Base(partial)

	// 如果 partial 以 / 结尾，说明是目录
	if strings.HasSuffix(partial, "/") {
		dir = partial
		base = ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return partial, nil
	}

	var candidates []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if base != "" && !strings.HasPrefix(strings.ToLower(e.Name()), strings.ToLower(base)) {
			continue
		}
		name := filepath.Join(dir, e.Name())
		if e.IsDir() {
			name += "/"
		}
		candidates = append(candidates, name)
	}

	if len(candidates) == 1 {
		return candidates[0], candidates
	}

	if len(candidates) > 1 {
		prefix := findCommonPrefix(candidates)
		if len(prefix) > len(partial) {
			return prefix, candidates
		}
	}

	return partial, candidates
}

// formatSize 格式化文件大小
func formatSize(size int64) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%dB", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(size)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(size)/1024/1024)
	}
}
