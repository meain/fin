package provider

import (
	"encoding/json"
	"testing"

	tp "github.com/meain/fin/internal/types"
)

func TestMessagesToOpenAI_Basic(t *testing.T) {
	msgs := []tp.Message{
		{Role: tp.RoleSystem, Content: "be concise"},
		{Role: tp.RoleUser, Content: "hello"},
		{Role: tp.RoleAssistant, Content: "hi there"},
	}
	oaiMsgs := messagesToOpenAI(msgs)
	if len(oaiMsgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(oaiMsgs))
	}

	if oaiMsgs[0].Role != "system" || oaiMsgs[0].Content != "be concise" {
		t.Fatalf("unexpected system message: %+v", oaiMsgs[0])
	}
	if oaiMsgs[1].Role != "user" || oaiMsgs[1].Content != "hello" {
		t.Fatalf("unexpected user message: %+v", oaiMsgs[1])
	}
	if oaiMsgs[2].Role != "assistant" || oaiMsgs[2].Content != "hi there" {
		t.Fatalf("unexpected assistant message: %+v", oaiMsgs[2])
	}
}

func TestMessagesToOpenAI_ToolCalls(t *testing.T) {
	msgs := []tp.Message{
		{
			Role: tp.RoleAssistant,
			ToolCalls: []tp.ToolCall{
				{ID: "call_1", Name: "search", Arguments: `{"q":"foo"}`},
				{ID: "call_2", Name: "read", Arguments: `{"path":"/tmp"}`},
			},
		},
	}
	oaiMsgs := messagesToOpenAI(msgs)
	if len(oaiMsgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(oaiMsgs))
	}
	if len(oaiMsgs[0].ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(oaiMsgs[0].ToolCalls))
	}
	tc := oaiMsgs[0].ToolCalls[0]
	if tc.ID != "call_1" {
		t.Fatalf("expected ID 'call_1', got %q", tc.ID)
	}
	if tc.Type != "function" {
		t.Fatalf("expected type 'function', got %q", tc.Type)
	}
	if tc.Function.Name != "search" {
		t.Fatalf("expected name 'search', got %q", tc.Function.Name)
	}
	if tc.Function.Arguments != `{"q":"foo"}` {
		t.Fatalf("expected arguments, got %q", tc.Function.Arguments)
	}
}

func TestMessagesToOpenAI_ToolResult(t *testing.T) {
	msgs := []tp.Message{
		{Role: tp.RoleTool, Content: "result data", ToolCallID: "call_1"},
	}
	oaiMsgs := messagesToOpenAI(msgs)
	if len(oaiMsgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(oaiMsgs))
	}
	if oaiMsgs[0].Role != "tool" {
		t.Fatalf("expected role 'tool', got %q", oaiMsgs[0].Role)
	}
	if oaiMsgs[0].Content != "result data" {
		t.Fatalf("expected content 'result data', got %q", oaiMsgs[0].Content)
	}
	if oaiMsgs[0].ToolCallID != "call_1" {
		t.Fatalf("expected tool_call_id 'call_1', got %q", oaiMsgs[0].ToolCallID)
	}
}

func TestMessagesToOpenAI_Empty(t *testing.T) {
	oaiMsgs := messagesToOpenAI(nil)
	if len(oaiMsgs) != 0 {
		t.Fatalf("expected 0 messages for nil input, got %d", len(oaiMsgs))
	}
}

func TestToolDefsToOpenAI(t *testing.T) {
	tools := []tp.ToolDef{
		{
			Name:        "search",
			Description: "Search for things",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
				"required": []any{"query"},
			},
		},
	}
	oaiTools := toolDefsToOpenAI(tools)
	if len(oaiTools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(oaiTools))
	}
	if oaiTools[0].Type != "function" {
		t.Fatalf("expected type 'function', got %q", oaiTools[0].Type)
	}
	if oaiTools[0].Function.Name != "search" {
		t.Fatalf("expected name 'search', got %q", oaiTools[0].Function.Name)
	}
	if oaiTools[0].Function.Description != "Search for things" {
		t.Fatalf("expected description, got %q", oaiTools[0].Function.Description)
	}
	if oaiTools[0].Function.Parameters == nil {
		t.Fatal("expected non-nil parameters")
	}
}

func TestToolDefsToOpenAI_Empty(t *testing.T) {
	oaiTools := toolDefsToOpenAI(nil)
	if len(oaiTools) != 0 {
		t.Fatalf("expected 0 tools for nil input, got %d", len(oaiTools))
	}
}

func TestToolDefsToOpenAI_MultipleTools(t *testing.T) {
	tools := []tp.ToolDef{
		{Name: "a", Description: "tool a", Parameters: map[string]any{"type": "object"}},
		{Name: "b", Description: "tool b", Parameters: map[string]any{"type": "object"}},
		{Name: "c", Description: "tool c", Parameters: map[string]any{"type": "object"}},
	}
	oaiTools := toolDefsToOpenAI(tools)
	if len(oaiTools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(oaiTools))
	}
	for i, name := range []string{"a", "b", "c"} {
		if oaiTools[i].Function.Name != name {
			t.Fatalf("expected tool %d name %q, got %q", i, name, oaiTools[i].Function.Name)
		}
	}
}

// --- JSON round-trip verification ---

func TestOpenAIRequestJSON(t *testing.T) {
	msgs := []tp.Message{
		{Role: tp.RoleUser, Content: "hello"},
	}
	tools := []tp.ToolDef{
		{Name: "test", Description: "a test tool", Parameters: map[string]any{"type": "object"}},
	}

	req := oaiRequest{
		Model:    "gpt-4",
		Messages: messagesToOpenAI(msgs),
		Tools:    toolDefsToOpenAI(tools),
		Stream:   true,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded["model"] != "gpt-4" {
		t.Fatalf("expected model 'gpt-4', got %v", decoded["model"])
	}
	if decoded["stream"] != true {
		t.Fatalf("expected stream true, got %v", decoded["stream"])
	}
	messages, ok := decoded["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("expected 1 message in JSON, got %v", decoded["messages"])
	}
	toolsArr, ok := decoded["tools"].([]any)
	if !ok || len(toolsArr) != 1 {
		t.Fatalf("expected 1 tool in JSON, got %v", decoded["tools"])
	}
}
