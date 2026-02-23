package config

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestDB(t *testing.T) func() {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	os.Setenv("KELE_DB_PATH", dbPath)
	return func() {
		os.Unsetenv("KELE_DB_PATH")
	}
}

func TestProviderCRUD(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 添加
	err := AddProvider(ProviderProfile{
		Name:         "z-ai",
		Type:         "openai",
		APIBase:      "https://api.z.ai/v1",
		APIKey:       "sk-zai-test",
		DefaultModel: "deepseek-chat",
	})
	if err != nil {
		t.Fatalf("AddProvider failed: %v", err)
	}

	// 重复添加应失败
	err = AddProvider(ProviderProfile{Name: "z-ai", Type: "openai", APIBase: "https://x.com"})
	if err == nil {
		t.Fatal("重复添加应返回错误")
	}

	// 获取
	p, err := GetProvider("z-ai")
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}
	if p.APIBase != "https://api.z.ai/v1" {
		t.Errorf("APIBase = %s, want https://api.z.ai/v1", p.APIBase)
	}
	if p.DefaultModel != "deepseek-chat" {
		t.Errorf("DefaultModel = %s, want deepseek-chat", p.DefaultModel)
	}

	// 获取不存在的
	_, err = GetProvider("not-exist")
	if err == nil {
		t.Fatal("获取不存在的供应商应返回错误")
	}

	// 列表
	list, err := ListProviderProfiles()
	if err != nil {
		t.Fatalf("ListProviderProfiles failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}

	// 更新字段
	err = UpdateProviderField("z-ai", "api_base", "https://new.z.ai/v1")
	if err != nil {
		t.Fatalf("UpdateProviderField failed: %v", err)
	}
	p, _ = GetProvider("z-ai")
	if p.APIBase != "https://new.z.ai/v1" {
		t.Errorf("更新后 APIBase = %s, want https://new.z.ai/v1", p.APIBase)
	}

	// 更新不支持的字段
	err = UpdateProviderField("z-ai", "invalid_field", "xxx")
	if err == nil {
		t.Fatal("不支持的字段应返回错误")
	}

	// 更新不存在的供应商
	err = UpdateProviderField("not-exist", "api_key", "xxx")
	if err == nil {
		t.Fatal("更新不存在的供应商应返回错误")
	}

	// 删除
	err = RemoveProvider("z-ai")
	if err != nil {
		t.Fatalf("RemoveProvider failed: %v", err)
	}
	list, _ = ListProviderProfiles()
	if len(list) != 0 {
		t.Errorf("删除后 list len = %d, want 0", len(list))
	}

	// 重复删除应失败
	err = RemoveProvider("z-ai")
	if err == nil {
		t.Fatal("删除不存在的供应商应返回错误")
	}
}

func TestProviderMultiple(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	providers := []ProviderProfile{
		{Name: "openrouter", Type: "openai", APIBase: "https://openrouter.ai/api/v1", APIKey: "sk-or-test"},
		{Name: "z-ai", Type: "openai", APIBase: "https://api.z.ai/v1", APIKey: "sk-zai"},
		{Name: "custom-claude", Type: "anthropic", APIBase: "https://custom.anthropic.com", APIKey: "sk-ant"},
	}

	for _, p := range providers {
		if err := AddProvider(p); err != nil {
			t.Fatalf("AddProvider %s failed: %v", p.Name, err)
		}
	}

	list, err := ListProviderProfiles()
	if err != nil {
		t.Fatalf("ListProviderProfiles failed: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("list len = %d, want 3", len(list))
	}

	// 列表应按 name 排序
	if list[0].Name != "custom-claude" {
		t.Errorf("list[0].Name = %s, want custom-claude", list[0].Name)
	}
}
