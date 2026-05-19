package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

type WhitelistItem struct {
	ChatID          string `json:"chat_id"`
	ChatName        string `json:"chat_name"`
	ChatType        string `json:"chat_type"`
	IsGroup         bool   `json:"is_group"`
	Enabled         bool   `json:"enabled"`
	AnalysisEnabled bool   `json:"analysis_enabled"`
	RealtimeEnabled bool   `json:"realtime_enabled"`
	Notes           string `json:"notes"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type Message struct {
	ID         int64  `json:"id"`
	DedupeKey  string `json:"dedupe_key"`
	ChatID     string `json:"chat_id"`
	ChatName   string `json:"chat_name"`
	ChatType   string `json:"chat_type"`
	IsGroup    bool   `json:"is_group"`
	SenderID   string `json:"sender_id"`
	SenderName string `json:"sender_name"`
	Content    string `json:"content"`
	Type       string `json:"type"`
	Timestamp  int64  `json:"timestamp"`
	TimeText   string `json:"time_text"`
	LocalID    *int64 `json:"local_id,omitempty"`
	URL        string `json:"url,omitempty"`
	Source     string `json:"source"`
	CreatedAt  string `json:"created_at"`
}

type MessageInput struct {
	DedupeKey  string
	ChatID     string
	ChatName   string
	ChatType   string
	IsGroup    bool
	SenderID   string
	SenderName string
	Content    string
	Type       string
	Timestamp  int64
	TimeText   string
	LocalID    *int64
	URL        string
	Source     string
}

type Settings struct {
	Days int `json:"days"`
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	db := &DB{conn: conn}
	if err := db.init(); err != nil {
		conn.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) init() error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
	}
	for _, p := range pragmas {
		if _, err := db.conn.Exec(p); err != nil {
			return err
		}
	}
	schema := `
CREATE TABLE IF NOT EXISTS whitelist (
	chat_id TEXT PRIMARY KEY,
	chat_name TEXT NOT NULL,
	chat_type TEXT NOT NULL,
	is_group INTEGER NOT NULL DEFAULT 0,
	enabled INTEGER NOT NULL DEFAULT 1,
	analysis_enabled INTEGER NOT NULL DEFAULT 1,
	realtime_enabled INTEGER NOT NULL DEFAULT 0,
	notes TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	dedupe_key TEXT NOT NULL UNIQUE,
	chat_id TEXT NOT NULL,
	chat_name TEXT NOT NULL DEFAULT '',
	chat_type TEXT NOT NULL DEFAULT '',
	is_group INTEGER NOT NULL DEFAULT 0,
	sender_id TEXT NOT NULL DEFAULT '',
	sender_name TEXT NOT NULL DEFAULT '',
	content TEXT NOT NULL DEFAULT '',
	type TEXT NOT NULL DEFAULT '',
	timestamp INTEGER NOT NULL DEFAULT 0,
	time_text TEXT NOT NULL DEFAULT '',
	local_id INTEGER,
	url TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_messages_chat_timestamp ON messages(chat_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp);

CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

INSERT OR IGNORE INTO settings(key, value) VALUES ('days', '7');
`
	_, err := db.conn.Exec(schema)
	return err
}

func (db *DB) Settings() (Settings, error) {
	var raw string
	if err := db.conn.QueryRow(`SELECT value FROM settings WHERE key = 'days'`).Scan(&raw); err != nil {
		return Settings{}, err
	}
	var days int
	if _, err := fmt.Sscanf(raw, "%d", &days); err != nil || days <= 0 {
		days = 7
	}
	return Settings{Days: days}, nil
}

func (db *DB) SaveSettings(s Settings) error {
	if s.Days <= 0 || s.Days > 3650 {
		return errors.New("days must be between 1 and 3650")
	}
	_, err := db.conn.Exec(`INSERT INTO settings(key, value) VALUES('days', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, fmt.Sprint(s.Days))
	return err
}

func (db *DB) Whitelist() ([]WhitelistItem, error) {
	rows, err := db.conn.Query(`SELECT chat_id, chat_name, chat_type, is_group, enabled, analysis_enabled, realtime_enabled, notes, created_at, updated_at FROM whitelist ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []WhitelistItem{}
	for rows.Next() {
		var it WhitelistItem
		var isGroup, enabled, analysis, realtime int
		if err := rows.Scan(&it.ChatID, &it.ChatName, &it.ChatType, &isGroup, &enabled, &analysis, &realtime, &it.Notes, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		it.IsGroup = isGroup != 0
		it.Enabled = enabled != 0
		it.AnalysisEnabled = analysis != 0
		it.RealtimeEnabled = realtime != 0
		items = append(items, it)
	}
	return items, rows.Err()
}

func (db *DB) EnabledWhitelist() ([]WhitelistItem, error) {
	rows, err := db.conn.Query(`SELECT chat_id, chat_name, chat_type, is_group, enabled, analysis_enabled, realtime_enabled, notes, created_at, updated_at FROM whitelist WHERE enabled = 1 ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []WhitelistItem{}
	for rows.Next() {
		var it WhitelistItem
		var isGroup, enabled, analysis, realtime int
		if err := rows.Scan(&it.ChatID, &it.ChatName, &it.ChatType, &isGroup, &enabled, &analysis, &realtime, &it.Notes, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		it.IsGroup = isGroup != 0
		it.Enabled = enabled != 0
		it.AnalysisEnabled = analysis != 0
		it.RealtimeEnabled = realtime != 0
		items = append(items, it)
	}
	return items, rows.Err()
}

func (db *DB) UpsertWhitelist(it WhitelistItem) error {
	now := time.Now().Format(time.RFC3339)
	if strings.TrimSpace(it.ChatID) == "" {
		return errors.New("chat_id is required")
	}
	if it.ChatName == "" {
		it.ChatName = it.ChatID
	}
	_, err := db.conn.Exec(`
INSERT INTO whitelist(chat_id, chat_name, chat_type, is_group, enabled, analysis_enabled, realtime_enabled, notes, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(chat_id) DO UPDATE SET
	chat_name = excluded.chat_name,
	chat_type = excluded.chat_type,
	is_group = excluded.is_group,
	enabled = excluded.enabled,
	analysis_enabled = excluded.analysis_enabled,
	realtime_enabled = excluded.realtime_enabled,
	notes = excluded.notes,
	updated_at = excluded.updated_at`,
		it.ChatID, it.ChatName, it.ChatType, boolInt(it.IsGroup), boolInt(it.Enabled), boolInt(it.AnalysisEnabled), boolInt(it.RealtimeEnabled), it.Notes, now, now)
	return err
}

func (db *DB) DeleteWhitelist(chatID string) error {
	_, err := db.conn.Exec(`DELETE FROM whitelist WHERE chat_id = ?`, chatID)
	return err
}

func (db *DB) InsertMessages(messages []MessageInput) (int, error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
INSERT OR IGNORE INTO messages(dedupe_key, chat_id, chat_name, chat_type, is_group, sender_id, sender_name, content, type, timestamp, time_text, local_id, url, source, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	now := time.Now().Format(time.RFC3339)
	inserted := 0
	for _, m := range messages {
		res, err := stmt.Exec(m.DedupeKey, m.ChatID, m.ChatName, m.ChatType, boolInt(m.IsGroup), m.SenderID, m.SenderName, m.Content, m.Type, m.Timestamp, m.TimeText, m.LocalID, m.URL, m.Source, now)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		inserted += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return inserted, nil
}

func (db *DB) Messages(chatID, query string, days, limit int) ([]Message, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	if days <= 0 {
		days = 7
	}
	minTS := time.Now().AddDate(0, 0, -days).Unix()

	clauses := []string{"timestamp >= ?"}
	args := []any{minTS}
	if chatID != "" {
		clauses = append(clauses, "chat_id = ?")
		args = append(args, chatID)
	}
	if query != "" {
		clauses = append(clauses, "(content LIKE ? OR sender_name LIKE ? OR sender_id LIKE ? OR chat_name LIKE ?)")
		like := "%" + query + "%"
		args = append(args, like, like, like, like)
	}
	args = append(args, limit)

	rows, err := db.conn.Query(`
SELECT id, dedupe_key, chat_id, chat_name, chat_type, is_group, sender_id, sender_name, content, type, timestamp, time_text, local_id, url, source, created_at
FROM messages
WHERE `+strings.Join(clauses, " AND ")+`
ORDER BY timestamp DESC, id DESC
LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Message{}
	for rows.Next() {
		var m Message
		var isGroup int
		var local sql.NullInt64
		if err := rows.Scan(&m.ID, &m.DedupeKey, &m.ChatID, &m.ChatName, &m.ChatType, &isGroup, &m.SenderID, &m.SenderName, &m.Content, &m.Type, &m.Timestamp, &m.TimeText, &local, &m.URL, &m.Source, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.IsGroup = isGroup != 0
		if local.Valid {
			m.LocalID = &local.Int64
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
