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

	// Ctrl+W 关闭（应该关闭第1个，切到第2个）
	ctrlW := tea.KeyMsg{Type: tea.KeyCtrlW}
	model, _ = app.Update(ctrlW)
	app = model.(*App)

	if len(app.sessions) != 1 {
		t.Fatalf("关闭后应有 1 个会话, 实际 %d", len(app.sessions))
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

	rendered := renderMessages(msgs, 80)
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
