package config

import (
	"database/sql"
	"fmt"
	"time"
)

// ProviderProfile 自定义供应商配置
type ProviderProfile struct {
	Name         string // 唯一标识: "z-ai", "openrouter"
	Type         string // "openai" | "anthropic" | "ollama"
	APIBase      string // "https://api.z.ai/v1"
	APIKey       string
	DefaultModel string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ensureProvidersTable 确保 providers 表存在
func ensureProvidersTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS providers (
			name TEXT PRIMARY KEY,
			type TEXT NOT NULL DEFAULT 'openai',
			api_base TEXT NOT NULL,
			api_key TEXT NOT NULL DEFAULT '',
			default_model TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// AddProvider 添加自定义供应商
func AddProvider(p ProviderProfile) error {
	db, err := openConfigDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if err := ensureProvidersTable(db); err != nil {
		return err
	}

	_, err = db.Exec(`
		INSERT INTO providers (name, type, api_base, api_key, default_model)
		VALUES (?, ?, ?, ?, ?)
	`, p.Name, p.Type, p.APIBase, p.APIKey, p.DefaultModel)
	if err != nil {
		return fmt.Errorf("供应商 %q 已存在或写入失败: %w", p.Name, err)
	}
	return nil
}

// GetProvider 获取指定供应商
func GetProvider(name string) (*ProviderProfile, error) {
	db, err := openConfigDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if err := ensureProvidersTable(db); err != nil {
		return nil, err
	}

	var p ProviderProfile
	err = db.QueryRow(`
		SELECT name, type, api_base, api_key, default_model, created_at, updated_at
		FROM providers WHERE name = ?
	`, name).Scan(&p.Name, &p.Type, &p.APIBase, &p.APIKey, &p.DefaultModel, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("供应商不存在: %s", name)
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListProviderProfiles 列出所有自定义供应商
func ListProviderProfiles() ([]ProviderProfile, error) {
	db, err := openConfigDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if err := ensureProvidersTable(db); err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT name, type, api_base, api_key, default_model, created_at, updated_at
		FROM providers ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ProviderProfile
	for rows.Next() {
		var p ProviderProfile
		if err := rows.Scan(&p.Name, &p.Type, &p.APIBase, &p.APIKey, &p.DefaultModel, &p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}
		result = append(result, p)
	}
	return result, nil
}

// UpdateProviderField 更新供应商的单个字段
func UpdateProviderField(name, field, value string) error {
	// 白名单校验
	allowed := map[string]bool{
		"api_base": true, "api_key": true, "default_model": true, "type": true,
	}
	if !allowed[field] {
		return fmt.Errorf("不支持的字段: %s (可用: api_base, api_key, default_model, type)", field)
	}

	db, err := openConfigDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if err := ensureProvidersTable(db); err != nil {
		return err
	}

	// 使用白名单确保安全的列名拼接
	query := fmt.Sprintf("UPDATE providers SET %s = ?, updated_at = CURRENT_TIMESTAMP WHERE name = ?", field)
	res, err := db.Exec(query, value, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("供应商不存在: %s", name)
	}
	return nil
}

// RemoveProvider 删除自定义供应商
func RemoveProvider(name string) error {
	db, err := openConfigDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if err := ensureProvidersTable(db); err != nil {
		return err
	}

	res, err := db.Exec("DELETE FROM providers WHERE name = ?", name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("供应商不存在: %s", name)
	}
	return nil
}

// MaskProviderKey 对供应商 Key 脱敏
func MaskProviderKey(key string) string {
	return maskSecret(key)
}
