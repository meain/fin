package provider

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"

	tp "github.com/meain/fin/internal/types"
)

// newTestOpenAIStream builds an openaiStream over a static SSE-style body
// (no real HTTP body/Close needed for Recv-only tests).
func newTestOpenAIStream(body string) *openaiStream {
	return &openaiStream{reader: bufio.NewReader(strings.NewReader(body))}
}

// TestOpenAIStream_ParallelToolCallIndex guards a bug where the wire
// "index" field on each streamed tool-call delta chunk (used to multiplex
// fragments across multiple simultaneous tool calls) was silently dropped,
// so every delta reported Index 0 and consumeStream collapsed all parallel
// tool calls in a turn into one corrupted bucket.
func TestOpenAIStream_ParallelToolCallIndex(t *testing.T) {
	body := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read","arguments":"{\"path\":\"a\"}"}}]}}]}
` +
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_2","function":{"name":"read","arguments":"{\"path\":\"b\"}"}}]}}]}
` +
		`data: [DONE]` + "\n"

	s := newTestOpenAIStream(body)

	var deltas []tp.ToolCallDelta
	for {
		d, err := s.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		deltas = append(deltas, d.ToolCalls...)
	}

	if len(deltas) != 2 {
		t.Fatalf("expected 2 tool call deltas, got %d: %+v", len(deltas), deltas)
	}
	if deltas[0].Index != 0 || deltas[0].ID != "call_1" {
		t.Fatalf("expected first delta Index=0 ID=call_1, got %+v", deltas[0])
	}
	if deltas[1].Index != 1 || deltas[1].ID != "call_2" {
		t.Fatalf("expected second delta Index=1 ID=call_2, got %+v", deltas[1])
	}
}

// TestOpenAIStream_TrailingLineWithoutNewline guards against dropping the
// final SSE chunk when the connection closes immediately after it without a
// trailing newline (io.EOF returned together with a non-empty line).
func TestOpenAIStream_TrailingLineWithoutNewline(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"hi"}}]}` // no trailing \n
	s := newTestOpenAIStream(body)

	d, err := s.Recv()
	if err != nil {
		t.Fatalf("expected the final unterminated line to be processed, got err=%v", err)
	}
	if d.Content != "hi" {
		t.Fatalf("expected content %q, got %q", "hi", d.Content)
	}

	_, err = s.Recv()
	if err != io.EOF {
		t.Fatalf("expected io.EOF after the trailing line is consumed, got %v", err)
	}
}

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

func TestMessagesToOpenAI_ToolResultWithImages(t *testing.T) {
	msgs := []tp.Message{
		{
			Role:       tp.RoleTool,
			Content:    "Image content of file.png",
			ToolCallID: "call_1",
			Images: []tp.Image{
				{MediaType: "image/png", Data: "iVBORw0KGgo="},
			},
		},
	}
	oaiMsgs := messagesToOpenAI(msgs)
	// Tool result + injected user message with images
	if len(oaiMsgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(oaiMsgs))
	}

	// First: tool message with text only
	if oaiMsgs[0].Role != "tool" {
		t.Fatalf("expected role 'tool', got %q", oaiMsgs[0].Role)
	}
	if oaiMsgs[0].Content != "Image content of file.png" {
		t.Fatalf("expected string content, got %v", oaiMsgs[0].Content)
	}

	// Second: user message with image parts
	if oaiMsgs[1].Role != "user" {
		t.Fatalf("expected role 'user', got %q", oaiMsgs[1].Role)
	}
	parts, ok := oaiMsgs[1].Content.([]oaiContentPart)
	if !ok {
		t.Fatalf("expected content parts array, got %T", oaiMsgs[1].Content)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(parts))
	}
	if parts[0].Type != "text" {
		t.Fatalf("expected text part, got %q", parts[0].Type)
	}
	if parts[1].Type != "image_url" || parts[1].ImageURL == nil {
		t.Fatalf("expected image_url part, got %+v", parts[1])
	}
	if parts[1].ImageURL.URL != "data:image/png;base64,iVBORw0KGgo=" {
		t.Fatalf("unexpected image URL: %q", parts[1].ImageURL.URL)
	}
}

func TestMessagesToOpenAI_UserMessageWithImages(t *testing.T) {
	msgs := []tp.Message{
		{
			Role:    tp.RoleUser,
			Content: "What is in this image?",
			Images: []tp.Image{
				{MediaType: "image/jpeg", Data: "/9j/4AAQ="},
			},
		},
	}
	oaiMsgs := messagesToOpenAI(msgs)
	if len(oaiMsgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(oaiMsgs))
	}
	parts, ok := oaiMsgs[0].Content.([]oaiContentPart)
	if !ok {
		t.Fatalf("expected content parts array, got %T", oaiMsgs[0].Content)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts (text + image), got %d", len(parts))
	}
	if parts[0].Type != "text" || parts[0].Text != "What is in this image?" {
		t.Fatalf("unexpected text part: %+v", parts[0])
	}
	if parts[1].Type != "image_url" || parts[1].ImageURL.URL != "data:image/jpeg;base64,/9j/4AAQ=" {
		t.Fatalf("unexpected image part: %+v", parts[1])
	}
}

func TestMessagesToOpenAI_ToolResultWithImagesJSON(t *testing.T) {
	msgs := []tp.Message{
		{
			Role:       tp.RoleTool,
			Content:    "screenshot captured",
			ToolCallID: "call_img",
			Images: []tp.Image{
				{MediaType: "image/png", Data: "AAAA"},
			},
		},
	}
	req := oaiRequest{
		Model:    "gpt-4o",
		Messages: messagesToOpenAI(msgs),
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
	messages := decoded["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages in JSON, got %d", len(messages))
	}
	// Verify user message has content array with image_url
	userMsg := messages[1].(map[string]any)
	contentArr := userMsg["content"].([]any)
	imgPart := contentArr[1].(map[string]any)
	if imgPart["type"] != "image_url" {
		t.Fatalf("expected image_url type, got %v", imgPart["type"])
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
