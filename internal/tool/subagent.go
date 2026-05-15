package tool

import (
	"context"
	"fmt"

	t "github.com/meain/fin/internal/types"
)

// SubagentTool delegates a task to an isolated subagent.
type SubagentTool struct {
	// RunSubagent is provided by the caller (main package) to avoid circular
	// imports. It runs a subagent with the given task and optional model
	// override, returning the result with the full conversation for export.
	RunSubagent func(ctx context.Context, task, model string) (t.ToolResult, error)
}

func (s *SubagentTool) Name() string { return "subagent" }

func (s *SubagentTool) Label(args map[string]any) ToolLabel {
	task, _ := args["task"].(string)
	if len(task) > 60 {
		task = task[:60] + "…"
	}
	return ToolLabel{Primary: task}
}

func (s *SubagentTool) Description() string {
	return "Delegate a task to an isolated subagent. The subagent gets its own conversation with the same tools but no shared history. Use this for focused subtasks that benefit from a clean context."
}

func (s *SubagentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task": map[string]any{
				"type":        "string",
				"description": "The task to delegate to the subagent. Be specific and self-contained — the subagent has no access to the parent conversation.",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Optional model override (e.g. 'anthropic/claude-haiku-3'). Defaults to the same model as the parent.",
			},
		},
		"required": []string{"task"},
	}
}

func (s *SubagentTool) Run(ctx context.Context, args map[string]any) (t.ToolResult, error) {
	task, _ := args["task"].(string)
	if task == "" {
		return t.ToolResult{}, fmt.Errorf("task is required")
	}

	model, _ := args["model"].(string)

	if s.RunSubagent == nil {
		return t.ToolResult{}, fmt.Errorf("subagent runner not configured")
	}

	return s.RunSubagent(ctx, task, model)
}
