package config

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	_ "github.com/mattn/go-sqlite3"
)

// ConfigStorePath 返回配置存储路径（SQLite DB）
func ConfigStorePath() string {
	return configDBPath()
}

// configDBPath 返回数据库路径，和 memory.db 共用
func configDBPath() string {
	if v := os.Getenv("KELE_DB_PATH"); v != "" {
		return v
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".kele", "memory.db")
}

// openConfigDB 打开数据库并确保 system_settings 表存在
func openConfigDB() (*sql.DB, error) {
	dbPath := configDBPath()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS system_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// GetValue 从 DB 获取配置值
func GetValue(key string) (string, error) {
	db, err := openConfigDB()
	if err != nil {
		return "", err
	}
	defer db.Close()

	var value string
	err = db.QueryRow("SELECT value FROM system_settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("key not found: %s", key)
	}
	return value, err
}

// SetValue 设置配置值到 DB
func SetValue(key, value string) error {
	db, err := openConfigDB()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(`
		INSERT INTO system_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = CURRENT_TIMESTAMP
	`, key, value, value)
	return err
}

// DeleteValue 删除配置项
func DeleteValue(key string) error {
	db, err := openConfigDB()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec("DELETE FROM system_settings WHERE key = ?", key)
	return err
}

// ListValues 列出所有配置项
func ListValues() (map[string]string, error) {
	db, err := openConfigDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query("SELECT key, value FROM system_settings ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		result[k] = v
	}
	return result, nil
}

// ApplyToConfig 从 DB 加载配置覆盖到 Config 结构体
func ApplyToConfig(cfg *Config) {
	db, err := openConfigDB()
	if err != nil {
		return
	}
	defer db.Close()

	entries := loadAllEntries(db)
	if len(entries) == 0 {
		return
	}

	// LLM
	applyStr(entries, "llm.openai_api_base", &cfg.LLM.OpenAIAPIBase)
	applyStr(entries, "llm.openai_api_key", &cfg.LLM.OpenAIAPIKey)
	applyStr(entries, "llm.openai_model", &cfg.LLM.OpenAIModel)
	applyStr(entries, "llm.small_model", &cfg.LLM.SmallModel)
	applyFloat(entries, "llm.temperature", &cfg.LLM.Temperature)
	applyInt(entries, "llm.max_tokens", &cfg.LLM.MaxTokens)
	applyStr(entries, "llm.anthropic_api_key", &cfg.LLM.AnthropicAPIKey)
	applyStr(entries, "llm.anthropic_api_base", &cfg.LLM.AnthropicAPIBase)
	applyStr(entries, "llm.ollama_host", &cfg.LLM.OllamaHost)
	applyInt(entries, "llm.max_tool_rounds", &cfg.LLM.MaxToolRounds)
	applyInt(entries, "llm.max_turns", &cfg.LLM.MaxTurns)
	applyInt(entries, "llm.complete_timeout", &cfg.LLM.CompleteTimeout)

	// Tools
	applyInt(entries, "tools.bash_timeout", &cfg.Tools.BashTimeout)
	applyInt(entries, "tools.max_output_size", &cfg.Tools.MaxOutputSize)
	applyInt(entries, "tools.max_write_size", &cfg.Tools.MaxWriteSize)

	// TUI
	applyInt(entries, "tui.max_sessions", &cfg.TUI.MaxSessions)
	applyInt(entries, "tui.max_input_chars", &cfg.TUI.MaxInputChars)

	// Cron
	applyInt(entries, "cron.job_timeout", &cfg.Cron.JobTimeout)
	applyInt(entries, "cron.log_retention", &cfg.Cron.LogRetention)
	applyInt(entries, "cron.max_concurrent", &cfg.Cron.MaxConcurrent)

	// Telegram
	applyStr(entries, "telegram.bot_token", &cfg.Telegram.BotToken)
	applyInt64(entries, "telegram.allowed_chat", &cfg.Telegram.AllowedChat)
}

// --- 内部辅助函数 ---

func loadAllEntries(db *sql.DB) map[string]string {
	entries := make(map[string]string)
	rows, err := db.Query("SELECT key, value FROM system_settings")
	if err != nil {
		return entries
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		entries[k] = v
	}
	return entries
}

func applyStr(entries map[string]string, key string, target *string) {
	if v, ok := entries[key]; ok && v != "" {
		*target = v
	}
}

func applyInt(entries map[string]string, key string, target *int) {
	if v, ok := entries[key]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			*target = n
		}
	}
}

func applyInt64(entries map[string]string, key string, target *int64) {
	if v, ok := entries[key]; ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			*target = n
		}
	}
}

func applyFloat(entries map[string]string, key string, target *float64) {
	if v, ok := entries[key]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			*target = f
		}
	}
}

// AllSettings 从 Config 结构体导出所有可配置项的当前生效值
func AllSettings(cfg *Config) map[string]string {
	m := map[string]string{
		// LLM
		"llm.openai_api_base":  cfg.LLM.OpenAIAPIBase,
		"llm.openai_api_key":   maskSecret(cfg.LLM.OpenAIAPIKey),
		"llm.openai_model":     cfg.LLM.OpenAIModel,
		"llm.small_model":      cfg.LLM.SmallModel,
		"llm.temperature":      strconv.FormatFloat(cfg.LLM.Temperature, 'f', -1, 64),
		"llm.max_tokens":       strconv.Itoa(cfg.LLM.MaxTokens),
		"llm.anthropic_api_key":  maskSecret(cfg.LLM.AnthropicAPIKey),
		"llm.anthropic_api_base": cfg.LLM.AnthropicAPIBase,
		"llm.ollama_host":      cfg.LLM.OllamaHost,
		"llm.max_tool_rounds":  strconv.Itoa(cfg.LLM.MaxToolRounds),
		"llm.max_turns":        strconv.Itoa(cfg.LLM.MaxTurns),
		"llm.complete_timeout": strconv.Itoa(cfg.LLM.CompleteTimeout),

		// Tools
		"tools.bash_timeout":    strconv.Itoa(cfg.Tools.BashTimeout),
		"tools.max_output_size": strconv.Itoa(cfg.Tools.MaxOutputSize),
		"tools.max_write_size":  strconv.Itoa(cfg.Tools.MaxWriteSize),

		// TUI
		"tui.max_sessions":   strconv.Itoa(cfg.TUI.MaxSessions),
		"tui.max_input_chars": strconv.Itoa(cfg.TUI.MaxInputChars),

		// Cron
		"cron.job_timeout":    strconv.Itoa(cfg.Cron.JobTimeout),
		"cron.log_retention":  strconv.Itoa(cfg.Cron.LogRetention),
		"cron.max_concurrent": strconv.Itoa(cfg.Cron.MaxConcurrent),

		// Telegram
		"telegram.bot_token":   maskSecret(cfg.Telegram.BotToken),
		"telegram.allowed_chat": strconv.FormatInt(cfg.Telegram.AllowedChat, 10),
	}
	return m
}

// maskSecret 敏感值脱敏，只显示前4和后4位
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 12 {
		return s[:2] + "****" + s[len(s)-2:]
	}
	return s[:4] + "****" + s[len(s)-4:]
}
