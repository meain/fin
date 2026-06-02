package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	t "github.com/meain/fin/internal/types"
)

// readFile parses a JSONL session file line by line. Tolerant of a corrupt
// trailing line — if the very last record fails to decode it is dropped
// silently, since a crash mid-append can truncate the final line. Earlier
// parse errors are fatal.
func readFile(path string) (*Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<25) // up to 32 MiB per line

	var header sessionHeader
	haveHeader := false
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := json.Unmarshal(line, &header); err != nil {
			return nil, fmt.Errorf("session header: %w", err)
		}
		haveHeader = true
		break
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !haveHeader {
		return &Session{}, nil
	}

	sess := &Session{
		ID:              header.ID,
		Title:           header.Title,
		Model:           header.Model,
		Cwd:             header.Cwd,
		Name:            header.Name,
		PreviousSession: header.PreviousSession,
		Temp:            header.Temp,
		StartedAt:       header.StartedAt,
	}

	var pending *t.Message
	pendingTruncated := false
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if pendingTruncated {
			return nil, fmt.Errorf("session message %d: truncated record followed by more data", len(sess.Messages))
		}
		if pending != nil {
			sess.Messages = append(sess.Messages, *pending)
			pending = nil
		}
		var m t.Message
		if err := json.Unmarshal(line, &m); err != nil {
			pendingTruncated = true
			continue
		}
		pending = &m
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if pending != nil {
		sess.Messages = append(sess.Messages, *pending)
	}
	return sess, nil
}

// parseFiles reads and parses the given session files in parallel (8 workers).
// Failed reads are silently skipped. Input order is preserved.
func parseFiles(paths []string) []Session {
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
			s, err := readFile(path)
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

// uuidFromFilename extracts the UUID from a session filename like
// "YYYYMMDD-HHMMSS_<uuid>[_<name>].jsonl".
func uuidFromFilename(name string) string {
	base := strings.TrimSuffix(name, ".jsonl")
	parts := strings.SplitN(base, "_", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}
