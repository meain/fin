package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	t "github.com/meain/fin/internal/types"
	"golang.org/x/term"
)

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

// SessionWriter handles incremental session saving to a stable file.
type SessionWriter struct {
	id              string
	model           string
	cwd             string
	name            string
	previousSession string
	title           string // LLM-generated title override
	started         time.Time
	filepath        string
}

// NewSessionWriter creates a new session file and returns a writer for incremental saves.
// If id is empty, a new UUID is generated.
func NewSessionWriter(id, model, name string) *SessionWriter {
	dir := sessionPath()
	os.MkdirAll(dir, 0755)

	if id == "" {
		id = uuid.New().String()
	}
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

// SetTitle sets an LLM-generated title that overrides the default truncated-message title.
func (sw *SessionWriter) SetTitle(title string) {
	sw.title = title
}

// Save writes the current messages to disk.
func (sw *SessionWriter) Save(messages []t.Message) error {
	title := sw.title
	if title == "" {
		title = sessionTitle(messages)
	}
	sess := Session{
		ID:              sw.id,
		Title:           title,
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

// sessionEntry is a cheap (no JSON parse) view of a session file.
type sessionEntry struct {
	path  string
	name  string
	mtime time.Time
}

// sessionEntries returns all session files in the sessions dir, sorted by mtime desc.
// No JSON parsing — uses only dirent + stat.
func sessionEntries() ([]sessionEntry, error) {
	dir := sessionPath()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]sessionEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, sessionEntry{
			path:  filepath.Join(dir, e.Name()),
			name:  e.Name(),
			mtime: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].mtime.After(out[j].mtime) })
	return out, nil
}

// readSessionFile reads and unmarshals a single session JSON file.
func readSessionFile(path string) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

// parseSessionFiles reads and parses the given session files in parallel.
// Failed reads are silently skipped. Input order is preserved.
func parseSessionFiles(paths []string) []Session {
	out := make([]Session, len(paths))
	ok := make([]bool, len(paths))
	const workers = 8
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i, p := range paths {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, path string) {
			defer wg.Done()
			defer func() { <-sem }()
			s, err := readSessionFile(path)
			if err != nil {
				return
			}
			out[idx] = *s
			ok[idx] = true
		}(i, p)
	}
	wg.Wait()
	filtered := make([]Session, 0, len(paths))
	for i, v := range ok {
		if v {
			filtered = append(filtered, out[i])
		}
	}
	return filtered
}

// uuidFromFilename extracts the UUID portion of a session filename.
// Filenames are "YYYYMMDD-HHMMSS_<uuid>[_<name>].json".
func uuidFromFilename(name string) string {
	base := strings.TrimSuffix(name, ".json")
	parts := strings.SplitN(base, "_", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// LoadSessionByIndex loads the Nth most recent session (1-based).
// Ranking by mtime; parses only the chosen file.
func LoadSessionByIndex(index int) (*Session, error) {
	entries, err := sessionEntries()
	if err != nil || len(entries) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}
	if index < 1 || index > len(entries) {
		return nil, fmt.Errorf("session index %d out of range (1-%d)", index, len(entries))
	}
	return readSessionFile(entries[index-1].path)
}

// LoadSessionByID loads a session by its UUID (or prefix) using the filename.
// Parses only the matching file.
func LoadSessionByID(id string) (*Session, error) {
	entries, err := sessionEntries()
	if err != nil {
		return nil, fmt.Errorf("session %s not found", id)
	}
	for _, e := range entries {
		uuidPart := uuidFromFilename(e.name)
		if uuidPart == "" {
			continue
		}
		if uuidPart == id || strings.HasPrefix(uuidPart, id) {
			return readSessionFile(e.path)
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
	// Use the most recent match (last alphabetically since filenames start with timestamp).
	return readSessionFile(matches[len(matches)-1])
}

// LoadLastSession loads the most recently modified session.
func LoadLastSession() (*Session, error) {
	entries, err := sessionEntries()
	if err != nil || len(entries) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}
	return readSessionFile(entries[0].path)
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

// filterSessionsSince returns sessions whose last activity is after the cutoff.
// A zero cutoff returns the slice unchanged. Kept for tests; ListSessions
// uses mtime-based filtering on file entries before parsing.
func filterSessionsSince(sessions []Session, since time.Time) []Session {
	if since.IsZero() {
		return sessions
	}
	var filtered []Session
	for _, s := range sessions {
		if lastMessageTime(s).After(since) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// ListSessions prints saved sessions. limit=-1 for all. since is zero to skip filtering.
// Outputs JSON when stdout is not a terminal.
// Pre-ranks by mtime (cheap), then parses only the kept slice.
func ListSessions(limit int, since time.Time) {
	entries, err := sessionEntries()
	if err != nil || len(entries) == 0 {
		fmt.Fprintf(os.Stderr, "no sessions found\n")
		return
	}

	if !since.IsZero() {
		kept := entries[:0]
		for _, e := range entries {
			if e.mtime.After(since) {
				kept = append(kept, e)
			}
		}
		entries = kept
	}
	if len(entries) == 0 {
		fmt.Fprintf(os.Stderr, "no sessions found\n")
		return
	}

	total := len(entries)
	if limit > 0 && total > limit {
		entries = entries[:limit]
	}

	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.path
	}
	sessions := parseSessionFiles(paths)

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
