# Thinking 展示 + 补全状态优化

> 创建: 2026-02-07
> 状态: 已完成

## 目标

1. 显示 AI 思考过程（支持 reasoning_content），可展开/折叠，有动画
2. 在界面上显示独立的补全状态指示器

## 步骤

### Feature 1: Thinking 展示

- [ ] LLM 层: StreamChunk.Delta 添加 ReasoningContent 字段
- [ ] LLM 层: ChatStream 发送 reasoning 事件
- [ ] Agent 层: Brain.ChatStream 转发 reasoning 事件
- [ ] TUI 层: streamEvent 添加 thinking 类型
- [ ] TUI 层: Message 添加 Thinking 字段
- [ ] TUI 层: Session 添加 thinkingBuffer
- [ ] TUI 层: App 添加 thinkingExpanded, thinkingFrame
- [ ] TUI 层: tickMsg 定时器驱动动画
- [ ] TUI 层: handleStreamMsg 处理 thinking 事件
- [ ] TUI 层: view.go 渲染 thinking 块（展开/折叠/动画）
- [ ] TUI 层: styles.go 添加 thinking 样式
- [ ] TUI 层: keys.go 添加 Ctrl+E 切换展开/折叠

### Feature 2: 补全状态

- [ ] completion.go: completionMsg 携带 error 信息
- [ ] app.go: 添加 completionState 字段 (idle/pending/loading/done/error)
- [ ] view.go: 渲染补全状态指示器
- [ ] 补全加载时显示 spinner 动画

### 测试与收尾

- [ ] 更新测试
- [ ] 编译验证
- [ ] 提交

## 设计

### Thinking 块布局

流式中（展开 + 动画）:
```
  Kele
  [思考 ⠋] 正在分析你的问题...
  [思考 ⠋] 第一步考虑...

  实际回答内容流式输出...
```

完成后（折叠）:
```
  Kele
  [思考] ...最新思考内容片段 (共8行, Ctrl+E展开)

  完整回答内容
```

完成后（展开，Ctrl+E）:
```
  Kele
  [思考] 完整思考内容...
  [思考] 第一步...
  [思考] 第二步...

  完整回答内容
```

### 补全状态指示器

显示在补全提示行右侧:
```
[Tab] /help  /history             [AI: 请求中 ⠋]
                                  [AI: 失败]
```

### 动画

- Spinner: braille 点阵 ["⠋","⠙","⠹","⠸","⠼","⠴","⠦","⠧","⠇","⠏"]
- Tick: 100ms 刷新
- 触发条件: streaming 或 completionState=="loading"

### 快捷键

- Ctrl+E: 切换所有 thinking 块展开/折叠
- 流式中的 thinking 块始终展开

## 完成标准

- [ ] DeepSeek reasoning_content 正确显示
- [ ] 无 reasoning 的模型也能显示 thinking 动画
- [ ] Ctrl+E 正确切换展开/折叠
- [ ] 补全状态实时更新（pending/loading/done/error）
- [ ] 所有测试通过
