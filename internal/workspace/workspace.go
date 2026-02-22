package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

var nameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// WorkInfo 工作空间信息
type WorkInfo struct {
	Name    string
	Path    string
	Size    int64
	ModTime time.Time
}

// Manager 工作空间管理器
type Manager struct {
	baseDir string
}

// NewManager 创建管理器，baseDir 默认为 ~/.kele/works/
func NewManager() *Manager {
	homeDir, _ := os.UserHomeDir()
	return &Manager{
		baseDir: filepath.Join(homeDir, ".kele", "works"),
	}
}

// newManagerWithDir 创建指定目录的管理器（仅用于测试）
func newManagerWithDir(dir string) *Manager {
	return &Manager{baseDir: dir}
}

// Create 创建工作空间，返回路径
func (m *Manager) Create(name string) (string, error) {
	if !nameRegex.MatchString(name) {
		return "", fmt.Errorf("工作空间名称无效（仅允许字母、数字、连字符、下划线，最长64字符）")
	}
	dir := filepath.Join(m.baseDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建工作空间失败: %v", err)
	}
	return dir, nil
}

// List 列出所有工作空间
func (m *Manager) List() ([]WorkInfo, error) {
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var result []WorkInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(m.baseDir, e.Name())
		result = append(result, WorkInfo{
			Name:    e.Name(),
			Path:    path,
			Size:    dirSize(path),
			ModTime: info.ModTime(),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ModTime.After(result[j].ModTime)
	})
	return result, nil
}

// Delete 删除指定工作空间
func (m *Manager) Delete(name string) error {
	if !nameRegex.MatchString(name) {
		return fmt.Errorf("工作空间名称无效")
	}
	dir := filepath.Join(m.baseDir, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("工作空间不存在: %s", name)
	}
	return os.RemoveAll(dir)
}

// Clear 清空所有工作空间，返回删除数量
func (m *Manager) Clear() (int, error) {
	list, err := m.List()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, w := range list {
		if err := os.RemoveAll(w.Path); err == nil {
			count++
		}
	}
	return count, nil
}

// Ensure 获取或创建工作空间
func (m *Manager) Ensure(name string) (string, error) {
	dir := filepath.Join(m.baseDir, name)
	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	}
	return m.Create(name)
}

// BaseDir 返回基础目录
func (m *Manager) BaseDir() string {
	return m.baseDir
}

// dirSize 计算目录大小
func dirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		size += info.Size()
		return nil
	})
	return size
}

// FormatSize 格式化文件大小
func FormatSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/1024/1024)
	case bytes >= 1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
