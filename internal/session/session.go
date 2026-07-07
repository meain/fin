// Package session persists conversations to JSONL files and exposes
// loaders, summaries, and a relevance-scored search for past sessions.
//
// On-disk format: line 1 is sessionHeader (session-level metadata),
// subsequent lines are individual Message records. The header/twin split
// is load-bearing — appendNew opens the file with O_APPEND and avoids
// rewriting the whole session on every turn.
package session

import (
	"strings"
	"time"

	t "github.com/meain/fin/internal/types"
)

// Session is the in-memory representation of a saved conversation.
type Session struct {
	ID              string      `json:"id"`
	Title           string      `json:"title"`
	Model           string      `json:"model"`
	Cwd             string      `json:"cwd"`
	Name            string      `json:"name,omitempty"`
	PreviousSession string      `json:"previous_session,omitempty"`
	Temp            bool        `json:"temp,omitempty"`
	Tags            []string    `json:"tags,omitempty"`
	StartedAt       time.Time   `json:"started_at"`
	Messages        []t.Message `json:"messages"`
}

// sessionHeader mirrors the metadata-only top-of-file JSONL line. Kept as a
// twin of Session (no Messages field) so append-only saves never have to
// rewrite the entire file when the title or other header data changes.
type sessionHeader struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Model           string    `json:"model"`
	Cwd             string    `json:"cwd"`
	Name            string    `json:"name,omitempty"`
	PreviousSession string    `json:"previous_session,omitempty"`
	Temp            bool      `json:"temp,omitempty"`
	Tags            []string  `json:"tags,omitempty"`
	StartedAt       time.Time `json:"started_at"`
}

// TitleFromFirstMessage derives a short title from the first user message:
// trims whitespace, collapses newlines, truncates to 50 chars with ellipsis.
func TitleFromFirstMessage(messages []t.Message) string {
	for _, m := range messages {
		if m.Role != t.RoleUser {
			continue
		}
		s := strings.TrimSpace(m.Content)
		s = strings.Join(strings.Fields(s), " ")
		if len(s) > 50 {
			s = s[:50] + "…"
		}
		return s
	}
	return ""
}

// LastMessageTime returns the most recent Message timestamp, or the session
// start time when no timestamps are recorded.
func LastMessageTime(s Session) time.Time {
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if !s.Messages[i].Timestamp.IsZero() {
			return s.Messages[i].Timestamp
		}
	}
	return s.StartedAt
}
