package wx

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

func (c *Client) CacheHistory(ctx context.Context, chat string, since time.Time, limit int) (HistoryResponse, error) {
	if limit <= 0 {
		limit = 1000
	}
	dbPath, err := newestCacheDB()
	if err != nil {
		return HistoryResponse{}, err
	}
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return HistoryResponse{}, err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return HistoryResponse{}, err
	}

	table := "Msg_" + md5Hex(chat)
	exists, err := tableExists(ctx, db, table)
	if err != nil {
		return HistoryResponse{}, err
	}
	if !exists {
		return HistoryResponse{}, fmt.Errorf("cache table not found: %s", table)
	}

	names, err := loadNameMap(ctx, db)
	if err != nil {
		return HistoryResponse{}, err
	}
	displayNames, _ := contactDisplayNames(ctx)

	minTS := int64(0)
	if !since.IsZero() {
		minTS = since.Unix()
	}
	query := fmt.Sprintf(`SELECT local_id, local_type, real_sender_id, create_time, message_content
FROM %s
WHERE create_time >= ?
ORDER BY create_time DESC
LIMIT ?`, quoteIdent(table))
	rows, err := db.QueryContext(ctx, query, minTS, limit)
	if err != nil {
		return HistoryResponse{}, err
	}
	defer rows.Close()

	messages := []Message{}
	for rows.Next() {
		var localID, localType, senderID, ts int64
		var content sql.NullString
		if err := rows.Scan(&localID, &localType, &senderID, &ts, &content); err != nil {
			return HistoryResponse{}, err
		}
		localIDCopy := localID
		sender := names[senderID]
		if sender == "" {
			sender = fmt.Sprint(senderID)
		}
		senderName := displayNames[sender]
		if senderName == "" {
			senderName = sender
		}
		body := content.String
		if isProbablyCompressed(body) {
			body = "[压缩内容暂未解码]"
		}
		messages = append(messages, Message{
			Chat:      chat,
			ChatType:  inferChatType(chat),
			Content:   body,
			IsGroup:   strings.HasSuffix(chat, "@chatroom"),
			LocalID:   &localIDCopy,
			Sender:    senderName,
			Time:      time.Unix(ts, 0).Format("2006-01-02 15:04"),
			Timestamp: ts,
			Type:      messageType(localType),
			Username:  chat,
		})
	}
	if err := rows.Err(); err != nil {
		return HistoryResponse{}, err
	}

	return HistoryResponse{
		Chat:     chat,
		ChatType: inferChatType(chat),
		Count:    len(messages),
		IsGroup:  strings.HasSuffix(chat, "@chatroom"),
		Messages: messages,
		Username: chat,
	}, nil
}

func (c *Client) EnrichSessions(ctx context.Context, sessions []Session) []Session {
	displayNames, err := contactDisplayNames(ctx)
	if err != nil {
		return sessions
	}
	for i := range sessions {
		id := sessions[i].Username
		if id == "" {
			id = sessions[i].Chat
		}
		if name := displayNames[id]; name != "" {
			sessions[i].DisplayName = name
		}
		if sessions[i].LastSender != "" {
			if name := displayNames[sessions[i].LastSender]; name != "" {
				sessions[i].LastSender = name
			}
		}
	}
	return sessions
}

func newestCacheDB() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".wx-cli", "cache")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	type candidate struct {
		path string
		size int64
		mod  time.Time
	}
	candidates := []candidate{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".db" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{
			path: filepath.Join(dir, entry.Name()),
			size: info.Size(),
			mod:  info.ModTime(),
		})
	}
	if len(candidates) == 0 {
		return "", errors.New("wx-cli cache db not found")
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].size == candidates[j].size {
			return candidates[i].mod.After(candidates[j].mod)
		}
		return candidates[i].size > candidates[j].size
	})
	return candidates[0].path, nil
}

func contactDisplayNames(ctx context.Context) (map[string]string, error) {
	dbPath, err := cacheDBWithTable("contact")
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT username, remark, nick_name, alias FROM contact`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var username, remark, nick, alias string
		if err := rows.Scan(&username, &remark, &nick, &alias); err != nil {
			return nil, err
		}
		name := strings.TrimSpace(remark)
		if name == "" {
			name = strings.TrimSpace(nick)
		}
		if name == "" {
			name = strings.TrimSpace(alias)
		}
		if username != "" && name != "" {
			out[username] = name
		}
	}
	return out, rows.Err()
}

func cacheDBWithTable(table string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".wx-cli", "cache")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".db" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		db, err := sql.Open("sqlite", path+"?mode=ro")
		if err != nil {
			continue
		}
		exists, err := tableExists(context.Background(), db, table)
		db.Close()
		if err == nil && exists {
			return path, nil
		}
	}
	return "", fmt.Errorf("cache db with table %s not found", table)
}

func tableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n)
	return n > 0, err
}

func loadNameMap(ctx context.Context, db *sql.DB) (map[int64]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT rowid, user_name FROM Name2Id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func inferChatType(chat string) string {
	switch {
	case strings.HasSuffix(chat, "@chatroom"):
		return "group"
	case strings.HasPrefix(chat, "gh_"):
		return "official_account"
	default:
		return "private"
	}
}

func messageType(localType int64) string {
	switch localType {
	case 1:
		return "文本"
	case 3:
		return "图片"
	case 34:
		return "语音"
	case 43:
		return "视频"
	case 47:
		return "表情"
	case 49:
		return "链接/文件"
	case 10000:
		return "系统"
	default:
		return fmt.Sprint(localType)
	}
}

func isProbablyCompressed(s string) bool {
	if s == "" {
		return false
	}
	if len(s) >= 4 && s[0] == 0x28 && s[1] == 0xb5 && s[2] == 0x2f && s[3] == 0xfd {
		return true
	}
	if !utf8.ValidString(s) {
		return true
	}
	control := 0
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			control++
		}
	}
	return control > 2
}
