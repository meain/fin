package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	t "github.com/meain/fin/internal/types"
	"golang.org/x/term"
)

const sessionDir = "~/.local/share/fin/sessions"

type Session struct {
	ID              string      `json:"id"`
	Title           string      `json:"title"`
	Model           string      `json:"model"`
	Cwd             string      `json:"cwd"`
	Name            string      `json:"name,omitempty"`
	PreviousSession string      `json:"previous_session,omitempty"`
	StartedAt       time.Time   `json:"started_at"`
	Messages        []t.Message `json:"messages"`
}

func sessionPath() string {
	return expandHome(sessionDir)
}

// SessionWriter handles incremental session saving to a stable file.
type SessionWriter struct {
	id              string
	model           string
	cwd             string
	name            string
	previousSession string
	started         time.Time
	filepath        string
}

// NewSessionWriter creates a new session file and returns a writer for incremental saves.
func NewSessionWriter(model, name string) *SessionWriter {
	dir := sessionPath()
	os.MkdirAll(dir, 0755)

	id := uuid.New().String()
	cwd, _ := os.Getwd()
	filename := fmt.Sprintf("%s_%s.json", time.Now().Format("20060102-150405"), id)
	if name != "" {
		filename = fmt.Sprintf("%s_%s_%s.json", time.Now().Format("20060102-150405"), id, name)
	}

	return &SessionWriter{
		id:       id,
		model:    model,
		cwd:      cwd,
		name:     name,
		started:  time.Now(),
		filepath: filepath.Join(dir, filename),
	}
}

// SessionWriterForExisting creates a writer that overwrites an existing session file.
func SessionWriterForExisting(sess *Session) *SessionWriter {
	dir := sessionPath()
	// Find the existing file
	entries, _ := os.ReadDir(dir)
	var fp string
	for _, e := range entries {
		if !e.IsDir() && strings.Contains(e.Name(), sess.ID) {
			fp = filepath.Join(dir, e.Name())
			break
		}
	}
	if fp == "" {
		// Fallback: create a new file
		fp = filepath.Join(dir, fmt.Sprintf("%s_%s.json", time.Now().Format("20060102-150405"), sess.ID))
	}

	return &SessionWriter{
		id:       sess.ID,
		model:    sess.Model,
		cwd:      sess.Cwd,
		name:     sess.Name,
		started:  sess.StartedAt,
		filepath: fp,
	}
}

// Save writes the current messages to disk.
func (sw *SessionWriter) Save(messages []t.Message) error {
	sess := Session{
		ID:              sw.id,
		Title:           sessionTitle(messages),
		Model:           sw.model,
		Cwd:             sw.cwd,
		Name:            sw.name,
		PreviousSession: sw.previousSession,
		StartedAt:       sw.started,
		Messages:        messages,
	}

	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(sw.filepath, data, 0644)
}

// sessionTitle generates a title from the first user message.
func sessionTitle(messages []t.Message) string {
	for _, m := range messages {
		if m.Role != t.RoleUser {
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

// LoadSessionByIndex loads the Nth most recent session (1-based).
func LoadSessionByIndex(index int) (*Session, error) {
	sessions, err := loadAllSessions()
	if err != nil || len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}
	if index < 1 || index > len(sessions) {
		return nil, fmt.Errorf("session index %d out of range (1-%d)", index, len(sessions))
	}
	return &sessions[index-1], nil
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

// LoadSessionByName loads a session by its name from the filename.
func LoadSessionByName(name string) (*Session, error) {
	dir := sessionPath()
	matches, err := filepath.Glob(filepath.Join(dir, "*_"+name+".json"))
	if err != nil || len(matches) == 0 {
		return nil, fmt.Errorf("session %q not found", name)
	}
	// Use the most recent match (last alphabetically since filenames start with timestamp)
	fp := matches[len(matches)-1]
	data, err := os.ReadFile(fp)
	if err != nil {
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

// LoadLastSession loads the most recent session.
func LoadLastSession() (*Session, error) {
	sessions, err := loadAllSessions()
	if err != nil || len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}
	return &sessions[0], nil
}

// SessionSummary is a lightweight representation for JSON output.
type SessionSummary struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Model        string    `json:"model"`
	Name         string    `json:"name,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	MessageCount int       `json:"message_count"`
	LastActivity time.Time `json:"last_activity"`
}

// parseSince parses a human duration string (e.g. "2d", "1w", "3h") into a time.Time cutoff.
func parseSince(s string) (time.Time, error) {
	var d time.Duration
	switch {
	case strings.HasSuffix(s, "w"):
		n, err := strconv.Atoi(strings.TrimSuffix(s, "w"))
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid duration %q", s)
		}
		d = time.Duration(n) * 7 * 24 * time.Hour
	case strings.HasSuffix(s, "d"):
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid duration %q", s)
		}
		d = time.Duration(n) * 24 * time.Hour
	default:
		var err error
		d, err = time.ParseDuration(s)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid duration %q", s)
		}
	}
	return time.Now().Add(-d), nil
}

// ListSessions prints saved sessions. limit=-1 for all. since is zero to skip filtering.
// Outputs JSON when stdout is not a terminal.
func ListSessions(limit int, since time.Time) {
	sessions, err := loadAllSessions()
	if err != nil || len(sessions) == 0 {
		fmt.Fprintf(os.Stderr, "no sessions found\n")
		return
	}

	if !since.IsZero() {
		filtered := sessions[:0]
		for _, s := range sessions {
			if lastMessageTime(s).After(since) {
				filtered = append(filtered, s)
			}
		}
		sessions = filtered
		if len(sessions) == 0 {
			fmt.Fprintf(os.Stderr, "no sessions found\n")
			return
		}
	}

	total := len(sessions)
	if limit > 0 && total > limit {
		sessions = sessions[:limit]
	}

	if !term.IsTerminal(int(os.Stdout.Fd())) {
		summaries := make([]SessionSummary, len(sessions))
		for i, sess := range sessions {
			msgCount := 0
			for _, m := range sess.Messages {
				if m.Role != t.RoleSystem {
					msgCount++
				}
			}
			summaries[i] = SessionSummary{
				ID:           sess.ID,
				Title:        sess.Title,
				Model:        sess.Model,
				Name:         sess.Name,
				StartedAt:    sess.StartedAt,
				MessageCount: msgCount,
				LastActivity: lastMessageTime(sess),
			}
		}
		data, _ := json.MarshalIndent(summaries, "", "  ")
		fmt.Println(string(data))
		return
	}

	termWidth, _, _ := term.GetSize(int(os.Stdout.Fd()))
	if termWidth <= 0 {
		termWidth = 80
	}

	for idx, sess := range sessions {
		title := sess.Title
		if title == "" {
			for _, m := range sess.Messages {
				if m.Role == t.RoleUser {
					title = m.Content
					break
				}
			}
		}
		title = strings.ReplaceAll(title, "\n", " ")

		msgCount := 0
		for _, m := range sess.Messages {
			if m.Role != t.RoleSystem {
				msgCount++
			}
		}

		age := relativeTime(lastMessageTime(sess))
		short := sess.ID[:8]
		if sess.Name != "" {
			short = fmt.Sprintf("%s [%s]", short, sess.Name)
		}

		counter := fmt.Sprintf("%d.", idx+1)
		meta := fmt.Sprintf("(%s, %d msgs)", age, msgCount)
		maxTitle := termWidth - len(counter) - len(short) - len(meta) - 3
		if maxTitle < 10 {
			maxTitle = 10
		}
		titleRunes := []rune(title)
		if len(titleRunes) > maxTitle {
			title = string(titleRunes[:maxTitle-1]) + "…"
		}

		fmt.Printf("%s%s%s %s %s %s%s%s\n", dim, counter, reset, short, title, dim, meta, reset)
	}

	if limit > 0 && total > limit {
		fmt.Fprintf(os.Stderr, "\n%sshowing %d of %d sessions, use -all to see all%s\n", dim, limit, total, reset)
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
