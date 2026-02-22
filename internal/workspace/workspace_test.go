package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateAndList(t *testing.T) {
	dir := t.TempDir()
	m := newManagerWithDir(dir)

	// 创建工作空间
	path, err := m.Create("test-project")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if path != filepath.Join(dir, "test-project") {
		t.Fatalf("unexpected path: %s", path)
	}

	// 验证目录存在
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory")
	}

	// 列表应包含
	list, err := m.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(list))
	}
	if list[0].Name != "test-project" {
		t.Fatalf("unexpected name: %s", list[0].Name)
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	m := newManagerWithDir(dir)

	_, err := m.Create("to-delete")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// 删除
	if err := m.Delete("to-delete"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// 列表应为空
	list, err := m.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 workspaces, got %d", len(list))
	}
}

func TestDeleteNonExistent(t *testing.T) {
	dir := t.TempDir()
	m := newManagerWithDir(dir)

	err := m.Delete("not-exist")
	if err == nil {
		t.Fatal("expected error for non-existent workspace")
	}
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	m := newManagerWithDir(dir)

	m.Create("ws1")
	m.Create("ws2")
	m.Create("ws3")

	count, err := m.Clear()
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 cleared, got %d", count)
	}

	list, err := m.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 workspaces after clear, got %d", len(list))
	}
}

func TestEnsure(t *testing.T) {
	dir := t.TempDir()
	m := newManagerWithDir(dir)

	// 第一次 Ensure 创建
	path1, err := m.Ensure("my-ws")
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}

	// 第二次 Ensure 返回相同路径
	path2, err := m.Ensure("my-ws")
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}
	if path1 != path2 {
		t.Fatalf("paths differ: %s vs %s", path1, path2)
	}
}

func TestInvalidName(t *testing.T) {
	dir := t.TempDir()
	m := newManagerWithDir(dir)

	invalidNames := []string{
		"",
		"has space",
		"has/slash",
		"has.dot",
		"too-long-name-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}

	for _, name := range invalidNames {
		_, err := m.Create(name)
		if err == nil {
			t.Errorf("expected error for invalid name %q", name)
		}
	}
}

func TestListEmptyDir(t *testing.T) {
	dir := t.TempDir()
	m := newManagerWithDir(filepath.Join(dir, "not-exist"))

	list, err := m.List()
	if err != nil {
		t.Fatalf("List on non-existent dir should not error: %v", err)
	}
	if list != nil {
		t.Fatalf("expected nil list, got %v", list)
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
	}
	for _, tc := range tests {
		got := FormatSize(tc.bytes)
		if got != tc.expected {
			t.Errorf("FormatSize(%d) = %q, want %q", tc.bytes, got, tc.expected)
		}
	}
}
