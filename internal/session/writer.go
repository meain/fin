package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/meain/fin/internal/config"
	t "github.com/meain/fin/internal/types"
)

// Writer handles incremental session saving to a stable file. Appends new
// messages by default; falls back to a full rewrite when the header changes
// (title set), on the first save, or when the in-memory log is shorter than
// what's on disk (e.g. after compaction).
type Writer struct {
	id              string
	model           string
	cwd             string
	name            string
	previousSession string
	title           string // LLM-generated title override
	temp            bool
	started         time.Time
	filepath        string

	lastWrittenCount int       // messages already on disk
	headerDirty      bool      // header changed since last full rewrite
	lastSeenMtime    time.Time // file mtime after our last successful write
	conflicted       bool      // another writer touched the file
}

// NewWriter creates a new session file and returns a writer for incremental
// saves. If id is empty, a new UUID is generated.
func NewWriter(id, model, name string, temp bool) *Writer {
	dir := config.SessionPath()
	os.MkdirAll(dir, 0755)

	if id == "" {
		id = uuid.New().String()
	}
	cwd, _ := os.Getwd()
	suffix := ""
	if temp {
		suffix = "_temp"
	}
	filename := fmt.Sprintf("%s_%s%s.jsonl", time.Now().Format("20060102-150405"), id, suffix)
	if name != "" {
		filename = fmt.Sprintf("%s_%s_%s%s.jsonl", time.Now().Format("20060102-150405"), id, name, suffix)
	}

	return &Writer{
		id:      id,
		model:   model,
		cwd:     cwd,
		name:    name,
		temp:    temp,
		started: time.Now(),
		filepath: filepath.Join(dir, filename),
	}
}

// WriterForExisting creates a writer that resumes an existing session file.
// The first Save after resume is a full rewrite (headerDirty=true) so header
// fields (notably title) reflect the in-memory state; subsequent saves
// append new tail messages.
func WriterForExisting(sess *Session) *Writer {
	dir := config.SessionPath()
	entries, _ := os.ReadDir(dir)
	var fp string
	for _, e := range entries {
		if !e.IsDir() && strings.Contains(e.Name(), sess.ID) {
			fp = filepath.Join(dir, e.Name())
			break
		}
	}
	if fp == "" {
		fp = filepath.Join(dir, fmt.Sprintf("%s_%s.jsonl", time.Now().Format("20060102-150405"), sess.ID))
	}

	w := &Writer{
		id:              sess.ID,
		model:           sess.Model,
		cwd:             sess.Cwd,
		name:            sess.Name,
		previousSession: sess.PreviousSession,
		title:           sess.Title,
		temp:            sess.Temp,
		started:         sess.StartedAt,
		filepath:        fp,
		headerDirty:     true,
	}
	if info, err := os.Stat(fp); err == nil {
		w.lastSeenMtime = info.ModTime()
	}
	return w
}

// ID returns the writer's session ID.
func (w *Writer) ID() string { return w.id }

// SetPreviousSession records the previous session UUID (used after compaction
// to link the new session back to its predecessor).
func (w *Writer) SetPreviousSession(id string) {
	w.previousSession = id
	w.headerDirty = true
}

// SetTitle stores an LLM-generated title that overrides the truncated default.
// Triggers a full-rewrite on the next Save.
func (w *Writer) SetTitle(title string) {
	if title == w.title {
		return
	}
	w.title = title
	w.headerDirty = true
}

// ErrConflict reports that the session file was modified by another process
// between our last save and this one. Once returned, the writer is poisoned
// and all subsequent Save calls return this error.
var ErrConflict = fmt.Errorf("session file modified by another process; refusing to overwrite")

// Save persists messages. Append for new tail messages; fall back to a full
// rewrite when the header changed, on first save, or when the in-memory log
// is shorter than what's on disk.
func (w *Writer) Save(messages []t.Message) error {
	if w.conflicted {
		return ErrConflict
	}
	if err := w.checkUnchanged(); err != nil {
		return err
	}

	var err error
	switch {
	case w.headerDirty || w.lastWrittenCount == 0 || len(messages) < w.lastWrittenCount:
		err = w.fullRewrite(messages)
	case len(messages) == w.lastWrittenCount:
		return nil
	default:
		err = w.appendNew(messages)
	}
	if err != nil {
		return err
	}

	w.recordMtime()
	return nil
}

// checkUnchanged compares the current file mtime against our last
// observation. A mismatch means another process wrote to the file; we mark
// the writer as conflicted and stop writing.
func (w *Writer) checkUnchanged() error {
	info, err := os.Stat(w.filepath)
	if err != nil {
		if os.IsNotExist(err) {
			if !w.lastSeenMtime.IsZero() {
				w.conflicted = true
				return ErrConflict
			}
			return nil
		}
		return err
	}
	if !w.lastSeenMtime.IsZero() && !info.ModTime().Equal(w.lastSeenMtime) {
		w.conflicted = true
		return ErrConflict
	}
	return nil
}

func (w *Writer) recordMtime() {
	if info, err := os.Stat(w.filepath); err == nil {
		w.lastSeenMtime = info.ModTime()
	}
}

func (w *Writer) fullRewrite(messages []t.Message) error {
	title := w.title
	if title == "" {
		title = TitleFromFirstMessage(messages)
	}
	header := sessionHeader{
		ID:              w.id,
		Title:           title,
		Model:           w.model,
		Cwd:             w.cwd,
		Name:            w.name,
		PreviousSession: w.previousSession,
		Temp:            w.temp,
		StartedAt:       w.started,
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(header); err != nil {
		return err
	}
	for i := range messages {
		if err := enc.Encode(messages[i]); err != nil {
			return err
		}
	}

	tmp := w.filepath + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, w.filepath); err != nil {
		os.Remove(tmp)
		return err
	}

	w.lastWrittenCount = len(messages)
	w.headerDirty = false
	return nil
}

func (w *Writer) appendNew(messages []t.Message) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for i := w.lastWrittenCount; i < len(messages); i++ {
		if err := enc.Encode(messages[i]); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(w.filepath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(buf.Bytes()); err != nil {
		return err
	}
	w.lastWrittenCount = len(messages)
	return nil
}
