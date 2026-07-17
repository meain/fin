package session

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	t2 "github.com/meain/fin/internal/types"
)

func TestTitleFromFirstMessage(t *testing.T) {
	messages := []t2.Message{
		{Role: t2.RoleSystem, Content: "You are an assistant."},
		{Role: t2.RoleUser, Content: "Hello, how are you?"},
		{Role: t2.RoleAssistant, Content: "I'm fine!"},
	}
	if title := TitleFromFirstMessage(messages); title != "Hello, how are you?" {
		t.Errorf("expected %q, got %q", "Hello, how are you?", title)
	}
}

func TestTitleFromFirstMessage_Truncates(t *testing.T) {
	long := "This is a very long message that should definitely be truncated because it exceeds fifty characters"
	title := TitleFromFirstMessage([]t2.Message{{Role: t2.RoleUser, Content: long}})
	if len(title) > 55 {
		t.Errorf("expected truncated, got length %d: %q", len(title), title)
	}
	if title[:50] != long[:50] {
		t.Errorf("expected title to start with first 50 chars")
	}
}

// TestTitleFromFirstMessage_TruncatesMultiByteRunes guards against byte-based
// slicing (s[:50]) cutting a multi-byte UTF-8 rune in half, which produces
// invalid UTF-8 that renders as mojibake wherever the title is shown.
func TestTitleFromFirstMessage_TruncatesMultiByteRunes(t *testing.T) {
	long := strings.Repeat("世", 60) // each rune is 3 bytes; 60 runes > 50-rune cutoff
	title := TitleFromFirstMessage([]t2.Message{{Role: t2.RoleUser, Content: long}})
	if !utf8.ValidString(title) {
		t.Fatalf("truncated title is not valid UTF-8: %q", title)
	}
	runes := []rune(title)
	// 50 content runes + the ellipsis rune appended.
	if len(runes) != 51 {
		t.Fatalf("expected 51 runes (50 + ellipsis), got %d: %q", len(runes), title)
	}
}

func TestTitleFromFirstMessage_NoUserMessages(t *testing.T) {
	if title := TitleFromFirstMessage([]t2.Message{
		{Role: t2.RoleSystem, Content: "system prompt"},
		{Role: t2.RoleAssistant, Content: "hello"},
	}); title != "" {
		t.Errorf("expected empty title, got %q", title)
	}
}

func TestTitleFromFirstMessage_Empty(t *testing.T) {
	if title := TitleFromFirstMessage(nil); title != "" {
		t.Errorf("expected empty title, got %q", title)
	}
}

func TestTitleFromFirstMessage_CollapsesWhitespace(t *testing.T) {
	title := TitleFromFirstMessage([]t2.Message{
		{Role: t2.RoleUser, Content: "  hello   world\n\nnewline  "},
	})
	if title != "hello world newline" {
		t.Errorf("expected %q, got %q", "hello world newline", title)
	}
}

func TestWriter_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	w := &Writer{
		id:       "test-id-1234",
		model:    "anthropic/claude-sonnet",
		cwd:      "/tmp/test",
		started:  time.Now(),
		filepath: filepath.Join(dir, "test_session.jsonl"),
	}

	messages := []t2.Message{
		{Role: t2.RoleUser, Content: "What is Go?", Timestamp: time.Now()},
		{Role: t2.RoleAssistant, Content: "Go is a programming language.", Timestamp: time.Now()},
	}

	if err := w.Save(messages); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	sess, err := readFile(w.filepath)
	if err != nil {
		t.Fatalf("failed to read saved session: %v", err)
	}
	if sess.ID != "test-id-1234" {
		t.Errorf("expected ID %q, got %q", "test-id-1234", sess.ID)
	}
	if sess.Model != "anthropic/claude-sonnet" {
		t.Errorf("expected model %q, got %q", "anthropic/claude-sonnet", sess.Model)
	}
	if sess.Title != "What is Go?" {
		t.Errorf("expected title %q, got %q", "What is Go?", sess.Title)
	}
	if len(sess.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(sess.Messages))
	}
}

func TestWriter_AppendOnly(t *testing.T) {
	dir := t.TempDir()
	w := &Writer{
		id:       "append-test",
		model:    "m",
		started:  time.Now(),
		filepath: filepath.Join(dir, "append.jsonl"),
	}
	msgs := []t2.Message{{Role: t2.RoleUser, Content: "a"}}
	if err := w.Save(msgs); err != nil {
		t.Fatal(err)
	}
	statBefore, _ := os.Stat(w.filepath)
	msgs = append(msgs, t2.Message{Role: t2.RoleAssistant, Content: "b"})
	if err := w.Save(msgs); err != nil {
		t.Fatal(err)
	}
	statAfter, _ := os.Stat(w.filepath)
	if statAfter.Size() <= statBefore.Size() {
		t.Fatalf("expected file to grow, before=%d after=%d", statBefore.Size(), statAfter.Size())
	}
	sess, err := readFile(w.filepath)
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(sess.Messages))
	}
}

func TestWriter_ResumeAppends(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, ".local", "share", "fin", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	w := NewWriter("resume-id", "m", "", false, nil)
	if err := w.Save([]t2.Message{{Role: t2.RoleUser, Content: "first"}}); err != nil {
		t.Fatal(err)
	}
	originalPath := w.filepath

	loaded, err := readFile(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(loaded.Messages))
	}

	w2 := WriterForExisting(loaded)
	if w2.filepath != originalPath {
		t.Fatalf("expected reuse of %s, got %s", originalPath, w2.filepath)
	}
	if !w2.headerDirty {
		t.Fatal("resumed writer should have headerDirty=true")
	}

	msgs := append(loaded.Messages, t2.Message{Role: t2.RoleAssistant, Content: "reply"})
	if err := w2.Save(msgs); err != nil {
		t.Fatal(err)
	}
	if w2.headerDirty {
		t.Error("headerDirty should be cleared after rewrite")
	}
	if w2.lastWrittenCount != 2 {
		t.Errorf("expected lastWrittenCount=2, got %d", w2.lastWrittenCount)
	}

	sizeBefore, _ := os.Stat(w2.filepath)
	msgs = append(msgs, t2.Message{Role: t2.RoleUser, Content: "third"})
	if err := w2.Save(msgs); err != nil {
		t.Fatal(err)
	}
	sizeAfter, _ := os.Stat(w2.filepath)
	if sizeAfter.Size() <= sizeBefore.Size() {
		t.Errorf("expected file to grow on append, before=%d after=%d", sizeBefore.Size(), sizeAfter.Size())
	}

	final, err := readFile(w2.filepath)
	if err != nil {
		t.Fatal(err)
	}
	if len(final.Messages) != 3 {
		t.Errorf("expected 3 messages after append, got %d", len(final.Messages))
	}
	if final.Messages[2].Content != "third" {
		t.Errorf("expected third message preserved, got %q", final.Messages[2].Content)
	}
}

func TestWriter_ConflictDetected(t *testing.T) {
	dir := t.TempDir()
	w := &Writer{
		id:       "conflict-test",
		model:    "m",
		started:  time.Now(),
		filepath: filepath.Join(dir, "conflict.jsonl"),
	}
	msgs := []t2.Message{{Role: t2.RoleUser, Content: "first"}}
	if err := w.Save(msgs); err != nil {
		t.Fatal(err)
	}

	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(w.filepath, future, future); err != nil {
		t.Fatal(err)
	}

	msgs = append(msgs, t2.Message{Role: t2.RoleAssistant, Content: "second"})
	if err := w.Save(msgs); err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !w.conflicted {
		t.Error("writer should be marked conflicted after mismatch")
	}
	statBefore, _ := os.Stat(w.filepath)
	if err := w.Save(msgs); err == nil {
		t.Fatal("expected conflict error on subsequent save, got nil")
	}
	statAfter, _ := os.Stat(w.filepath)
	if statAfter.Size() != statBefore.Size() {
		t.Errorf("file should not change after conflict; before=%d after=%d", statBefore.Size(), statAfter.Size())
	}
}

func TestWriter_ResumeDetectsExternalWrite(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, ".local", "share", "fin", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	w := NewWriter("ext-test", "m", "", false, nil)
	if err := w.Save([]t2.Message{{Role: t2.RoleUser, Content: "first"}}); err != nil {
		t.Fatal(err)
	}
	loaded, err := readFile(w.filepath)
	if err != nil {
		t.Fatal(err)
	}

	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(w.filepath, future, future); err != nil {
		t.Fatal(err)
	}

	resumed := WriterForExisting(loaded)
	further := time.Now().Add(4 * time.Second)
	if err := os.Chtimes(w.filepath, further, further); err != nil {
		t.Fatal(err)
	}

	msgs := append(loaded.Messages, t2.Message{Role: t2.RoleAssistant, Content: "second"})
	if err := resumed.Save(msgs); err == nil {
		t.Fatal("expected conflict on resumed save after external mtime change, got nil")
	}
}

func TestReadFile_DropsTruncatedTrailingLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "truncated.jsonl")
	w := &Writer{
		id:       "trunc-test",
		model:    "m",
		started:  time.Now(),
		filepath: path,
	}
	msgs := []t2.Message{
		{Role: t2.RoleUser, Content: "first"},
		{Role: t2.RoleAssistant, Content: "second"},
	}
	if err := w.Save(msgs); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"role":"user","content":"halfwr`); err != nil {
		t.Fatal(err)
	}
	f.Close()

	sess, err := readFile(path)
	if err != nil {
		t.Fatalf("reader should tolerate trailing corrupt line, got: %v", err)
	}
	if len(sess.Messages) != 2 {
		t.Fatalf("expected 2 messages (trailing corrupt dropped), got %d", len(sess.Messages))
	}
	if sess.Messages[1].Content != "second" {
		t.Errorf("expected last good message preserved, got %q", sess.Messages[1].Content)
	}
}

func TestReadFile_RejectsCorruptMiddleLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt-middle.jsonl")
	content := `{"id":"x","title":"t","model":"m","cwd":"","started_at":"2026-01-01T00:00:00Z"}
{"role":"user","content":"first"}
not-json-at-all
{"role":"assistant","content":"third"}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readFile(path); err == nil {
		t.Fatal("expected error for corrupt middle line, got nil")
	}
}

func TestWriter_SetTitleTriggersRewrite(t *testing.T) {
	dir := t.TempDir()
	w := &Writer{
		id:       "title-test",
		model:    "m",
		started:  time.Now(),
		filepath: filepath.Join(dir, "title.jsonl"),
	}
	msgs := []t2.Message{{Role: t2.RoleUser, Content: "hello"}}
	if err := w.Save(msgs); err != nil {
		t.Fatal(err)
	}
	w.SetTitle("My Title")
	msgs = append(msgs, t2.Message{Role: t2.RoleAssistant, Content: "hi"})
	if err := w.Save(msgs); err != nil {
		t.Fatal(err)
	}
	sess, err := readFile(w.filepath)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Title != "My Title" {
		t.Errorf("expected title %q, got %q", "My Title", sess.Title)
	}
	if len(sess.Messages) != 2 {
		t.Errorf("expected 2 messages after rewrite, got %d", len(sess.Messages))
	}
}

func TestLastMessageTime_ReturnsLastTimestamp(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-10 * time.Minute)
	sess := Session{
		StartedAt: earlier.Add(-1 * time.Hour),
		Messages: []t2.Message{
			{Role: t2.RoleUser, Content: "hi", Timestamp: earlier},
			{Role: t2.RoleAssistant, Content: "hello", Timestamp: now},
		},
	}
	if got := LastMessageTime(sess); !got.Equal(now) {
		t.Errorf("expected %v, got %v", now, got)
	}
}

func TestLastMessageTime_SkipsZeroTimestamps(t *testing.T) {
	now := time.Now()
	sess := Session{
		StartedAt: now.Add(-1 * time.Hour),
		Messages: []t2.Message{
			{Role: t2.RoleUser, Content: "hi", Timestamp: now},
			{Role: t2.RoleAssistant, Content: "hello"},
		},
	}
	if got := LastMessageTime(sess); !got.Equal(now) {
		t.Errorf("expected %v, got %v", now, got)
	}
}

func TestLastMessageTime_FallsBackToStartedAt(t *testing.T) {
	startedAt := time.Now().Add(-1 * time.Hour)
	sess := Session{
		StartedAt: startedAt,
		Messages:  []t2.Message{{Role: t2.RoleUser, Content: "hi"}},
	}
	if got := LastMessageTime(sess); !got.Equal(startedAt) {
		t.Errorf("expected StartedAt %v, got %v", startedAt, got)
	}
}

func TestParseSince_Hours(t *testing.T) {
	before := time.Now()
	cutoff, err := ParseSince("2h")
	after := time.Now()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	low := before.Add(-2*time.Hour - time.Second)
	high := after.Add(-2*time.Hour + time.Second)
	if cutoff.Before(low) || cutoff.After(high) {
		t.Errorf("cutoff %v not within expected range for 2h", cutoff)
	}
}

func TestParseSince_Days(t *testing.T) {
	before := time.Now()
	cutoff, err := ParseSince("3d")
	after := time.Now()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := 3 * 24 * time.Hour
	low := before.Add(-expected - time.Second)
	high := after.Add(-expected + time.Second)
	if cutoff.Before(low) || cutoff.After(high) {
		t.Errorf("cutoff %v not within expected range for 3d", cutoff)
	}
}

func TestParseSince_Weeks(t *testing.T) {
	before := time.Now()
	cutoff, err := ParseSince("2w")
	after := time.Now()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := 2 * 7 * 24 * time.Hour
	low := before.Add(-expected - time.Second)
	high := after.Add(-expected + time.Second)
	if cutoff.Before(low) || cutoff.After(high) {
		t.Errorf("cutoff %v not within expected range for 2w", cutoff)
	}
}

func TestParseSince_Minutes(t *testing.T) {
	cutoff, err := ParseSince("30m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(cutoff) < 29*time.Minute || time.Since(cutoff) > 31*time.Minute {
		t.Errorf("cutoff %v not ~30 minutes ago", cutoff)
	}
}

func TestParseSince_InvalidSuffix(t *testing.T) {
	if _, err := ParseSince("foo"); err == nil {
		t.Error("expected error for invalid duration")
	}
}

func TestParseSince_InvalidNumber(t *testing.T) {
	for _, s := range []string{"xd", "yw", "zd"} {
		if _, err := ParseSince(s); err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}

// writeTestSession saves a minimal session JSONL to dir for use in
// LoadSummaries tests.
func writeTestSession(t *testing.T, dir string, id string, age time.Duration, msgCount int) {
	t.Helper()
	writeTestSessionFull(t, dir, id, age, msgCount, "", nil)
}

// writeTestSessionWithRepo is like writeTestSession but also stamps the
// session (header and filename) with the given repo and an optional tag, for
// repo/tag filtering tests.
func writeTestSessionWithRepo(t *testing.T, dir string, id string, age time.Duration, repo string, tags []string) {
	t.Helper()
	writeTestSessionFull(t, dir, id, age, 1, repo, tags)
}

// writeTestSessionFull writes a session file in the current filename format
// (see filename.go), backdated by age, for use in store.go tests.
func writeTestSessionFull(t *testing.T, dir string, id string, age time.Duration, msgCount int, repo string, tags []string) {
	t.Helper()
	now := time.Now()
	backdated := now.Add(-age)
	msgs := make([]t2.Message, msgCount)
	for i := range msgs {
		msgs[i] = t2.Message{Role: t2.RoleUser, Content: "test", Timestamp: backdated}
	}
	filename := buildFilename(backdated.Format("20060102-150405"), id, repo, "", false)
	path := filepath.Join(dir, filename)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(sessionHeader{
		ID:        id,
		Title:     "test session " + id,
		Model:     "test/model",
		Repo:      repo,
		Tags:      tags,
		StartedAt: backdated,
	}); err != nil {
		t.Fatalf("writeTestSessionFull encode header: %v", err)
	}
	for i := range msgs {
		if err := enc.Encode(msgs[i]); err != nil {
			t.Fatalf("writeTestSessionFull encode msg: %v", err)
		}
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("writeTestSessionFull: %v", err)
	}
	if err := os.Chtimes(path, backdated, backdated); err != nil {
		t.Fatalf("writeTestSessionFull chtimes: %v", err)
	}
}

func TestLoadSummaries_AllRecent(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, ".local", "share", "fin", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestSession(t, sessDir, "aaaa1111-0000-0000-0000-000000000000", time.Hour, 2)
	writeTestSession(t, sessDir, "bbbb2222-0000-0000-0000-000000000000", 2*time.Hour, 3)

	t.Setenv("HOME", home)

	sessions, total, err := LoadSummaries(-1, time.Time{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || total != 2 {
		t.Errorf("expected 2/2, got %d/%d", len(sessions), total)
	}
}

// TestEntries_NamePrefixNotMisdetectedAsTemp guards against name/temp
// ambiguity: a permanent session named e.g. "foo_temp_report" must not be
// misclassified as temporary just because "_temp" appears in its name. The
// filename format's dedicated name and temp fields make this structurally
// impossible (see filename.go), unlike a suffix-based heuristic.
func TestEntries_NamePrefixNotMisdetectedAsTemp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	w := NewWriter("", "test/model", "foo_temp_report", false, nil)
	if err := w.Save([]t2.Message{{Role: t2.RoleUser, Content: "hi", Timestamp: time.Now()}}); err != nil {
		t.Fatalf("save: %v", err)
	}

	es, err := entries()
	if err != nil {
		t.Fatalf("entries: %v", err)
	}
	if len(es) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(es))
	}
	if es[0].temp {
		t.Errorf("session named %q was misclassified as temp", w.name)
	}

	perm, err := permanentEntries()
	if err != nil {
		t.Fatalf("permanentEntries: %v", err)
	}
	if len(perm) != 1 {
		t.Errorf("expected the permanent session to be included, permanentEntries returned %d", len(perm))
	}
}

// TestLoadByName_FindsNamedTempSession guards against LoadByName failing to
// match a named session that was also created with -temp, since name and
// temp are independent fields in the filename.
func TestLoadByName_FindsNamedTempSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	w := NewWriter("", "test/model", "mysession", true, nil)
	if err := w.Save([]t2.Message{{Role: t2.RoleUser, Content: "hi", Timestamp: time.Now()}}); err != nil {
		t.Fatalf("save: %v", err)
	}

	sess, err := LoadByName("mysession")
	if err != nil {
		t.Fatalf("LoadByName(%q) failed: %v", "mysession", err)
	}
	if sess.ID != w.ID() {
		t.Errorf("expected session ID %q, got %q", w.ID(), sess.ID)
	}
	if !sess.Temp {
		t.Errorf("expected loaded session to be marked temp")
	}
}

// TestEntries_NameEndingInTempNotMisdetected guards against a session whose
// *name* literally ends in "_temp" (e.g. "-n my_temp") being misclassified as
// a temp session when temp=false. A suffix-based heuristic on the whole
// filename can't tell these apart; the dedicated temp field can.
func TestEntries_NameEndingInTempNotMisdetected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	w := NewWriter("", "test/model", "my_temp", false, nil)
	if err := w.Save([]t2.Message{{Role: t2.RoleUser, Content: "hi", Timestamp: time.Now()}}); err != nil {
		t.Fatalf("save: %v", err)
	}

	es, err := entries()
	if err != nil {
		t.Fatalf("entries: %v", err)
	}
	if len(es) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(es))
	}
	if es[0].temp {
		t.Errorf("session named %q was misclassified as temp", w.name)
	}

	sess, err := LoadByName("my_temp")
	if err != nil {
		t.Fatalf("LoadByName(%q) failed: %v", "my_temp", err)
	}
	if sess.Temp {
		t.Errorf("expected loaded session to not be marked temp")
	}
}

func TestLoadSummaries_SinceFilter(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, ".local", "share", "fin", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestSession(t, sessDir, "recent11-0000-0000-0000-000000000000", time.Hour, 1)
	writeTestSession(t, sessDir, "old11111-0000-0000-0000-000000000000", 72*time.Hour, 1)

	t.Setenv("HOME", home)

	since := time.Now().Add(-24 * time.Hour)
	sessions, total, err := LoadSummaries(-1, since, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || total != 1 {
		t.Errorf("expected 1/1, got %d/%d", len(sessions), total)
	}
	if len(sessions) > 0 && sessions[0].ID != "recent11-0000-0000-0000-000000000000" {
		t.Errorf("expected recent session, got %q", sessions[0].ID)
	}
}

func TestNewWriter_SetsRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	w := NewWriter("", "test/model", "", false, nil)
	if w.repo == "" {
		t.Errorf("expected NewWriter to set a non-empty repo")
	}
}

func TestLoadSummaries_RepoFilter(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, ".local", "share", "fin", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestSessionWithRepo(t, sessDir, "reposesh1-0000-0000-0000-000000000000", time.Hour, "fin", nil)
	writeTestSessionWithRepo(t, sessDir, "reposesh2-0000-0000-0000-000000000000", 2*time.Hour, "other-repo", nil)

	t.Setenv("HOME", home)

	sessions, total, err := LoadSummaries(-1, time.Time{}, "", "fin")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || total != 1 {
		t.Fatalf("expected 1/1, got %d/%d", len(sessions), total)
	}
	if sessions[0].ID != "reposesh1-0000-0000-0000-000000000000" {
		t.Errorf("expected fin-repo session, got %q", sessions[0].ID)
	}
}

func TestLoadLastWithFilter_Repo(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, ".local", "share", "fin", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestSessionWithRepo(t, sessDir, "reposesh3-0000-0000-0000-000000000000", time.Hour, "other-repo", nil)
	writeTestSessionWithRepo(t, sessDir, "reposesh4-0000-0000-0000-000000000000", 2*time.Hour, "fin", nil)

	t.Setenv("HOME", home)

	sess, err := LoadLastWithFilter("", "fin")
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID != "reposesh4-0000-0000-0000-000000000000" {
		t.Errorf("expected fin-repo session, got %q", sess.ID)
	}

	if _, err := LoadLastWithFilter("", "no-such-repo"); err == nil {
		t.Errorf("expected error for repo with no matching sessions")
	}
}

func TestLoadLastWithFilter_TagAndRepoCombined(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, ".local", "share", "fin", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestSessionWithRepo(t, sessDir, "reposesh5-0000-0000-0000-000000000000", time.Hour, "fin", []string{"other"})
	writeTestSessionWithRepo(t, sessDir, "reposesh6-0000-0000-0000-000000000000", 2*time.Hour, "fin", []string{"work"})
	writeTestSessionWithRepo(t, sessDir, "reposesh7-0000-0000-0000-000000000000", 3*time.Hour, "other-repo", []string{"work"})

	t.Setenv("HOME", home)

	sess, err := LoadLastWithFilter("work", "fin")
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID != "reposesh6-0000-0000-0000-000000000000" {
		t.Errorf("expected session tagged work in fin repo, got %q", sess.ID)
	}
}
