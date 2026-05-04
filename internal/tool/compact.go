package tool

import (
	"context"
	"fmt"

	t "github.com/meain/fin/internal/types"
)

// CompactTool lets the agent compact the conversation by summarizing it
// and starting a fresh session with the summary as context.
type CompactTool struct{}

func (c *CompactTool) Name() string { return "compact" }

func (c *CompactTool) Description() string {
	return "Compact the conversation into a new session. This discards all messages and replaces them with your summary as the sole context. The old session is preserved on disk and linked via previous_session in the new session. Use when the conversation is getting long and earlier exchanges are no longer relevant. The summary you provide is the ONLY context carried forward — anything not in it is lost. Be thorough: include key decisions, current state of the work, file paths and code references that matter, ongoing tasks, unresolved issues, and any constraints or preferences established during the conversation."
}

func (c *CompactTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": "Comprehensive summary to carry forward. Must include: key decisions made, current state of work, relevant file paths and code details, ongoing/incomplete tasks, unresolved issues, and any user preferences or constraints. This becomes the only context in the new session.",
			},
		},
		"required": []string{"summary"},
	}
}

func (c *CompactTool) Run(_ context.Context, args map[string]any) (t.ToolResult, error) {
	summary, _ := args["summary"].(string)
	if summary == "" {
		return t.ToolResult{}, fmt.Errorf("summary is required")
	}
	// The actual compaction is handled by the agent loop which detects this tool.
	return t.ToolResult{Content: "Conversation compacted."}, nil
}
