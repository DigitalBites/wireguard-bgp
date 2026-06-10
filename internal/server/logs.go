package server

import (
	"net/http"
	"sync"
	"time"
)

type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

type LogStore struct {
	mu      sync.Mutex
	limit   int
	entries []LogEntry
}

func NewLogStore(limit int) *LogStore {
	if limit <= 0 {
		limit = 100
	}
	store := &LogStore{limit: limit}
	store.Add("info", "application initialized", "")
	return store
}

func (l *LogStore) Add(level, message, detail string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, LogEntry{
		Time:    time.Now().UTC().Format(time.RFC3339),
		Level:   level,
		Message: message,
		Detail:  detail,
	})
	if len(l.entries) > l.limit {
		l.entries = append([]LogEntry(nil), l.entries[len(l.entries)-l.limit:]...)
	}
}

func (l *LogStore) Entries() []LogEntry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]LogEntry(nil), l.entries...)
}

func (s *Server) getLogs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"entries": s.logs.Entries()})
}
