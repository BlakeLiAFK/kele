# 记忆系统实现

使用 Go 实现混合检索的记忆系统。

## 依赖安装

```bash
go get github.com/mattn/go-sqlite3@latest
go get github.com/tiktoken-go/tokenizer@latest
```

## 核心结构

```go
// internal/memory/store.go
package memory

import (
    "database/sql"
    _ "github.com/mattn/go-sqlite3"
)

type Store struct {
    db     *sql.DB
    config *types.MemoryConfig
}

func NewStore(cfg *types.MemoryConfig) (*Store, error) {
    db, err := sql.Open("sqlite3", cfg.DBPath)
    if err != nil {
        return nil, err
    }

    store := &Store{
        db:     db,
        config: cfg,
    }

    if err := store.initSchema(); err != nil {
        return nil, err
    }

    return store, nil
}

func (s *Store) initSchema() error {
    schema := `
    -- 全文检索表
    CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
        content,
        source,
        timestamp UNINDEXED
    );

    -- 关联表
    CREATE TABLE IF NOT EXISTS memory_refs (
        rowid INTEGER PRIMARY KEY AUTOINCREMENT,
        source_file TEXT NOT NULL,
        chunk_index INTEGER,
        content TEXT NOT NULL,
        timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
    );
    `

    _, err := s.db.Exec(schema)
    return err
}
```

## 索引文档

```go
// internal/memory/indexer.go
package memory

func (s *Store) IndexDocument(filePath string) error {
    // 1. 读取文件
    content, err := os.ReadFile(filePath)
    if err != nil {
        return err
    }

    // 2. 分块
    chunks := s.splitIntoChunks(string(content), 500)

    // 3. 索引每个块
    for i, chunk := range chunks {
        // 插入关联表
        result, err := s.db.Exec(
            "INSERT INTO memory_refs (source_file, chunk_index, content) VALUES (?, ?, ?)",
            filePath, i, chunk,
        )
        if err != nil {
            return err
        }

        rowid, _ := result.LastInsertId()

        // 插入 FTS5
        _, err = s.db.Exec(
            "INSERT INTO memory_fts (rowid, content, source) VALUES (?, ?, ?)",
            rowid, chunk, filePath,
        )
        if err != nil {
            return err
        }
    }

    logger.Info("Document indexed", "file", filePath, "chunks", len(chunks))
    return nil
}

func (s *Store) splitIntoChunks(text string, maxTokens int) []string {
    // 简单实现：按句子分割
    sentences := strings.Split(text, "。")
    chunks := []string{}
    current := ""

    for _, sentence := range sentences {
        if len(current)+len(sentence) > maxTokens*4 {
            if current != "" {
                chunks = append(chunks, current)
            }
            current = sentence
        } else {
            current += sentence + "。"
        }
    }

    if current != "" {
        chunks = append(chunks, current)
    }

    return chunks
}
```

## 全文检索

```go
// internal/memory/fts.go
package memory

type SearchResult struct {
    Content string
    Source  string
    Score   float64
}

func (s *Store) FTSSearch(query string, limit int) ([]SearchResult, error) {
    rows, err := s.db.Query(`
        SELECT r.content, r.source_file, f.rank
        FROM memory_fts f
        JOIN memory_refs r ON r.rowid = f.rowid
        WHERE f.content MATCH ?
        ORDER BY f.rank
        LIMIT ?
    `, query, limit)

    if err != nil {
        return nil, err
    }
    defer rows.Close()

    results := []SearchResult{}

    for rows.Next() {
        var result SearchResult
        if err := rows.Scan(&result.Content, &result.Source, &result.Score); err != nil {
            return nil, err
        }
        results = append(results, result)
    }

    return results, nil
}
```

## 混合搜索

```go
// internal/memory/hybrid.go
package memory

func (s *Store) HybridSearch(query string, limit int) ([]SearchResult, error) {
    // 1. 全文检索
    ftsResults, err := s.FTSSearch(query, 20)
    if err != nil {
        return nil, err
    }

    // 2. 向量检索（如果实现了 sqlite-vec）
    // vecResults, err := s.VectorSearch(query, 20)

    // 3. RRF 融合
    merged := s.reciprocalRankFusion(ftsResults, nil)

    // 4. 返回 top K
    if len(merged) > limit {
        merged = merged[:limit]
    }

    return merged, nil
}

func (s *Store) reciprocalRankFusion(
    results1 []SearchResult,
    results2 []SearchResult,
) []SearchResult {
    const k = 60
    scores := make(map[string]float64)
    contentMap := make(map[string]SearchResult)

    // 计算 RRF 分数
    for i, r := range results1 {
        score := 1.0 / float64(k+i+1)
        key := r.Content[:min(50, len(r.Content))] // 使用内容前缀作为 key
        scores[key] += score
        contentMap[key] = r
    }

    // 排序
    var merged []SearchResult
    for key, score := range scores {
        result := contentMap[key]
        result.Score = score
        merged = append(merged, result)
    }

    sort.Slice(merged, func(i, j int) bool {
        return merged[i].Score > merged[j].Score
    })

    return merged
}
```

---

**相关文档**:
- [记忆系统架构](../03-core-features/memory-system.md)
