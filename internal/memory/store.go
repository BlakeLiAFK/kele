package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Store 记忆存储
type Store struct {
	db         *sql.DB
	memoryFile string
}

// NewStore 创建存储
func NewStore() *Store {
	dbPath := ".kele/memory.db"
	memoryFile := ".kele/MEMORY.md"

	// 确保目录存在
	os.MkdirAll(".kele/sessions", 0755)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		panic(err)
	}

	store := &Store{
		db:         db,
		memoryFile: memoryFile,
	}

	store.initSchema()
	return store
}

// initSchema 初始化数据库结构
func (s *Store) initSchema() {
	schema := `
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS memory_entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		key TEXT NOT NULL UNIQUE,
		value TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_memory_value ON memory_entries(value);
	`

	if _, err := s.db.Exec(schema); err != nil {
		panic(err)
	}
}

// SaveMessage 保存消息
func (s *Store) SaveMessage(role, content string) error {
	_, err := s.db.Exec(
		"INSERT INTO messages (role, content) VALUES (?, ?)",
		role, content,
	)
	return err
}

// GetRecentMessages 获取最近的消息
func (s *Store) GetRecentMessages(limit int) ([]Message, error) {
	rows, err := s.db.Query(
		"SELECT role, content, timestamp FROM messages ORDER BY id DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var timestamp string
		if err := rows.Scan(&msg.Role, &msg.Content, &timestamp); err != nil {
			return nil, err
		}
		msg.Timestamp, _ = time.Parse("2006-01-02 15:04:05", timestamp)
		messages = append(messages, msg)
	}

	// 反转顺序（最旧的在前）
	for i := 0; i < len(messages)/2; i++ {
		j := len(messages) - 1 - i
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// UpdateMemory 更新记忆条目
func (s *Store) UpdateMemory(key, value string) error {
	// 更新数据库
	_, err := s.db.Exec(
		`INSERT INTO memory_entries (key, value, updated_at)
		 VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value=?, updated_at=CURRENT_TIMESTAMP`,
		key, value, value,
	)
	if err != nil {
		return err
	}

	// 同步到 MEMORY.md
	return s.syncToFile()
}

// GetMemory 获取记忆条目
func (s *Store) GetMemory(key string) (string, error) {
	var value string
	err := s.db.QueryRow(
		"SELECT value FROM memory_entries WHERE key = ?",
		key,
	).Scan(&value)
	return value, err
}

// Search 搜索记忆
func (s *Store) Search(query string, limit int) ([]string, error) {
	rows, err := s.db.Query(
		"SELECT value FROM memory_entries WHERE value LIKE ? LIMIT ?",
		"%"+query+"%", limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			continue
		}
		results = append(results, content)
	}

	return results, nil
}

// syncToFile 同步到 MEMORY.md
func (s *Store) syncToFile() error {
	rows, err := s.db.Query(
		"SELECT key, value, updated_at FROM memory_entries ORDER BY updated_at DESC",
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	content := "# 长期记忆\n\n"
	content += fmt.Sprintf("> 最后更新: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	for rows.Next() {
		var key, value, updatedAt string
		if err := rows.Scan(&key, &value, &updatedAt); err != nil {
			continue
		}
		content += fmt.Sprintf("## %s\n\n%s\n\n", key, value)
	}

	return os.WriteFile(s.memoryFile, []byte(content), 0644)
}

// SaveSession 保存会话
func (s *Store) SaveSession(sessionID string, messages []Message) error {
	sessionFile := filepath.Join(".kele/sessions", sessionID+".jsonl")

	file, err := os.OpenFile(sessionFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, msg := range messages {
		line, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		file.Write(line)
		file.Write([]byte("\n"))
	}

	return nil
}

// LoadSession 加载会话
func (s *Store) LoadSession(sessionID string) ([]Message, error) {
	sessionFile := filepath.Join(".kele/sessions", sessionID+".jsonl")

	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return nil, err
	}

	var messages []Message
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var msg Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

// Close 关闭存储
func (s *Store) Close() error {
	return s.db.Close()
}

// Message 消息结构
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}
