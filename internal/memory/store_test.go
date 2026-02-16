package memory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BlakeLiAFK/kele/internal/config"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		Memory: config.MemoryConfig{
			DBPath:     filepath.Join(dir, "test.db"),
			MemoryFile: filepath.Join(dir, "MEMORY.md"),
			SessionDir: filepath.Join(dir, "sessions"),
		},
	}
	return NewStore(cfg)
}

func TestSaveAndGetMessage(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	err := store.SaveMessage("user", "hello world")
	if err != nil {
		t.Fatalf("SaveMessage 失败: %v", err)
	}

	msgs, err := store.GetRecentMessages(10)
	if err != nil {
		t.Fatalf("GetRecentMessages 失败: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("应有 1 条消息, 实际 %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello world" {
		t.Errorf("消息内容不匹配: %+v", msgs[0])
	}
}

func TestRecentMessagesOrder(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	store.SaveMessage("user", "first")
	store.SaveMessage("assistant", "second")
	store.SaveMessage("user", "third")

	msgs, err := store.GetRecentMessages(2)
	if err != nil {
		t.Fatalf("失败: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("应有 2 条消息, 实际 %d", len(msgs))
	}
	// 应按时间正序（最早在前）
	if msgs[0].Content != "second" {
		t.Errorf("第一条应为 second, 实际 %s", msgs[0].Content)
	}
	if msgs[1].Content != "third" {
		t.Errorf("第二条应为 third, 实际 %s", msgs[1].Content)
	}
}

func TestMemoryUpdateAndGet(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	err := store.UpdateMemory("project", "kele is an AI assistant")
	if err != nil {
		t.Fatalf("UpdateMemory 失败: %v", err)
	}

	val, err := store.GetMemory("project")
	if err != nil {
		t.Fatalf("GetMemory 失败: %v", err)
	}
	if val != "kele is an AI assistant" {
		t.Errorf("值应为 'kele is an AI assistant', 实际 %s", val)
	}

	// 覆盖更新
	store.UpdateMemory("project", "updated value")
	val, _ = store.GetMemory("project")
	if val != "updated value" {
		t.Errorf("更新后值应为 'updated value', 实际 %s", val)
	}
}

func TestSearchSingleKeyword(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	store.UpdateMemory("note1", "Go programming language")
	store.UpdateMemory("note2", "Python scripting")
	store.UpdateMemory("note3", "Go concurrency patterns")

	results, err := store.Search("Go", 10)
	if err != nil {
		t.Fatalf("Search 失败: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("应找到 2 条含 Go 的结果, 实际 %d", len(results))
	}
}

func TestSearchMultiKeyword(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	store.UpdateMemory("note1", "Go programming language")
	store.UpdateMemory("note2", "Go concurrency patterns")
	store.UpdateMemory("note3", "Python scripting")

	// AND 搜索
	results, err := store.Search("Go concurrency", 10)
	if err != nil {
		t.Fatalf("Search 失败: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("AND 搜索应找到 1 条, 实际 %d", len(results))
	}
}

func TestSearchNoResults(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	store.UpdateMemory("note1", "Go programming")

	results, err := store.Search("Java", 10)
	if err != nil {
		t.Fatalf("Search 失败: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("搜索 Java 应无结果, 实际 %d", len(results))
	}
}

func TestSessionPersistence(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	msgs := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}

	err := store.SaveSession("test-session-1", msgs)
	if err != nil {
		t.Fatalf("SaveSession 失败: %v", err)
	}

	// 加载
	loaded, err := store.LoadSession("test-session-1")
	if err != nil {
		t.Fatalf("LoadSession 失败: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("应加载 2 条消息, 实际 %d", len(loaded))
	}
	if loaded[0].Role != "user" || loaded[0].Content != "hello" {
		t.Errorf("第一条消息不匹配: %+v", loaded[0])
	}
	if loaded[1].Role != "assistant" || loaded[1].Content != "hi there" {
		t.Errorf("第二条消息不匹配: %+v", loaded[1])
	}
}

func TestListSessions(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	store.SaveSession("s1", []Message{{Role: "user", Content: "first session"}})
	store.SaveSession("s2", []Message{{Role: "user", Content: "second session"}})

	sessions, err := store.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions 失败: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("应有 2 个会话, 实际 %d", len(sessions))
	}
}

func TestMemoryFileSync(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	store.UpdateMemory("test_key", "test_value")

	// 检查文件是否生成
	content, err := os.ReadFile(store.memoryFile)
	if err != nil {
		t.Fatalf("读取记忆文件失败: %v", err)
	}
	if len(content) == 0 {
		t.Error("记忆文件不应为空")
	}
}

func TestLoadSessionNotFound(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	_, err := store.LoadSession("nonexistent")
	if err == nil {
		t.Error("加载不存在的会话应报错")
	}
}
