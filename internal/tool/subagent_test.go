package tool

import (
	"context"
	"fmt"
	"testing"
)

func TestSubagentTool_Name(t *testing.T) {
	st := &SubagentTool{}
	if st.Name() != "subagent" {
		t.Errorf("Name() = %q, want %q", st.Name(), "subagent")
	}
}

func TestSubagentTool_Parameters(t *testing.T) {
	st := &SubagentTool{}
	params := st.Parameters()

	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("parameters missing properties")
	}
	if _, ok := props["task"]; !ok {
		t.Error("parameters missing task property")
	}
	if _, ok := props["model"]; !ok {
		t.Error("parameters missing model property")
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("parameters missing required")
	}
	if len(required) != 1 || required[0] != "task" {
		t.Errorf("required = %v, want [task]", required)
	}
}

func TestSubagentTool_EmptyTask(t *testing.T) {
	st := &SubagentTool{
		RunSubagent: func(_ context.Context, _, _ string) (string, error) {
			t.Fatal("should not be called")
			return "", nil
		},
	}

	_, err := st.Run(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for empty task")
	}
}

func TestSubagentTool_NilRunner(t *testing.T) {
	st := &SubagentTool{}

	_, err := st.Run(context.Background(), map[string]any{"task": "do something"})
	if err == nil {
		t.Fatal("expected error for nil runner")
	}
}

func TestSubagentTool_Success(t *testing.T) {
	st := &SubagentTool{
		RunSubagent: func(_ context.Context, task, model string) (string, error) {
			if task != "list files" {
				t.Errorf("task = %q, want %q", task, "list files")
			}
			if model != "" {
				t.Errorf("model = %q, want empty", model)
			}
			return "here are the files", nil
		},
	}

	result, err := st.Run(context.Background(), map[string]any{"task": "list files"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "here are the files" {
		t.Errorf("content = %q, want %q", result.Content, "here are the files")
	}
}

func TestSubagentTool_WithModel(t *testing.T) {
	var gotModel string
	st := &SubagentTool{
		RunSubagent: func(_ context.Context, _, model string) (string, error) {
			gotModel = model
			return "done", nil
		},
	}

	_, err := st.Run(context.Background(), map[string]any{
		"task":  "summarize",
		"model": "anthropic/claude-haiku-3",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotModel != "anthropic/claude-haiku-3" {
		t.Errorf("model = %q, want %q", gotModel, "anthropic/claude-haiku-3")
	}
}

func TestSubagentTool_RunnerError(t *testing.T) {
	st := &SubagentTool{
		RunSubagent: func(_ context.Context, _, _ string) (string, error) {
			return "", fmt.Errorf("subagent failed")
		},
	}

	_, err := st.Run(context.Background(), map[string]any{"task": "fail"})
	if err == nil {
		t.Fatal("expected error from runner")
	}
	if err.Error() != "subagent failed" {
		t.Errorf("error = %q, want %q", err.Error(), "subagent failed")
	}
}
