package tool

import (
	"context"
	"strings"
	"testing"
)

func TestShellTool_SimpleCommand(t *testing.T) {
	st := &ShellTool{}
	result, err := st.Run(context.Background(), map[string]any{
		"command": "echo hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.TrimSpace(result.Content) != "hello" {
		t.Errorf("expected %q, got %q", "hello", strings.TrimSpace(result.Content))
	}
}

func TestShellTool_Stderr(t *testing.T) {
	st := &ShellTool{}
	result, err := st.Run(context.Background(), map[string]any{
		"command": "echo out && echo err >&2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "out") {
		t.Errorf("expected stdout 'out' in result, got: %q", result.Content)
	}
	if !strings.Contains(result.Content, "stderr:") {
		t.Errorf("expected 'stderr:' label in result, got: %q", result.Content)
	}
	if !strings.Contains(result.Content, "err") {
		t.Errorf("expected stderr 'err' in result, got: %q", result.Content)
	}
}

func TestShellTool_NonZeroExit(t *testing.T) {
	st := &ShellTool{}
	result, err := st.Run(context.Background(), map[string]any{
		"command": "exit 42",
	})
	// Non-zero exit does not return an error, it returns exit code in content
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "exit code: 42") {
		t.Errorf("expected 'exit code: 42' in result, got: %q", result.Content)
	}
}

func TestShellTool_Timeout(t *testing.T) {
	st := &ShellTool{}
	_, err := st.Run(context.Background(), map[string]any{
		"command": "sleep 10",
		"timeout": float64(1),
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("unexpected error: %v", err)
	}
}
