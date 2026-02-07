package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestCompletionIntegration 测试补全触发的完整流程
func TestCompletionIntegration(t *testing.T) {
	app := NewApp()

	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = model.(*App)

	if !app.ready {
		t.Fatal("WindowSizeMsg 后 app 应该 ready")
	}

	t.Logf("初始 textarea value: %q", app.textarea.Value())
	t.Logf("初始 textarea focused: %v", app.textarea.Focused())

	// 输入 "/"
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	model, _ = app.Update(keyMsg)
	app = model.(*App)

	if app.textarea.Value() != "/" {
		t.Errorf("输入 '/' 后 textarea 值应为 '/', 实际为 %q", app.textarea.Value())
	}
	if app.completionHint == "" {
		t.Error("输入 '/' 后 completionHint 不应为空")
	}
	if app.suggestion == "" {
		t.Error("输入 '/' 后 suggestion 不应为空")
	}

	// 输入 "m"
	keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}
	model, _ = app.Update(keyMsg)
	app = model.(*App)

	if app.textarea.Value() != "/m" {
		t.Errorf("值应为 '/m', 实际为 %q", app.textarea.Value())
	}

	// 输入 "o"
	keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}}
	model, _ = app.Update(keyMsg)
	app = model.(*App)

	if app.completionHint == "" {
		t.Error("输入 '/mo' 后 completionHint 不应为空")
	}
	if app.suggestion == "" {
		t.Error("输入 '/mo' 后 suggestion 不应为空")
	}

	// Tab 接受补全
	savedSugg := app.suggestion
	if savedSugg != "" {
		tabMsg := tea.KeyMsg{Type: tea.KeyTab}
		model, _ = app.Update(tabMsg)
		app = model.(*App)

		if app.textarea.Value() != savedSugg {
			t.Errorf("Tab 后 textarea 值应为 %q, 实际为 %q", savedSugg, app.textarea.Value())
		}
		if app.suggestion != "" {
			t.Error("Tab 后 suggestion 应被清空")
		}
	}
}

// TestViewLayout 测试布局行数
func TestViewLayout(t *testing.T) {
	app := NewApp()
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = model.(*App)

	view := app.View()
	lines := strings.Split(view, "\n")
	t.Logf("无补全时 View 行数: %d", len(lines))

	// 输入 "/" 触发补全
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	model, _ = app.Update(keyMsg)
	app = model.(*App)

	view = app.View()
	lines = strings.Split(view, "\n")
	t.Logf("有补全时 View 行数: %d", len(lines))

	hintFound := false
	for i, line := range lines {
		if strings.Contains(line, "[Tab]") && strings.Contains(line, "/help") {
			hintFound = true
			t.Logf("补全提示行在第 %d 行", i+1)
			break
		}
	}
	if !hintFound {
		t.Error("View 输出中找不到补全提示行")
	}
}

// TestMultiSession 测试多会话管理
func TestMultiSession(t *testing.T) {
	app := NewApp()
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = model.(*App)

	// 初始应有 1 个会话
	if len(app.sessions) != 1 {
		t.Fatalf("初始应有 1 个会话, 实际 %d", len(app.sessions))
	}

	// Ctrl+T 新建会话
	ctrlT := tea.KeyMsg{Type: tea.KeyCtrlT}
	model, _ = app.Update(ctrlT)
	app = model.(*App)

	if len(app.sessions) != 2 {
		t.Fatalf("新建后应有 2 个会话, 实际 %d", len(app.sessions))
	}
	if app.activeIdx != 1 {
		t.Errorf("新建后应在第 2 个会话, 实际在 %d", app.activeIdx+1)
	}

	// Alt+1 切换回第一个会话
	alt1 := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}, Alt: true}
	model, _ = app.Update(alt1)
	app = model.(*App)

	if app.activeIdx != 0 {
		t.Errorf("Alt+1 后应在第 1 个会话, 实际在 %d", app.activeIdx+1)
	}

	// Ctrl+Right 切换到下一个会话
	ctrlRight := tea.KeyMsg{Type: tea.KeyCtrlRight}
	model, _ = app.Update(ctrlRight)
	app = model.(*App)

	if app.activeIdx != 1 {
		t.Errorf("Ctrl+Right 后应在第 2 个会话, 实际在 %d", app.activeIdx+1)
	}

	// Ctrl+Left 切回上一个会话
	ctrlLeft := tea.KeyMsg{Type: tea.KeyCtrlLeft}
	model, _ = app.Update(ctrlLeft)
	app = model.(*App)

	if app.activeIdx != 0 {
		t.Errorf("Ctrl+Left 后应在第 1 个会话, 实际在 %d", app.activeIdx+1)
	}

	// Ctrl+Left 循环到最后一个
	model, _ = app.Update(ctrlLeft)
	app = model.(*App)

	if app.activeIdx != 1 {
		t.Errorf("Ctrl+Left 循环后应在第 2 个会话, 实际在 %d", app.activeIdx+1)
	}

	// Ctrl+] 也能切换（备用）
	ctrlBracket := tea.KeyMsg{Type: tea.KeyCtrlCloseBracket}
	model, _ = app.Update(ctrlBracket)
	app = model.(*App)

	if app.activeIdx != 0 {
		t.Errorf("Ctrl+] 后应在第 1 个会话, 实际在 %d", app.activeIdx+1)
	}

	// Ctrl+W 关闭第 1 个会话（应切到剩余的会话）
	ctrlW := tea.KeyMsg{Type: tea.KeyCtrlW}
	model, _ = app.Update(ctrlW)
	app = model.(*App)

	if len(app.sessions) != 1 {
		t.Fatalf("关闭后应有 1 个会话, 实际 %d", len(app.sessions))
	}
}

// TestSessionTextareaPersistence 测试会话切换时输入框内容持久化
func TestSessionTextareaPersistence(t *testing.T) {
	app := NewApp()
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = model.(*App)

	// 在会话 1 中输入一些文字
	for _, r := range "hello from session 1" {
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		model, _ = app.Update(keyMsg)
		app = model.(*App)
	}

	if app.textarea.Value() != "hello from session 1" {
		t.Fatalf("会话1输入应为 'hello from session 1', 实际 %q", app.textarea.Value())
	}

	// Ctrl+T 新建会话 2
	ctrlT := tea.KeyMsg{Type: tea.KeyCtrlT}
	model, _ = app.Update(ctrlT)
	app = model.(*App)

	// 新会话输入框应为空
	if app.textarea.Value() != "" {
		t.Errorf("新会话输入框应为空, 实际 %q", app.textarea.Value())
	}

	// 在会话 2 中输入
	for _, r := range "session 2 text" {
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		model, _ = app.Update(keyMsg)
		app = model.(*App)
	}

	// 切回会话 1 (Alt+1)
	alt1 := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}, Alt: true}
	model, _ = app.Update(alt1)
	app = model.(*App)

	if app.textarea.Value() != "hello from session 1" {
		t.Errorf("切回会话1后输入框应为 'hello from session 1', 实际 %q", app.textarea.Value())
	}

	// 切回会话 2 (Ctrl+Right)
	ctrlRight := tea.KeyMsg{Type: tea.KeyCtrlRight}
	model, _ = app.Update(ctrlRight)
	app = model.(*App)

	if app.textarea.Value() != "session 2 text" {
		t.Errorf("切回会话2后输入框应为 'session 2 text', 实际 %q", app.textarea.Value())
	}

	// Ctrl+Left 切回会话 1
	ctrlLeft := tea.KeyMsg{Type: tea.KeyCtrlLeft}
	model, _ = app.Update(ctrlLeft)
	app = model.(*App)

	if app.textarea.Value() != "hello from session 1" {
		t.Errorf("Ctrl+Left切回会话1后输入框应为 'hello from session 1', 实际 %q", app.textarea.Value())
	}
}

// TestSessionIDUniqueness 测试会话 ID 唯一性（关闭后新建不重复）
func TestSessionIDUniqueness(t *testing.T) {
	app := NewApp()
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = model.(*App)

	firstID := app.sessions[0].id

	// 新建会话 2
	ctrlT := tea.KeyMsg{Type: tea.KeyCtrlT}
	model, _ = app.Update(ctrlT)
	app = model.(*App)
	secondID := app.sessions[1].id

	if secondID <= firstID {
		t.Errorf("第二个会话 ID(%d) 应大于第一个(%d)", secondID, firstID)
	}

	// 关闭会话 2
	ctrlW := tea.KeyMsg{Type: tea.KeyCtrlW}
	model, _ = app.Update(ctrlW)
	app = model.(*App)

	// 新建会话 3
	model, _ = app.Update(ctrlT)
	app = model.(*App)
	thirdID := app.sessions[1].id

	if thirdID <= secondID {
		t.Errorf("第三个会话 ID(%d) 应大于第二个(%d), 不应重复", thirdID, secondID)
	}
}

// TestDoubleCtrlC 测试双击 Ctrl+C 退出
func TestDoubleCtrlC(t *testing.T) {
	app := NewApp()
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = model.(*App)

	// 第一次 Ctrl+C
	ctrlC := tea.KeyMsg{Type: tea.KeyCtrlC}
	model, cmd := app.Update(ctrlC)
	app = model.(*App)

	if cmd != nil {
		t.Error("第一次 Ctrl+C 不应退出")
	}

	// 第二次 Ctrl+C（立即）
	model, cmd = app.Update(ctrlC)
	// cmd 应该是 tea.Quit
	if cmd == nil {
		t.Error("第二次 Ctrl+C 应触发退出")
	}
}

// TestInputHistory 测试输入历史
func TestInputHistory(t *testing.T) {
	app := NewApp()
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = model.(*App)

	sess := app.currentSession()

	// 模拟发送两条消息到历史
	sess.PushHistory("hello world")
	sess.PushHistory("second message")

	// Up 应该返回最近的消息
	text, ok := sess.HistoryUp("")
	if !ok || text != "second message" {
		t.Errorf("第一次 Up 应返回 'second message', 实际 %q, ok=%v", text, ok)
	}

	text, ok = sess.HistoryUp("")
	if !ok || text != "hello world" {
		t.Errorf("第二次 Up 应返回 'hello world', 实际 %q, ok=%v", text, ok)
	}

	// Down 回到 second
	text, ok = sess.HistoryDown()
	if !ok || text != "second message" {
		t.Errorf("Down 应返回 'second message', 实际 %q", text)
	}

	// Down 回到原始输入
	text, ok = sess.HistoryDown()
	if !ok || text != "" {
		t.Errorf("再次 Down 应返回空, 实际 %q", text)
	}
}

// TestCtrlJ 测试 Ctrl+J 换行
func TestCtrlJ(t *testing.T) {
	app := NewApp()
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = model.(*App)

	// 先输入一些文字
	for _, r := range "hello" {
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		model, _ = app.Update(keyMsg)
		app = model.(*App)
	}

	// Ctrl+J 换行
	ctrlJ := tea.KeyMsg{Type: tea.KeyCtrlJ}
	model, _ = app.Update(ctrlJ)
	app = model.(*App)

	val := app.textarea.Value()
	if !strings.Contains(val, "\n") {
		t.Errorf("Ctrl+J 后应包含换行, 实际值 %q", val)
	}
}

// TestBubbleRendering 测试气泡渲染
func TestBubbleRendering(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}

	rendered := renderMessages(msgs, 80, false, 0)
	if !strings.Contains(rendered, "You") {
		t.Error("应包含 'You' 标签")
	}
	if !strings.Contains(rendered, "Kele") {
		t.Error("应包含 'Kele' 标签")
	}
	if !strings.Contains(rendered, "hello") {
		t.Error("应包含用户消息 'hello'")
	}
	if !strings.Contains(rendered, "hi there") {
		t.Error("应包含 AI 消息 'hi there'")
	}
	t.Logf("渲染结果:\n%s", rendered)
}

// TestCtrlO 测试 Ctrl+O 设置面板
func TestCtrlO(t *testing.T) {
	app := NewApp()
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = model.(*App)

	// 打开设置
	ctrlO := tea.KeyMsg{Type: tea.KeyCtrlO}
	model, _ = app.Update(ctrlO)
	app = model.(*App)

	if app.overlayMode != "settings" {
		t.Error("Ctrl+O 应打开设置面板")
	}

	view := app.View()
	if !strings.Contains(view, "Settings") {
		t.Error("设置面板应包含 'Settings'")
	}

	// 再按 Ctrl+O 关闭
	model, _ = app.Update(ctrlO)
	app = model.(*App)

	if app.overlayMode != "" {
		t.Error("再按 Ctrl+O 应关闭设置面板")
	}
}

// TestThinkingBlockRendering 测试 Thinking 块渲染
func TestThinkingBlockRendering(t *testing.T) {
	// 流式中无 thinking 内容 - 显示动画
	msgs := []Message{
		{Role: "assistant", Content: "", IsStream: true},
	}
	rendered := renderMessages(msgs, 80, false, 0)
	if !strings.Contains(rendered, "Thinking") {
		t.Error("流式中应显示 Thinking 动画")
	}

	// 流式中有 thinking 内容
	msgs = []Message{
		{Role: "assistant", Content: "", Thinking: "let me analyze this", IsStream: true},
	}
	rendered = renderMessages(msgs, 80, false, 0)
	if !strings.Contains(rendered, "Thinking") {
		t.Error("有 thinking 内容时应显示 Thinking 标签")
	}
	if !strings.Contains(rendered, "let me analyze this") {
		t.Error("应包含 thinking 内容")
	}

	// 完成后折叠状态
	msgs = []Message{
		{Role: "assistant", Content: "the answer is 42", Thinking: "thinking line 1\nthinking line 2", IsStream: false},
	}
	rendered = renderMessages(msgs, 80, false, 0)
	if !strings.Contains(rendered, "Thinking") {
		t.Error("完成后折叠应显示 Thinking 标签")
	}
	if !strings.Contains(rendered, "the answer is 42") {
		t.Error("应包含回答内容")
	}

	// 完成后展开状态
	rendered = renderMessages(msgs, 80, true, 0)
	if !strings.Contains(rendered, "thinking line 1") {
		t.Error("展开后应显示完整 thinking 内容")
	}
	if !strings.Contains(rendered, "thinking line 2") {
		t.Error("展开后应显示所有 thinking 行")
	}

	t.Logf("折叠渲染:\n%s", renderMessages(msgs, 80, false, 0))
	t.Logf("展开渲染:\n%s", renderMessages(msgs, 80, true, 0))
}

// TestCtrlE 测试 Ctrl+E 切换 Thinking 展开/折叠
func TestCtrlE(t *testing.T) {
	app := NewApp()
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = model.(*App)

	// 默认折叠
	if app.thinkingExpanded {
		t.Error("默认应为折叠状态")
	}

	// Ctrl+E 展开
	ctrlE := tea.KeyMsg{Type: tea.KeyCtrlE}
	model, _ = app.Update(ctrlE)
	app = model.(*App)

	if !app.thinkingExpanded {
		t.Error("Ctrl+E 后应为展开状态")
	}

	// 再次 Ctrl+E 折叠
	model, _ = app.Update(ctrlE)
	app = model.(*App)

	if app.thinkingExpanded {
		t.Error("再次 Ctrl+E 后应回到折叠状态")
	}
}

// TestCompletionState 测试补全状态追踪
func TestCompletionState(t *testing.T) {
	app := NewApp()
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = model.(*App)

	// 初始状态为空
	if app.completionState != "" {
		t.Errorf("初始补全状态应为空, 实际 %q", app.completionState)
	}

	// 输入 "/" 触发本地补全 -> 状态变为 done
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	model, _ = app.Update(keyMsg)
	app = model.(*App)

	if app.completionState != "done" {
		t.Errorf("本地补全后状态应为 'done', 实际 %q", app.completionState)
	}

	// 清空输入 -> 状态重置
	app.textarea.SetValue("")
	// 手动触发输入变化
	app.onInputChanged("")
	if app.completionState != "" {
		t.Errorf("清空输入后补全状态应为空, 实际 %q", app.completionState)
	}
}

// TestCompletionStatusRendering 测试补全状态指示器渲染
func TestCompletionStatusRendering(t *testing.T) {
	// loading 状态
	status := renderCompletionStatus("loading", "", 0)
	if !strings.Contains(status, "AI") {
		t.Error("loading 状态应包含 AI 标记")
	}

	// error 状态
	status = renderCompletionStatus("error", "connection timeout", 0)
	if !strings.Contains(status, "connection timeout") {
		t.Error("error 状态应包含错误信息")
	}

	// done 状态
	status = renderCompletionStatus("done", "", 0)
	if !strings.Contains(status, "OK") {
		t.Error("done 状态应显示 OK")
	}

	// idle 状态
	status = renderCompletionStatus("", "", 0)
	if status != "" {
		t.Error("idle 状态应为空")
	}
}

// TestStreamMsgThinking 测试流式消息中的 thinking 处理
func TestStreamMsgThinking(t *testing.T) {
	app := NewApp()
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = model.(*App)

	sess := app.currentSession()
	sess.AddMessage("assistant", "")
	sess.messages[len(sess.messages)-1].IsStream = true
	sess.streaming = true

	// 模拟收到 thinking 消息
	thinkMsg := streamMsg{
		sessionID: sess.id,
		thinking:  "analyzing the problem",
	}
	app.handleStreamMsg(thinkMsg)

	if sess.thinkingBuffer != "analyzing the problem" {
		t.Errorf("thinkingBuffer 应为 'analyzing the problem', 实际 %q", sess.thinkingBuffer)
	}

	lastMsg := sess.messages[len(sess.messages)-1]
	if lastMsg.Thinking != "analyzing the problem" {
		t.Errorf("消息 Thinking 应为 'analyzing the problem', 实际 %q", lastMsg.Thinking)
	}

	// 模拟收到更多 thinking
	thinkMsg2 := streamMsg{
		sessionID: sess.id,
		thinking:  " step by step",
	}
	app.handleStreamMsg(thinkMsg2)

	if sess.thinkingBuffer != "analyzing the problem step by step" {
		t.Errorf("thinkingBuffer 应累积, 实际 %q", sess.thinkingBuffer)
	}

	// 模拟 done
	doneMsg := streamMsg{
		sessionID: sess.id,
		done:      true,
	}
	app.handleStreamMsg(doneMsg)

	if sess.thinkingBuffer != "" {
		t.Error("done 后 thinkingBuffer 应被清空")
	}
	if sess.streaming {
		t.Error("done 后 streaming 应为 false")
	}
}

// TestThinkingAnimationStops 测试 content 开始后 thinking 动画停止
func TestThinkingAnimationStops(t *testing.T) {
	// 流式中只有 thinking（无 content）→ thinkingActive=true → 显示 spinner
	msg := Message{Role: "assistant", Thinking: "analyzing...", Content: "", IsStream: true}
	rendered := renderMessages([]Message{msg}, 80, false, 3)
	if !strings.Contains(rendered, spinnerFrames[3]) {
		t.Error("thinking 阶段应显示 spinner 动画")
	}

	// 流式中 thinking+content → thinkingActive=false → 不显示 spinner
	msg.Content = "the answer is"
	rendered = renderMessages([]Message{msg}, 80, false, 3)
	// 折叠的 thinking 不应有 spinner
	if strings.Contains(rendered, spinnerFrames[3]) {
		t.Error("content 开始后 thinking 块不应显示 spinner")
	}
	if !strings.Contains(rendered, "Thinking") {
		t.Error("content 开始后仍应显示 Thinking 标签（折叠）")
	}
}

// TestTabForceCompletion 测试 Tab 强制触发补全
func TestTabForceCompletion(t *testing.T) {
	app := NewApp()
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = model.(*App)

	// 输入普通文本（不是 / 或 @）
	for _, r := range "hello world" {
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		model, _ = app.Update(keyMsg)
		app = model.(*App)
	}

	// 确认无 suggestion
	if app.suggestion != "" {
		t.Skipf("已有 suggestion %q, 跳过强制补全测试", app.suggestion)
	}

	// Tab 应触发强制补全（返回 cmd 不为 nil）
	// 注意：实际 API 调用需要 key，这里只验证状态变化
	tabMsg := tea.KeyMsg{Type: tea.KeyTab}
	model, cmd := app.Update(tabMsg)
	app = model.(*App)

	// forceComplete 应设置 loading 状态
	if app.completionState != "loading" {
		t.Errorf("Tab 强制补全后状态应为 'loading', 实际 %q", app.completionState)
	}
	if cmd == nil {
		t.Error("Tab 强制补全应返回非 nil cmd")
	}
}

// TestShouldTickPrecision 测试 ticker 精确性
func TestShouldTickPrecision(t *testing.T) {
	app := NewApp()
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = model.(*App)

	// 初始状态不需要 tick
	if app.shouldTick() {
		t.Error("初始状态不应需要 tick")
	}

	// streaming 但无 content（thinking 阶段）→ 需要 tick
	sess := app.currentSession()
	sess.streaming = true
	sess.streamBuffer = ""
	if !app.shouldTick() {
		t.Error("thinking 阶段应需要 tick")
	}

	// streaming 且有 content → 不需要 tick
	sess.streamBuffer = "some content"
	if app.shouldTick() {
		t.Error("content 阶段不应需要 tick")
	}

	// completionState=loading → 需要 tick
	sess.streaming = false
	sess.streamBuffer = ""
	app.completionState = "loading"
	if !app.shouldTick() {
		t.Error("补全加载中应需要 tick")
	}

	// completionState=done → 不需要 tick
	app.completionState = "done"
	if app.shouldTick() {
		t.Error("补全完成不应需要 tick")
	}
}
