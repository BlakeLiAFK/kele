# TUI 全面重构

> 创建: 2026-02-07
> 状态: 已完成

## 目标

8 项功能需求的全面实现，涉及架构调整和 UI 重写。

## 需求清单

1. Up/Down 历史导航（类似终端）
2. Ctrl+O 高级设置页面
3. 多 Session 多 Tab（Alt+1..9 切换，隔离但可 @ 引用）
4. 气泡布局（AI 左对齐，用户右对齐）
5. 双击 Ctrl+C 退出（2秒内按两次）
6. Ctrl+J 换行（Mac 无 Alt+Enter，Ctrl+J = Unix LF）
7. 文件读写 Tool Call（流式中支持工具调用）
8. 多任务执行（Task Chain，双击 ESC 打断）

## 实施步骤

- [x] 步骤 1: 创建 session.go — Session 结构/输入历史
- [x] 步骤 2: 创建 styles.go — 气泡样式
- [x] 步骤 3: 创建 view.go — 气泡渲染
- [x] 步骤 4: 创建 commands.go — 命令处理（含 /new /sessions /switch）
- [x] 步骤 5: 创建 keys.go — 按键处理（含 Ctrl+J/双击检测/历史/Alt+N）
- [x] 步骤 6: 重写 app.go — 多 Session 容器
- [x] 步骤 7: 修改 llm/client.go — 流式 tool_calls 支持
- [x] 步骤 8: 修改 agent/brain.go — Agentic 流式循环
- [x] 步骤 9: Ctrl+O 设置页
- [x] 步骤 10: 测试验证（24/24 通过）
- [x] 步骤 11: 提交代码

## 关键变更

### LLM 层 (internal/llm/)
- `types.go`: Message 扩展 ToolCalls/ToolCallID 字段，新增 StreamEvent
- `client.go`: ChatStream 返回统一 StreamEvent channel，累积流式 tool_calls

### Agent 层 (internal/agent/)
- `brain.go`: ChatStream 实现 Agentic 循环（stream -> tool_calls -> execute -> re-stream），最多 10 轮

### TUI 层 (internal/tui/)
- `session.go`: Session 结构/输入历史/taskRunning 跟踪
- `styles.go`: 气泡/Tab/Overlay 样式
- `view.go`: 气泡渲染/Tab 栏/补全提示/Ctrl+O 面板
- `commands.go`: 所有斜杠命令
- `keys.go`: 按键路由/双击检测
- `app.go`: 多 Session 容器/流式事件处理
