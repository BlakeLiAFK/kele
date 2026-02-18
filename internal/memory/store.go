package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BlakeLiAFK/kele/internal/config"
	_ "github.com/mattn/go-sqlite3"
)

// Store 记忆存储
type Store struct {
	db         *sql.DB
	memoryFile string
	sessionDir string
	hasFTS5    bool // FTS5 是否可用
}

// NewStore 创建存储（返回 error 而非 panic）
func NewStore(cfg *config.Config) (*Store, error) {
	dbPath := cfg.Memory.DBPath
	memoryFile := cfg.Memory.MemoryFile
	sessionDir := cfg.Memory.SessionDir

	os.MkdirAll(filepath.Dir(dbPath), 0755)
	os.MkdirAll(sessionDir, 0755)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	store := &Store{
		db:         db,
		memoryFile: memoryFile,
		sessionDir: sessionDir,
	}
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化数据库失败: %w", err)
	}
	return store, nil
}

func (s *Store) initSchema() error {
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
	CREATE INDEX IF NOT EXISTS idx_messages_ts ON messages(timestamp);

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		message_count INTEGER DEFAULT 0,
		summary TEXT DEFAULT ''
	);
	`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}

	// 尝试创建 FTS5 虚拟表（如果 FTS5 不可用则跳过）
	s.initFTS5()
	return nil
}

// initFTS5 尝试初始化 FTS5 全文搜索
func (s *Store) initFTS5() {
	_, err := s.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
			key, value,
			tokenize='unicode61'
		);
	`)
	if err != nil {
		// FTS5 不可用，使用 LIKE 降级方案
		s.hasFTS5 = false
		return
	}
	s.hasFTS5 = true

	// 同步已有数据到 FTS5 表（增量：只添加 FTS5 中不存在的）
	s.db.Exec(`
		INSERT OR IGNORE INTO memory_fts(rowid, key, value)
		SELECT id, key, value FROM memory_entries
		WHERE id NOT IN (SELECT rowid FROM memory_fts)
	`)
}

func (s *Store) SaveMessage(role, content string) error {
	_, err := s.db.Exec("INSERT INTO messages (role, content) VALUES (?, ?)", role, content)
	return err
}

func (s *Store) GetRecentMessages(limit int) ([]Message, error) {
	rows, err := s.db.Query("SELECT role, content, timestamp FROM messages ORDER BY id DESC LIMIT ?", limit)
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
	for i := 0; i < len(messages)/2; i++ {
		j := len(messages) - 1 - i
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

func (s *Store) UpdateMemory(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO memory_entries (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value=?, updated_at=CURRENT_TIMESTAMP`,
		key, value, value,
	)
	if err != nil {
		return err
	}

	// 同步到 FTS5
	if s.hasFTS5 {
		// 获取新插入/更新行的 ID
		var rowID int64
		s.db.QueryRow("SELECT id FROM memory_entries WHERE key = ?", key).Scan(&rowID)
		// 删除旧的 FTS5 条目，再插入新的
		s.db.Exec("DELETE FROM memory_fts WHERE rowid = ?", rowID)
		s.db.Exec("INSERT INTO memory_fts(rowid, key, value) VALUES (?, ?, ?)", rowID, key, value)
	}

	return s.syncToFile()
}

func (s *Store) GetMemory(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM memory_entries WHERE key = ?", key).Scan(&value)
	return value, err
}

// GetRecentMemories 获取最近的记忆条目（用于 system prompt 注入）
func (s *Store) GetRecentMemories(limit int) ([]string, error) {
	rows, err := s.db.Query(
		"SELECT key, value FROM memory_entries ORDER BY updated_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		entry := value
		if len([]rune(entry)) > 200 {
			entry = string([]rune(entry)[:200]) + "..."
		}
		results = append(results, fmt.Sprintf("[%s] %s", key, entry))
	}
	return results, nil
}

// Search 搜索记忆（优先使用 FTS5 BM25 排序，降级使用 LIKE）
func (s *Store) Search(query string, limit int) ([]string, error) {
	keywords := strings.Fields(query)
	if len(keywords) == 0 {
		return nil, nil
	}

	// 优先使用 FTS5 搜索
	if s.hasFTS5 {
		return s.searchFTS5(query, keywords, limit)
	}
	return s.searchLike(keywords, limit)
}

// searchFTS5 使用 FTS5 全文搜索（BM25 排序）
func (s *Store) searchFTS5(query string, keywords []string, limit int) ([]string, error) {
	// 构建 FTS5 查询表达式：多词用 AND 连接
	ftsQuery := strings.Join(keywords, " AND ")

	rows, err := s.db.Query(
		`SELECT value, bm25(memory_fts) AS rank
		 FROM memory_fts
		 WHERE memory_fts MATCH ?
		 ORDER BY rank
		 LIMIT ?`,
		ftsQuery, limit)
	if err != nil {
		// FTS5 查询失败，降级到 LIKE
		return s.searchLike(keywords, limit)
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var content string
		var rank float64
		if err := rows.Scan(&content, &rank); err != nil {
			continue
		}
		results = append(results, content)
	}

	// FTS5 无结果时尝试 OR 查询
	if len(results) == 0 && len(keywords) > 1 {
		orQuery := strings.Join(keywords, " OR ")
		rows2, err := s.db.Query(
			`SELECT value, bm25(memory_fts) AS rank
			 FROM memory_fts
			 WHERE memory_fts MATCH ?
			 ORDER BY rank
			 LIMIT ?`,
			orQuery, limit)
		if err != nil {
			return s.searchLike(keywords, limit)
		}
		defer rows2.Close()
		for rows2.Next() {
			var content string
			var rank float64
			if err := rows2.Scan(&content, &rank); err != nil {
				continue
			}
			results = append(results, content)
		}
	}

	return results, nil
}

// searchLike LIKE 降级搜索
func (s *Store) searchLike(keywords []string, limit int) ([]string, error) {
	var conditions []string
	var args []interface{}
	for _, kw := range keywords {
		conditions = append(conditions, "value LIKE ?")
		args = append(args, "%"+kw+"%")
	}
	args = append(args, limit)

	sqlStr := fmt.Sprintf(
		"SELECT value FROM memory_entries WHERE %s ORDER BY updated_at DESC LIMIT ?",
		strings.Join(conditions, " AND "),
	)
	rows, err := s.db.Query(sqlStr, args...)
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

	// 多关键词无结果时退化为 OR 搜索
	if len(results) == 0 && len(keywords) > 1 {
		var orConditions []string
		var orArgs []interface{}
		for _, kw := range keywords {
			orConditions = append(orConditions, "value LIKE ?")
			orArgs = append(orArgs, "%"+kw+"%")
		}
		orArgs = append(orArgs, limit)
		orSQL := fmt.Sprintf(
			"SELECT value FROM memory_entries WHERE %s ORDER BY updated_at DESC LIMIT ?",
			strings.Join(orConditions, " OR "),
		)
		rows2, err := s.db.Query(orSQL, orArgs...)
		if err != nil {
			return nil, err
		}
		defer rows2.Close()
		for rows2.Next() {
			var content string
			if err := rows2.Scan(&content); err != nil {
				continue
			}
			results = append(results, content)
		}
	}

	return results, nil
}

// HasFTS5 返回 FTS5 是否可用
func (s *Store) HasFTS5() bool {
	return s.hasFTS5
}

func (s *Store) syncToFile() error {
	rows, err := s.db.Query("SELECT key, value, updated_at FROM memory_entries ORDER BY updated_at DESC")
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

	os.MkdirAll(filepath.Dir(s.memoryFile), 0755)
	return os.WriteFile(s.memoryFile, []byte(content), 0644)
}

// --- 会话持久化 ---

func (s *Store) SaveSession(sessionID string, messages []Message) error {
	sessionFile := filepath.Join(s.sessionDir, sessionID+".jsonl")
	file, err := os.Create(sessionFile)
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

	summary := ""
	if len(messages) > 0 {
		summary = messages[0].Content
		if len([]rune(summary)) > 50 {
			summary = string([]rune(summary)[:50]) + "..."
		}
	}
	s.db.Exec(`INSERT INTO sessions (id, name, updated_at, message_count, summary)
		VALUES (?, ?, CURRENT_TIMESTAMP, ?, ?)
		ON CONFLICT(id) DO UPDATE SET updated_at=CURRENT_TIMESTAMP, message_count=?, summary=?`,
		sessionID, sessionID, len(messages), summary, len(messages), summary)
	return nil
}

func (s *Store) LoadSession(sessionID string) ([]Message, error) {
	sessionFile := filepath.Join(s.sessionDir, sessionID+".jsonl")
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return nil, err
	}
	var messages []Message
	for _, line := range strings.Split(string(data), "\n") {
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

func (s *Store) ListSessions() ([]SessionInfo, error) {
	rows, err := s.db.Query("SELECT id, name, created_at, updated_at, message_count, summary FROM sessions ORDER BY updated_at DESC LIMIT 20")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []SessionInfo
	for rows.Next() {
		var si SessionInfo
		var createdAt, updatedAt string
		if err := rows.Scan(&si.ID, &si.Name, &createdAt, &updatedAt, &si.MessageCount, &si.Summary); err != nil {
			continue
		}
		si.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		si.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		sessions = append(sessions, si)
	}
	return sessions, nil
}

// GetLatestSession 获取最近的会话（用于启动时自动恢复）
func (s *Store) GetLatestSession() (*SessionInfo, error) {
	sessions, err := s.ListSessions()
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil
	}
	return &sessions[0], nil
}

func (s *Store) Close() error { return s.db.Close() }

// Message 消息结构
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// SessionInfo 会话信息
type SessionInfo struct {
	ID           string
	Name         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	MessageCount int
	Summary      string
}
