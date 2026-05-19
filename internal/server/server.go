package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"we-flow/internal/store"
	"we-flow/internal/wx"
)

type Server struct {
	db     *store.DB
	wx     *wx.Client
	webDir string
}

func New(db *store.DB, wxClient *wx.Client, webDir string) *Server {
	return &Server{db: db, wx: wxClient, webDir: webDir}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("GET /api/sessions", s.sessions)
	mux.HandleFunc("GET /api/whitelist", s.getWhitelist)
	mux.HandleFunc("POST /api/whitelist", s.postWhitelist)
	mux.HandleFunc("DELETE /api/whitelist/", s.deleteWhitelist)
	mux.HandleFunc("POST /api/sync", s.sync)
	mux.HandleFunc("POST /api/poll", s.poll)
	mux.HandleFunc("GET /api/messages", s.messages)
	mux.Handle("/", http.FileServer(http.Dir(s.webDir)))
	return logRequests(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.db.Settings()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var settings store.Settings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, err)
		return
	}
	if err := s.db.SaveSettings(settings); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) sessions(w http.ResponseWriter, r *http.Request) {
	limit := intQuery(r, "limit", 100)
	sessions, err := s.wx.Sessions(r.Context(), limit)
	if err != nil {
		writeError(w, err)
		return
	}
	sessions = s.wx.EnrichSessions(r.Context(), sessions)
	sessions = filterUserChatSessions(sessions)
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (s *Server) getWhitelist(w http.ResponseWriter, r *http.Request) {
	items, err := s.db.Whitelist()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) postWhitelist(w http.ResponseWriter, r *http.Request) {
	var item store.WhitelistItem
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		writeError(w, err)
		return
	}
	if err := s.db.UpsertWhitelist(item); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) deleteWhitelist(w http.ResponseWriter, r *http.Request) {
	chatID := strings.TrimPrefix(r.URL.Path, "/api/whitelist/")
	if chatID == "" {
		writeError(w, errors.New("chat id is required"))
		return
	}
	if decoded, err := url.PathUnescape(chatID); err == nil {
		chatID = decoded
	}
	if err := s.db.DeleteWhitelist(chatID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) sync(w http.ResponseWriter, r *http.Request) {
	settings, err := s.db.Settings()
	if err != nil {
		writeError(w, err)
		return
	}
	days := intQuery(r, "days", settings.Days)
	if days <= 0 {
		days = settings.Days
	}
	items, err := s.db.EnabledWhitelist()
	if err != nil {
		writeError(w, err)
		return
	}
	since := time.Now().AddDate(0, 0, -days)
	result := map[string]any{
		"days":       days,
		"chats":      len(items),
		"inserted":   0,
		"failures":   []map[string]string{},
		"started_at": time.Now().Format(time.RFC3339),
	}

	total := 0
	failures := []map[string]string{}
	for _, item := range items {
		source := "history"
		history, err := s.wx.History(r.Context(), item.ChatID, since, 1000)
		if err != nil {
			cacheHistory, cacheErr := s.wx.CacheHistory(r.Context(), item.ChatID, since, 1000)
			if cacheErr != nil {
				failures = append(failures, map[string]string{
					"chat_id": item.ChatID,
					"name":    whitelistDisplayName(item),
					"error":   err.Error(),
					"hint":    syncHint(item.ChatID, err) + " 缓存库直读也失败：" + cacheErr.Error(),
				})
				continue
			}
			history = cacheHistory
			source = "cache"
		}
		chatID := history.Username
		if chatID == "" {
			chatID = item.ChatID
		}
		inputs := messageInputs(item, chatID, history.ChatType, history.IsGroup, history.Messages, source)
		inserted, err := s.db.InsertMessages(inputs)
		if err != nil {
			failures = append(failures, map[string]string{"chat_id": item.ChatID, "name": whitelistDisplayName(item), "error": err.Error()})
			continue
		}
		total += inserted
	}
	result["inserted"] = total
	result["failures"] = failures
	result["finished_at"] = time.Now().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) poll(w http.ResponseWriter, r *http.Request) {
	items, err := s.db.EnabledWhitelist()
	if err != nil {
		writeError(w, err)
		return
	}
	byID := make(map[string]store.WhitelistItem, len(items))
	for _, item := range items {
		byID[item.ChatID] = item
	}

	limit := intQuery(r, "limit", 300)
	messages, err := s.wx.NewMessages(r.Context(), limit)
	if err != nil {
		writeError(w, err)
		return
	}

	inputs := []store.MessageInput{}
	seenChats := map[string]int{}
	for _, msg := range messages.Messages {
		chatID := msg.Username
		if chatID == "" {
			chatID = msg.Chat
		}
		item, ok := byID[chatID]
		if !ok {
			continue
		}
		seenChats[chatID]++
		inputs = append(inputs, messageInputs(item, chatID, msg.ChatType, msg.IsGroup, []wx.Message{msg}, "new-messages")...)
	}
	inserted, err := s.db.InsertMessages(inputs)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"checked":        messages.Count,
		"matched":        len(inputs),
		"inserted":       inserted,
		"whitelist":      len(items),
		"matched_chats":  seenChats,
		"finished_at":    time.Now().Format(time.RFC3339),
		"source_command": "wx new-messages",
	})
}

func (s *Server) messages(w http.ResponseWriter, r *http.Request) {
	settings, _ := s.db.Settings()
	days := intQuery(r, "days", settings.Days)
	limit := intQuery(r, "limit", 200)
	chatID := r.URL.Query().Get("chat_id")
	query := r.URL.Query().Get("q")

	messages, err := s.db.Messages(chatID, query, days, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
}

func messageInputs(item store.WhitelistItem, chatID, chatType string, isGroup bool, messages []wx.Message, source string) []store.MessageInput {
	chatName := whitelistDisplayName(item)
	if chatType == "" {
		chatType = item.ChatType
	}
	if !isGroup {
		isGroup = item.IsGroup
	}
	inputs := make([]store.MessageInput, 0, len(messages))
	for _, msg := range messages {
		inputs = append(inputs, store.MessageInput{
			DedupeKey:  wx.DedupeKey(chatID, msg),
			ChatID:     chatID,
			ChatName:   chatName,
			ChatType:   chatType,
			IsGroup:    isGroup,
			SenderID:   msg.Sender,
			SenderName: msg.Sender,
			Content:    msg.Content,
			Type:       msg.Type,
			Timestamp:  msg.Timestamp,
			TimeText:   msg.Time,
			LocalID:    msg.LocalID,
			URL:        msg.URL,
			Source:     source,
		})
	}
	return inputs
}

func filterUserChatSessions(sessions []wx.Session) []wx.Session {
	out := make([]wx.Session, 0, len(sessions))
	for _, session := range sessions {
		id := session.Username
		if id == "" {
			id = session.Chat
		}
		switch session.ChatType {
		case "group", "private":
			if id == "brandsessionholder" || id == "brandservicesessionholder" || id == "@placeholder_foldgroup" {
				continue
			}
			if strings.HasPrefix(id, "gh_") {
				continue
			}
			out = append(out, session)
		}
	}
	return out
}

func whitelistDisplayName(item store.WhitelistItem) string {
	if item.ChatName == "" || item.ChatName == item.ChatID {
		return friendlyChatName(item.ChatID, item.ChatType, item.IsGroup)
	}
	return item.ChatName
}

func friendlyChatName(chatID, chatType string, isGroup bool) string {
	switch {
	case chatID == "filehelper":
		return "文件传输助手"
	case isGroup || chatType == "group" || strings.HasSuffix(chatID, "@chatroom"):
		return "群聊 " + shortID(chatID)
	case strings.HasPrefix(chatID, "wxid_"):
		return "好友 " + shortID(chatID)
	default:
		return chatID
	}
}

func shortID(chatID string) string {
	if len(chatID) <= 12 {
		return chatID
	}
	return chatID[:6] + "..." + chatID[len(chatID)-4:]
}

func syncHint(chatID string, err error) string {
	msg := err.Error()
	if chatID == "filehelper" || strings.Contains(msg, "找不到联系人") {
		return "wx history 无法回溯这个会话；可以点击“拉取新消息”，从 wx new-messages 的增量结果开始积累。"
	}
	return ""
}

func intQuery(r *http.Request, key string, fallback int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		fmt.Printf("%s %s %s\n", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func withTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = 30 * time.Second
	}
	return context.WithTimeout(parent, d)
}
