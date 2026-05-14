package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	t2 "github.com/meain/fin/internal/types"
)

func TestSessionTitle_FirstUserMessage(t *testing.T) {
	messages := []t2.Message{
		{Role: t2.RoleSystem, Content: "You are an assistant."},
		{Role: t2.RoleUser, Content: "Hello, how are you?"},
		{Role: t2.RoleAssistant, Content: "I'm fine!"},
	}
	title := sessionTitle(messages)
	if title != "Hello, how are you?" {
		t.Errorf("expected %q, got %q", "Hello, how are you?", title)
	}
}

func TestSessionTitle_TruncatesLongMessages(t *testing.T) {
	long := "This is a very long message that should definitely be truncated because it exceeds fifty characters"
	messages := []t2.Message{
		{Role: t2.RoleUser, Content: long},
	}
	title := sessionTitle(messages)
	if len(title) > 55 { // 50 + length of "..." ellipsis character
		t.Errorf("expected title to be truncated, got length %d: %q", len(title), title)
	}
	// Should start with the first 50 chars of the message
	if title[:50] != long[:50] {
		t.Errorf("expected title to start with first 50 chars")
	}
}

func TestSessionTitle_NoUserMessages(t *testing.T) {
	messages := []t2.Message{
		{Role: t2.RoleSystem, Content: "system prompt"},
		{Role: t2.RoleAssistant, Content: "hello"},
	}
	title := sessionTitle(messages)
	if title != "" {
		t.Errorf("expected empty title, got %q", title)
	}
}

func TestSessionTitle_EmptyMessages(t *testing.T) {
	title := sessionTitle(nil)
	if title != "" {
		t.Errorf("expected empty title, got %q", title)
	}
}

func TestSessionTitle_CollapsesWhitespace(t *testing.T) {
	messages := []t2.Message{
		{Role: t2.RoleUser, Content: "  hello   world\n\nnewline  "},
	}
	title := sessionTitle(messages)
	if title != "hello world newline" {
		t.Errorf("expected %q, got %q", "hello world newline", title)
	}
}

func TestSessionWriter_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	sw := &SessionWriter{
		id:       "test-id-1234",
		model:    "anthropic/claude-sonnet",
		cwd:      "/tmp/test",
		started:  time.Now(),
		filepath: filepath.Join(dir, "test_session.json"),
	}

	messages := []t2.Message{
		{Role: t2.RoleUser, Content: "What is Go?", Timestamp: time.Now()},
		{Role: t2.RoleAssistant, Content: "Go is a programming language.", Timestamp: time.Now()},
	}

	if err := sw.Save(messages); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file was written
	data, err := os.ReadFile(sw.filepath)
	if err != nil {
		t.Fatalf("failed to read saved session: %v", err)
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		t.Fatalf("failed to unmarshal session: %v", err)
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

func TestRelativeTime_Now(t *testing.T) {
	result := relativeTime(time.Now())
	if result != "now" {
		t.Errorf("expected %q, got %q", "now", result)
	}
}

func TestRelativeTime_Minutes(t *testing.T) {
	result := relativeTime(time.Now().Add(-5 * time.Minute))
	if result != "5m" {
		t.Errorf("expected %q, got %q", "5m", result)
	}
}

func TestRelativeTime_Hours(t *testing.T) {
	result := relativeTime(time.Now().Add(-3 * time.Hour))
	if result != "3h" {
		t.Errorf("expected %q, got %q", "3h", result)
	}
}

func TestRelativeTime_Days(t *testing.T) {
	result := relativeTime(time.Now().Add(-48 * time.Hour))
	if result != "2d" {
		t.Errorf("expected %q, got %q", "2d", result)
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
	result := lastMessageTime(sess)
	if !result.Equal(now) {
		t.Errorf("expected %v, got %v", now, result)
	}
}

func TestLastMessageTime_SkipsZeroTimestamps(t *testing.T) {
	now := time.Now()
	sess := Session{
		StartedAt: now.Add(-1 * time.Hour),
		Messages: []t2.Message{
			{Role: t2.RoleUser, Content: "hi", Timestamp: now},
			{Role: t2.RoleAssistant, Content: "hello"}, // zero timestamp
		},
	}
	result := lastMessageTime(sess)
	if !result.Equal(now) {
		t.Errorf("expected %v, got %v", now, result)
	}
}

func TestLastMessageTime_FallsBackToStartedAt(t *testing.T) {
	startedAt := time.Now().Add(-1 * time.Hour)
	sess := Session{
		StartedAt: startedAt,
		Messages: []t2.Message{
			{Role: t2.RoleUser, Content: "hi"}, // all zero timestamps
		},
	}
	result := lastMessageTime(sess)
	if !result.Equal(startedAt) {
		t.Errorf("expected StartedAt %v, got %v", startedAt, result)
	}
}

func TestParseSince_Hours(t *testing.T) {
	before := time.Now()
	cutoff, err := parseSince("2h")
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
	cutoff, err := parseSince("3d")
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
	cutoff, err := parseSince("2w")
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
	cutoff, err := parseSince("30m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(cutoff) < 29*time.Minute || time.Since(cutoff) > 31*time.Minute {
		t.Errorf("cutoff %v not ~30 minutes ago", cutoff)
	}
}

func TestParseSince_InvalidSuffix(t *testing.T) {
	_, err := parseSince("foo")
	if err == nil {
		t.Error("expected error for invalid duration")
	}
}

func TestParseSince_InvalidNumber(t *testing.T) {
	for _, s := range []string{"xd", "yw", "zd"} {
		_, err := parseSince(s)
		if err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}

func TestFilterSessionsSince_ZeroCutoff(t *testing.T) {
	sessions := []Session{
		{ID: "a", StartedAt: time.Now().Add(-48 * time.Hour)},
		{ID: "b", StartedAt: time.Now().Add(-1 * time.Hour)},
	}
	result := filterSessionsSince(sessions, time.Time{})
	if len(result) != 2 {
		t.Errorf("zero cutoff should return all sessions, got %d", len(result))
	}
}

func TestFilterSessionsSince_ExcludesOld(t *testing.T) {
	now := time.Now()
	sessions := []Session{
		{
			ID:        "old",
			StartedAt: now.Add(-72 * time.Hour),
			Messages:  []t2.Message{{Role: t2.RoleUser, Timestamp: now.Add(-72 * time.Hour)}},
		},
		{
			ID:        "recent",
			StartedAt: now.Add(-1 * time.Hour),
			Messages:  []t2.Message{{Role: t2.RoleUser, Timestamp: now.Add(-1 * time.Hour)}},
		},
	}
	cutoff := now.Add(-24 * time.Hour)
	result := filterSessionsSince(sessions, cutoff)
	if len(result) != 1 {
		t.Fatalf("expected 1 session, got %d", len(result))
	}
	if result[0].ID != "recent" {
		t.Errorf("expected recent session, got %q", result[0].ID)
	}
}

func TestFilterSessionsSince_AllRecent(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-7 * 24 * time.Hour)
	sessions := []Session{
		{ID: "a", Messages: []t2.Message{{Timestamp: now.Add(-1 * time.Hour)}}},
		{ID: "b", Messages: []t2.Message{{Timestamp: now.Add(-2 * time.Hour)}}},
		{ID: "c", Messages: []t2.Message{{Timestamp: now.Add(-3 * time.Hour)}}},
	}
	result := filterSessionsSince(sessions, cutoff)
	if len(result) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(result))
	}
}

func TestFilterSessionsSince_AllOld(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-1 * time.Hour)
	sessions := []Session{
		{ID: "a", Messages: []t2.Message{{Timestamp: now.Add(-48 * time.Hour)}}},
		{ID: "b", Messages: []t2.Message{{Timestamp: now.Add(-72 * time.Hour)}}},
	}
	result := filterSessionsSince(sessions, cutoff)
	if len(result) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(result))
	}
}

// writeTestSession saves a minimal session JSON to dir for use in ListSessions tests.
func writeTestSession(t *testing.T, dir string, id string, age time.Duration, msgCount int) {
	t.Helper()
	now := time.Now()
	msgs := make([]t2.Message, msgCount)
	for i := range msgs {
		msgs[i] = t2.Message{Role: t2.RoleUser, Content: "test", Timestamp: now.Add(-age)}
	}
	sess := Session{
		ID:        id,
		Title:     "test session " + id,
		Model:     "test/model",
		StartedAt: now.Add(-age),
		Messages:  msgs,
	}
	data, _ := json.MarshalIndent(sess, "", "  ")
	filename := now.Add(-age).Format("20060102-150405") + "_" + id + ".json"
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("writeTestSession: %v", err)
	}
	// Match mtime to the backdated session age so mtime-based filters work.
	backdated := now.Add(-age)
	if err := os.Chtimes(path, backdated, backdated); err != nil {
		t.Fatalf("writeTestSession chtimes: %v", err)
	}
}

func TestListSessions_JSONOutput(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, ".local", "share", "fin", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestSession(t, sessDir, "aaaa1111-0000-0000-0000-000000000000", time.Hour, 2)
	writeTestSession(t, sessDir, "bbbb2222-0000-0000-0000-000000000000", 2*time.Hour, 3)

	t.Setenv("HOME", home)

	// Capture stdout — in test env os.Stdout is not a terminal so JSON path runs.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ListSessions(-1, time.Time{})

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)

	var summaries []SessionSummary
	if err := json.Unmarshal(buf.Bytes(), &summaries); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(summaries) != 2 {
		t.Errorf("expected 2 summaries, got %d", len(summaries))
	}
	for _, s := range summaries {
		if s.ID == "" || s.Model == "" || s.MessageCount == 0 {
			t.Errorf("summary missing fields: %+v", s)
		}
	}
}

func TestListSessions_SinceFiltersJSON(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, ".local", "share", "fin", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestSession(t, sessDir, "recent11-0000-0000-0000-000000000000", time.Hour, 1)
	writeTestSession(t, sessDir, "old11111-0000-0000-0000-000000000000", 72*time.Hour, 1)

	t.Setenv("HOME", home)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	since := time.Now().Add(-24 * time.Hour)
	ListSessions(-1, since)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)

	var summaries []SessionSummary
	if err := json.Unmarshal(buf.Bytes(), &summaries); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(summaries) != 1 {
		t.Errorf("expected 1 summary (recent only), got %d", len(summaries))
	}
	if len(summaries) > 0 && summaries[0].ID != "recent11-0000-0000-0000-000000000000" {
		t.Errorf("expected recent session, got %q", summaries[0].ID)
	}
}
