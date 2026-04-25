package main

import (
	"encoding/json"
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
