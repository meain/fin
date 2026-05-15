package agent

import (
	"context"
	"fmt"

	"github.com/meain/fin/internal/prompt"
	"github.com/meain/fin/internal/tool"
	t "github.com/meain/fin/internal/types"
)

// runSubagent spawns an isolated child agent for a task. The child uses
// the same tools (minus subagent) and config, but a fresh conversation
// and a no-op UI. The full child conversation is returned as
// SubMessages so the parent's exporter can render it inline.
func (a *Agent) runSubagent(ctx context.Context, task, model string) (t.ToolResult, error) {
	childTools := BuildTools(a.skills, a.enabled, false)

	systemPrompt := prompt.BuildSystem(a.config, a.skills, a.sessionID, a.enabled)

	p := a.provider
	childModel := a.model
	if model != "" {
		wrapped, fullName, err := newProviderForModel(a.config, model)
		if err != nil {
			return t.ToolResult{}, fmt.Errorf("failed to create provider for subagent: %w", err)
		}
		p = wrapped
		childModel = fullName
	}

	// Wire subagent tool runner on the child's tool set.
	for _, tl := range childTools {
		if st, ok := tl.(*tool.SubagentTool); ok {
			st.RunSubagent = a.runSubagent
			break
		}
	}

	child := &Agent{
		provider: p,
		model:    childModel,
		tools:    childTools,
		config:   a.config,
		approval: a.approval.ForSubagent(),
		ui:       nullUI{},
		messages: []t.Message{
			{Role: t.RoleSystem, Content: systemPrompt},
		},
	}

	if err := child.AddUserMessage(ctx, task); err != nil {
		return t.ToolResult{}, err
	}

	// Return the last assistant message content + full conversation.
	for i := len(child.messages) - 1; i >= 0; i-- {
		if child.messages[i].Role == t.RoleAssistant && child.messages[i].Content != "" {
			return t.ToolResult{
				Content:     child.messages[i].Content,
				SubMessages: child.messages,
			}, nil
		}
	}

	return t.ToolResult{}, fmt.Errorf("subagent produced no response")
}
