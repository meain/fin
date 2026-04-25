package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const sessionDir = "~/.local/share/fin/sessions"

type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`
	Cwd       string    `json:"cwd"`
	StartedAt time.Time `json:"started_at"`
	Messages  []Message `json:"messages"`
}

func sessionPath() string {
	return expandHome(sessionDir)
}

// SaveSession writes the conversation to disk.
func SaveSession(model string, messages []Message) error {
	dir := sessionPath()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	id := uuid.New().String()
	cwd, _ := os.Getwd()
	sess := Session{
		ID:        id,
		Title:     sessionTitle(messages),
		Model:     model,
		Cwd:       cwd,
		StartedAt: time.Now(),
		Messages:  messages,
	}

	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}

	filename := fmt.Sprintf("%s_%s.json", time.Now().Format("20060102-150405"), id)

	return os.WriteFile(filepath.Join(dir, filename), data, 0644)
}

// sessionTitle generates a title from the first user message.
func sessionTitle(messages []Message) string {
	for _, m := range messages {
		if m.Role != RoleUser {
			continue
		}
		t := strings.TrimSpace(m.Content)
		// Collapse whitespace/newlines
		t = strings.Join(strings.Fields(t), " ")
		if len(t) > 50 {
			t = t[:50] + "…"
		}
		return t
	}
	return ""
}

// loadAllSessions reads and parses all session files, sorted newest first.
func loadAllSessions() ([]Session, error) {
	dir := sessionPath()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var sessions []Session
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}
		sessions = append(sessions, sess)
	}

	// Sort by last message timestamp descending
	sort.Slice(sessions, func(i, j int) bool {
		return lastMessageTime(sessions[i]).After(lastMessageTime(sessions[j]))
	})

	return sessions, nil
}

// LoadSessionByID loads a session by its UUID from the JSON metadata.
func LoadSessionByID(id string) (*Session, error) {
	sessions, err := loadAllSessions()
	if err != nil {
		return nil, fmt.Errorf("session %s not found", id)
	}
	for i := range sessions {
		if sessions[i].ID == id || strings.HasPrefix(sessions[i].ID, id) {
			return &sessions[i], nil
		}
	}
	return nil, fmt.Errorf("session %s not found", id)
}

// LoadLastSession loads the most recent session.
func LoadLastSession() (*Session, error) {
	sessions, err := loadAllSessions()
	if err != nil || len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}
	return &sessions[0], nil
}

// ListSessions prints saved sessions to stderr. limit=-1 for all.
func ListSessions(limit int) {
	sessions, err := loadAllSessions()
	if err != nil || len(sessions) == 0 {
		fmt.Fprintf(os.Stderr, "no sessions found\n")
		return
	}

	total := len(sessions)
	if limit > 0 && total > limit {
		sessions = sessions[:limit]
	}

	for _, sess := range sessions {
		title := sess.Title
		if title == "" {
			// Fallback for old sessions without a title
			for _, m := range sess.Messages {
				if m.Role == RoleUser {
					title = m.Content
					break
				}
			}
			if len(title) > 50 {
				title = title[:50] + "…"
			}
		}

		msgCount := 0
		for _, m := range sess.Messages {
			if m.Role != RoleSystem {
				msgCount++
			}
		}

		age := relativeTime(lastMessageTime(sess))
		short := sess.ID[:8]

		fmt.Fprintf(os.Stderr, "%s %s \033[2m(%s, %d msgs)\033[0m\n", short, title, age, msgCount)
	}

	if limit > 0 && total > limit {
		fmt.Fprintf(os.Stderr, "\n\033[2mshowing %d of %d sessions, use -all to see all\033[0m\n", limit, total)
	}
}

func lastMessageTime(s Session) time.Time {
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if !s.Messages[i].Timestamp.IsZero() {
			return s.Messages[i].Timestamp
		}
	}
	return s.StartedAt
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
