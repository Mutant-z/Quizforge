package sqlite

import (
	"database/sql"
	"fmt"
	"time"
)

// Repository 聚合所有 SQLite 数据访问。
type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) DB() *sql.DB { return r.db }

func now() time.Time { return time.Now().UTC() }

// parseTime 解析 SQLite datetime 字符串。
func parseTime(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	// SQLite datetime('now') 输出 "YYYY-MM-DD HH:MM:SS"
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s.String, time.UTC); err == nil {
			return &t
		}
	}
	return nil
}

func fmtTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

var _ = fmt.Sprintf
