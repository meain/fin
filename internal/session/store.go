package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/meain/fin/internal/config"
	t "github.com/meain/fin/internal/types"
)

// entry is a cheap (no JSON parse) view of a session file. temp and repo are
// read straight from the filename (see filename.go), so listing sessions
// never needs to open a file.
type entry struct {
	path  string
	name  string
	mtime time.Time
	temp  bool
	repo  string
}

// entries returns all .jsonl session files in the sessions dir, sorted by
// mtime descending. No JSON parsing — uses only dirent + stat, plus filename
// parsing for temp/repo. Files not in the current filename format are
// treated as neither temp nor associated with any repo.
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
		f, _ := parseFilename(e.Name())
		out = append(out, entry{
			path:  filepath.Join(dir, e.Name()),
			name:  e.Name(),
			mtime: info.ModTime(),
			temp:  f.temp,
			repo:  f.repo,
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

// LoadByName loads a session by its name field, using the filename's
// dedicated name field (independent of the temp field, so no name/temp
// ambiguity is possible).
func LoadByName(name string) (*Session, error) {
	dir := config.SessionPath()
	es, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("session %q not found", name)
	}
	var matches []string
	for _, e := range es {
		if e.IsDir() {
			continue
		}
		if f, ok := parseFilename(e.Name()); ok && f.name == name {
			matches = append(matches, filepath.Join(dir, e.Name()))
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("session %q not found", name)
	}
	// Filenames are timestamp-prefixed, so lexicographic order is
	// chronological.
	sort.Strings(matches)
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

// LoadLastTemp loads the most recently modified temporary session.
func LoadLastTemp() (*Session, error) {
	all, err := entries()
	if err != nil {
		return nil, fmt.Errorf("no sessions found")
	}
	for _, e := range all {
		if e.temp {
			return readFile(e.path)
		}
	}
	return nil, fmt.Errorf("no temp sessions found")
}

// LoadLastWithFilter loads the most recently modified permanent session
// matching both the tag and repo filters (either may be empty to skip that
// check). A tag prefixed with "-" negates the filter (sessions that do NOT
// have that tag). Repo is checked from the filename alone (no I/O); tag
// requires reading the header, but only for repo-matching candidates.
func LoadLastWithFilter(tag, repo string) (*Session, error) {
	exclude := strings.HasPrefix(tag, "-")
	if exclude {
		tag = tag[1:]
	}
	es, err := permanentEntries()
	if err != nil || len(es) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}
	for _, e := range es {
		if repo != "" && e.repo != repo {
			continue
		}
		if tag == "" {
			return readFile(e.path)
		}
		h, err := readHeader(e.path)
		if err != nil {
			continue
		}
		if slices.Contains(h.Tags, tag) == exclude {
			continue
		}
		return readFile(e.path)
	}
	if exclude {
		return nil, fmt.Errorf("no session without tag %q found", tag)
	}
	if tag != "" {
		return nil, fmt.Errorf("no session with tag %q found", tag)
	}
	return nil, fmt.Errorf("no session in repo %q found", repo)
}

// LoadSummaries returns up to limit recent sessions (limit<=0 means no
// limit), optionally filtered by since, tag, and/or repo. Returns the parsed
// sessions plus the total number of candidates after all filters so callers
// can show "showing N of M". Pure data — no I/O on stdout.
func LoadSummaries(limit int, since time.Time, tag string, repo string) ([]Session, int, error) {
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

	// Repo filter: checked from the filename alone (no I/O), before any
	// parsing.
	if repo != "" {
		kept := es[:0]
		for _, e := range es {
			if e.repo == repo {
				kept = append(kept, e)
			}
		}
		es = kept
	}

	// No tag filter: fast path — limit before parsing.
	if tag == "" {
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

	// Tag filter needs the header (tags aren't in the filename): parse all
	// remaining candidates, then filter, then limit. A leading "-" on the tag
	// means exclude sessions that have it.
	exclude := strings.HasPrefix(tag, "-")
	if exclude {
		tag = tag[1:]
	}
	paths := make([]string, len(es))
	for i, e := range es {
		paths[i] = e.path
	}
	all := parseFiles(paths)
	filtered := make([]Session, 0, len(all))
	for _, s := range all {
		if slices.Contains(s.Tags, tag) == exclude {
			continue
		}
		filtered = append(filtered, s)
	}
	total := len(filtered)
	if limit > 0 && total > limit {
		filtered = filtered[:limit]
	}
	return filtered, total, nil
}

// Summary is a lightweight representation for JSON output.
type Summary struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Model        string    `json:"model"`
	Name         string    `json:"name,omitempty"`
	Temp         bool      `json:"temp,omitempty"`
	Tags         []string  `json:"tags,omitempty"`
	Repo         string    `json:"repo,omitempty"`
	ParentID     string    `json:"parent_id,omitempty"`
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
			Tags:         sess.Tags,
			Repo:         sess.Repo,
			ParentID:     sess.PreviousSession,
			StartedAt:    sess.StartedAt,
			MessageCount: msgCount,
			LastActivity: LastMessageTime(sess),
		}
	}
	return json.MarshalIndent(summaries, "", "  ")
}

// LoadChain walks up the previous_session chain from sess to the root,
// returning [root, ..., sess] ordered oldest-first. Each hop is a direct
// file read by ID — no directory scan. Capped at 50 hops.
func LoadChain(sess *Session) []*Session {
	chain := []*Session{sess}
	for i := 0; i < 50; i++ {
		cur := chain[0]
		if cur.PreviousSession == "" {
			break
		}
		parent, err := LoadByID(cur.PreviousSession)
		if err != nil {
			break
		}
		chain = append([]*Session{parent}, chain...)
	}
	return chain
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
