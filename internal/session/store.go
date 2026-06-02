package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/meain/fin/internal/config"
	t "github.com/meain/fin/internal/types"
)

// entry is a cheap (no JSON parse) view of a session file.
type entry struct {
	path  string
	name  string
	mtime time.Time
	temp  bool
}

// entries returns all .jsonl session files in the sessions dir, sorted by
// mtime descending. No JSON parsing — uses only dirent + stat.
// Detects temp sessions from the _temp filename suffix.
func entries() ([]entry, error) {
	dir := config.SessionPath()
	es, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]entry, 0, len(es))
	for _, e := range es {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		temp := strings.Contains(e.Name(), "_temp")
		out = append(out, entry{
			path:  filepath.Join(dir, e.Name()),
			name:  e.Name(),
			mtime: info.ModTime(),
			temp:  temp,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].mtime.After(out[j].mtime) })
	return out, nil
}

// permanentEntries returns non-temp session entries, sorted by mtime descending.
func permanentEntries() ([]entry, error) {
	all, err := entries()
	if err != nil {
		return nil, err
	}
	out := make([]entry, 0, len(all))
	for _, e := range all {
		if !e.temp {
			out = append(out, e)
		}
	}
	return out, nil
}

// LoadByIndex loads the Nth most recent permanent session (1-based). Temp
// sessions are skipped for index lookup; use LoadByID for temp sessions.
func LoadByIndex(index int) (*Session, error) {
	es, err := permanentEntries()
	if err != nil || len(es) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}
	if index < 1 || index > len(es) {
		return nil, fmt.Errorf("session index %d out of range (1-%d)", index, len(es))
	}
	return readFile(es[index-1].path)
}

// LoadByID loads a session by its UUID (or unique prefix). Includes temp sessions.
func LoadByID(id string) (*Session, error) {
	es, err := entries()
	if err != nil {
		return nil, fmt.Errorf("session %s not found", id)
	}
	for _, e := range es {
		uuidPart := uuidFromFilename(e.name)
		if uuidPart == "" {
			continue
		}
		if uuidPart == id || strings.HasPrefix(uuidPart, id) {
			return readFile(e.path)
		}
	}
	return nil, fmt.Errorf("session %s not found", id)
}

// LoadByName loads a session by its trailing-name segment in the filename.
func LoadByName(name string) (*Session, error) {
	dir := config.SessionPath()
	matches, err := filepath.Glob(filepath.Join(dir, "*_"+name+".jsonl"))
	if err != nil || len(matches) == 0 {
		return nil, fmt.Errorf("session %q not found", name)
	}
	return readFile(matches[len(matches)-1])
}

// LoadLast loads the most recently modified permanent (non-temp) session.
func LoadLast() (*Session, error) {
	es, err := permanentEntries()
	if err != nil || len(es) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}
	return readFile(es[0].path)
}

// LoadSummaries returns up to limit recent sessions (limit<=0 means no
// limit), optionally filtered by since. Returns the parsed sessions plus
// the total number of candidates that passed the since filter so callers
// can show "showing N of M". Pure data — no I/O on stdout.
func LoadSummaries(limit int, since time.Time) ([]Session, int, error) {
	es, err := entries()
	if err != nil {
		return nil, 0, err
	}

	if !since.IsZero() {
		kept := es[:0]
		for _, e := range es {
			if e.mtime.After(since) {
				kept = append(kept, e)
			}
		}
		es = kept
	}
	total := len(es)
	if limit > 0 && total > limit {
		es = es[:limit]
	}

	paths := make([]string, len(es))
	for i, e := range es {
		paths[i] = e.path
	}
	return parseFiles(paths), total, nil
}

// Summary is a lightweight representation for JSON output.
type Summary struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Model        string    `json:"model"`
	Name         string    `json:"name,omitempty"`
	Temp         bool      `json:"temp,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	MessageCount int       `json:"message_count"`
	LastActivity time.Time `json:"last_activity"`
}

// SummariesJSON marshals the given sessions as a JSON array of Summary
// records to w.
func SummariesJSON(sessions []Session) ([]byte, error) {
	summaries := make([]Summary, len(sessions))
	for i, sess := range sessions {
		msgCount := 0
		for _, m := range sess.Messages {
			if m.Role != t.RoleSystem {
				msgCount++
			}
		}
		summaries[i] = Summary{
			ID:           sess.ID,
			Title:        sess.Title,
			Model:        sess.Model,
			Name:         sess.Name,
			Temp:         sess.Temp,
			StartedAt:    sess.StartedAt,
			MessageCount: msgCount,
			LastActivity: LastMessageTime(sess),
		}
	}
	return json.MarshalIndent(summaries, "", "  ")
}

// ParseSince parses a human duration string ("2d", "1w", "3h", "30m") into
// a time.Time cutoff relative to now.
func ParseSince(s string) (time.Time, error) {
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
