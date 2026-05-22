package wx

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

type Client struct {
	bin     string
	timeout time.Duration
}

type Session struct {
	Chat        string `json:"chat"`
	ChatType    string `json:"chat_type"`
	DisplayName string `json:"display_name"`
	IsGroup     bool   `json:"is_group"`
	LastMsgType string `json:"last_msg_type"`
	LastSender  string `json:"last_sender"`
	Summary     string `json:"summary"`
	Time        string `json:"time"`
	Timestamp   int64  `json:"timestamp"`
	Unread      int    `json:"unread"`
	Username    string `json:"username"`
}

type sessionsResponse struct {
	Sessions []Session `json:"sessions"`
}

type HistoryResponse struct {
	Chat     string    `json:"chat"`
	ChatType string    `json:"chat_type"`
	Count    int       `json:"count"`
	IsGroup  bool      `json:"is_group"`
	Messages []Message `json:"messages"`
	Username string    `json:"username"`
}

type NewMessagesResponse struct {
	Count    int       `json:"count"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Chat      string `json:"chat"`
	ChatType  string `json:"chat_type"`
	Content   string `json:"content"`
	IsGroup   bool   `json:"is_group"`
	LocalID   *int64 `json:"local_id"`
	Sender    string `json:"sender"`
	Time      string `json:"time"`
	Timestamp int64  `json:"timestamp"`
	Type      string `json:"type"`
	URL       string `json:"url"`
	Username  string `json:"username"`
}

func New(bin string, timeout time.Duration) *Client {
	return &Client{bin: bin, timeout: timeout}
}

func (c *Client) Sessions(ctx context.Context, limit int) ([]Session, error) {
	if limit <= 0 {
		limit = 100
	}
	var out sessionsResponse
	if err := c.runJSON(ctx, &out, "sessions", "-n", strconv.Itoa(limit), "--json"); err != nil {
		return nil, err
	}
	return out.Sessions, nil
}

func (c *Client) History(ctx context.Context, chat string, since time.Time, limit int) (HistoryResponse, error) {
	if limit <= 0 {
		limit = 1000
	}
	args := []string{"history", chat, "-n", strconv.Itoa(limit), "--json"}
	if !since.IsZero() {
		args = append(args, "--since", since.Format("2006-01-02"))
	}
	var out HistoryResponse
	if err := c.runJSON(ctx, &out, args...); err != nil {
		return HistoryResponse{}, err
	}
	return out, nil
}

func (c *Client) NewMessages(ctx context.Context, limit int) (NewMessagesResponse, error) {
	if limit <= 0 {
		limit = 200
	}
	var out NewMessagesResponse
	if err := c.runJSON(ctx, &out, "new-messages", "-n", strconv.Itoa(limit), "--json"); err != nil {
		return NewMessagesResponse{}, err
	}
	return out, nil
}

func DedupeKey(chatID string, m Message) string {
	if m.LocalID != nil {
		return fmt.Sprintf("%s:%d", chatID, *m.LocalID)
	}
	h := sha1.Sum([]byte(chatID + "\x00" + m.Sender + "\x00" + strconv.FormatInt(m.Timestamp, 10) + "\x00" + m.Type + "\x00" + m.Content))
	return chatID + ":hash:" + hex.EncodeToString(h[:])
}

func (c *Client) runJSON(ctx context.Context, dst any, args ...string) error {
	timeout := c.timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.bin, args...)
	raw, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("找不到 wx-cli 命令 %q。请安装只读工具 @jackwener/wx-cli，或用 WE_FLOW_WX_BIN 指向 wx 可执行文件", c.bin)
		}
		return fmt.Errorf("wx %v: %w: %s", args, err, string(raw))
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("decode wx json: %w: %s", err, string(raw))
	}
	return nil
}
