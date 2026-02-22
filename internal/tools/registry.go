package tools

import (
	"fmt"
	"sync"

	"github.com/BlakeLiAFK/kele/internal/llm"
)

// ToolHandler 工具处理器接口
type ToolHandler interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Execute(args map[string]interface{}) (string, error)
}

// Registry 工具注册表
type Registry struct {
	tools map[string]ToolHandler
	order []string // 保持注册顺序
	mu    sync.RWMutex
}

// NewRegistry 创建注册表
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]ToolHandler),
	}
}

// Register 注册工具
func (r *Registry) Register(tool ToolHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := tool.Name()
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = tool
}

// Execute 执行工具
func (r *Registry) Execute(name string, args map[string]interface{}) (string, error) {
	r.mu.RLock()
	tool, ok := r.tools[name]
	r.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("未知工具: %s", name)
	}
	return tool.Execute(args)
}

// GetTools 获取所有工具的 LLM 定义
func (r *Registry) GetTools() []llm.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tools []llm.Tool
	for _, name := range r.order {
		handler := r.tools[name]
		tools = append(tools, llm.Tool{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        handler.Name(),
				Description: handler.Description(),
				Parameters:  handler.Parameters(),
			},
		})
	}
	return tools
}

// List 列出所有已注册工具名
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]string, len(r.order))
	copy(result, r.order)
	return result
}

// Has 检查工具是否已注册
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tools[name]
	return ok
}

// GetHandler 获取指定名称的工具处理器
func (r *Registry) GetHandler(name string) (ToolHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}
