package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

const sessionDir = "~/.local/share/fin/sessions"

type Session struct {
	ID        string    `json:"id"`
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

// loadAllSessions reads and parses all session files, sorted newest first.
func loadAllSessions() ([]Session, error) {
	dir := sessionPath()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	// Sort by name descending (timestamp prefix ensures order)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() > entries[j].Name()
	})

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
	return sessions, nil
}

// LoadSessionByID loads a session by its UUID from the JSON metadata.
func LoadSessionByID(id string) (*Session, error) {
	sessions, err := loadAllSessions()
	if err != nil {
		return nil, fmt.Errorf("session %s not found", id)
	}
	for i := range sessions {
		if sessions[i].ID == id {
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

// ListSessions prints saved sessions to stderr.
func ListSessions() {
	sessions, err := loadAllSessions()
	if err != nil || len(sessions) == 0 {
		fmt.Fprintf(os.Stderr, "no sessions found\n")
		return
	}

	for _, sess := range sessions {
		msgCount := 0
		for _, m := range sess.Messages {
			if m.Role != RoleSystem {
				msgCount++
			}
		}

		preview := ""
		for _, m := range sess.Messages {
			if m.Role == RoleUser {
				preview = m.Content
				break
			}
		}
		if len(preview) > 60 {
			preview = preview[:60] + "..."
		}

		fmt.Fprintf(os.Stderr, "%s  %s  %s  %d msgs  %s\n",
			sess.ID,
			sess.StartedAt.Format("2006-01-02 15:04"),
			sess.Model,
			msgCount,
			preview,
		)
	}
}
