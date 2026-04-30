package tool

import (
	"context"
	"fmt"
	"testing"

	t "github.com/meain/fin/internal/types"
)

func TestSubagentTool_Name(tt *testing.T) {
	st := &SubagentTool{}
	if st.Name() != "subagent" {
		tt.Errorf("Name() = %q, want %q", st.Name(), "subagent")
	}
}

func TestSubagentTool_Parameters(tt *testing.T) {
	st := &SubagentTool{}
	params := st.Parameters()

	props, ok := params["properties"].(map[string]any)
	if !ok {
		tt.Fatal("parameters missing properties")
	}
	if _, ok := props["task"]; !ok {
		tt.Error("parameters missing task property")
	}
	if _, ok := props["model"]; !ok {
		tt.Error("parameters missing model property")
	}

	required, ok := params["required"].([]string)
	if !ok {
		tt.Fatal("parameters missing required")
	}
	if len(required) != 1 || required[0] != "task" {
		tt.Errorf("required = %v, want [task]", required)
	}
}

func TestSubagentTool_EmptyTask(tt *testing.T) {
	st := &SubagentTool{
		RunSubagent: func(_ context.Context, _, _ string) (t.ToolResult, error) {
			tt.Fatal("should not be called")
			return t.ToolResult{}, nil
		},
	}

	_, err := st.Run(context.Background(), map[string]any{})
	if err == nil {
		tt.Fatal("expected error for empty task")
	}
}

func TestSubagentTool_NilRunner(tt *testing.T) {
	st := &SubagentTool{}

	_, err := st.Run(context.Background(), map[string]any{"task": "do something"})
	if err == nil {
		tt.Fatal("expected error for nil runner")
	}
}

func TestSubagentTool_Success(tt *testing.T) {
	st := &SubagentTool{
		RunSubagent: func(_ context.Context, task, model string) (t.ToolResult, error) {
			if task != "list files" {
				tt.Errorf("task = %q, want %q", task, "list files")
			}
			if model != "" {
				tt.Errorf("model = %q, want empty", model)
			}
			return t.ToolResult{Content: "here are the files"}, nil
		},
	}

	result, err := st.Run(context.Background(), map[string]any{"task": "list files"})
	if err != nil {
		tt.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "here are the files" {
		tt.Errorf("content = %q, want %q", result.Content, "here are the files")
	}
}

func TestSubagentTool_WithModel(tt *testing.T) {
	var gotModel string
	st := &SubagentTool{
		RunSubagent: func(_ context.Context, _, model string) (t.ToolResult, error) {
			gotModel = model
			return t.ToolResult{Content: "done"}, nil
		},
	}

	_, err := st.Run(context.Background(), map[string]any{
		"task":  "summarize",
		"model": "anthropic/claude-haiku-3",
	})
	if err != nil {
		tt.Fatalf("unexpected error: %v", err)
	}
	if gotModel != "anthropic/claude-haiku-3" {
		tt.Errorf("model = %q, want %q", gotModel, "anthropic/claude-haiku-3")
	}
}

func TestSubagentTool_RunnerError(tt *testing.T) {
	st := &SubagentTool{
		RunSubagent: func(_ context.Context, _, _ string) (t.ToolResult, error) {
			return t.ToolResult{}, fmt.Errorf("subagent failed")
		},
	}

	_, err := st.Run(context.Background(), map[string]any{"task": "fail"})
	if err == nil {
		tt.Fatal("expected error from runner")
	}
	if err.Error() != "subagent failed" {
		tt.Errorf("error = %q, want %q", err.Error(), "subagent failed")
	}
}

func TestSubagentTool_SubMessagesPassedThrough(tt *testing.T) {
	msgs := []t.Message{
		{Role: t.RoleSystem, Content: "system prompt"},
		{Role: t.RoleUser, Content: "do stuff"},
		{Role: t.RoleAssistant, Content: "done"},
	}
	st := &SubagentTool{
		RunSubagent: func(_ context.Context, _, _ string) (t.ToolResult, error) {
			return t.ToolResult{Content: "done", SubMessages: msgs}, nil
		},
	}

	result, err := st.Run(context.Background(), map[string]any{"task": "work"})
	if err != nil {
		tt.Fatalf("unexpected error: %v", err)
	}
	if len(result.SubMessages) != 3 {
		tt.Errorf("SubMessages count = %d, want 3", len(result.SubMessages))
	}
}
