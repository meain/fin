package provider

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"

	tp "github.com/meain/fin/internal/types"
)

func TestMessagesToAnthropic_UserMessage(t *testing.T) {
	msgs := []tp.Message{
		{Role: tp.RoleUser, Content: "hello"},
	}
	system, anthMsgs := messagesToAnthropic(msgs)
	if len(system) != 0 {
		t.Fatalf("expected empty system, got %v", system)
	}
	if len(anthMsgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(anthMsgs))
	}
	if anthMsgs[0].Role != "user" {
		t.Fatalf("expected role 'user', got %q", anthMsgs[0].Role)
	}
	if anthMsgs[0].Content != "hello" {
		t.Fatalf("expected content 'hello', got %v", anthMsgs[0].Content)
	}
}

func TestMessagesToAnthropic_SystemMessage(t *testing.T) {
	msgs := []tp.Message{
		{Role: tp.RoleSystem, Content: "you are helpful"},
		{Role: tp.RoleUser, Content: "hi"},
	}
	system, anthMsgs := messagesToAnthropic(msgs)
	if len(system) != 1 || system[0].Text != "you are helpful" {
		t.Fatalf("expected system 'you are helpful', got %v", system)
	}
	if len(anthMsgs) != 1 {
		t.Fatalf("expected 1 message (system extracted), got %d", len(anthMsgs))
	}
	if anthMsgs[0].Role != "user" {
		t.Fatalf("expected role 'user', got %q", anthMsgs[0].Role)
	}
}

func TestMessagesToAnthropic_AssistantPlain(t *testing.T) {
	msgs := []tp.Message{
		{Role: tp.RoleAssistant, Content: "I can help"},
	}
	_, anthMsgs := messagesToAnthropic(msgs)
	if len(anthMsgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(anthMsgs))
	}
	if anthMsgs[0].Role != "assistant" {
		t.Fatalf("expected role 'assistant', got %q", anthMsgs[0].Role)
	}
	if anthMsgs[0].Content != "I can help" {
		t.Fatalf("expected content 'I can help', got %v", anthMsgs[0].Content)
	}
}

func TestMessagesToAnthropic_AssistantWithToolCalls(t *testing.T) {
	msgs := []tp.Message{
		{
			Role:    tp.RoleAssistant,
			Content: "Let me check",
			ToolCalls: []tp.ToolCall{
				{ID: "tc_1", Name: "search", Arguments: `{"query":"test"}`},
			},
		},
	}
	_, anthMsgs := messagesToAnthropic(msgs)
	if len(anthMsgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(anthMsgs))
	}
	blocks, ok := anthMsgs[0].Content.([]anthContentBlock)
	if !ok {
		t.Fatalf("expected content to be []anthContentBlock, got %T", anthMsgs[0].Content)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks (text + tool_use), got %d", len(blocks))
	}
	if blocks[0].Type != "text" || blocks[0].Text != "Let me check" {
		t.Fatalf("expected text block, got %+v", blocks[0])
	}
	if blocks[1].Type != "tool_use" || blocks[1].ID != "tc_1" || blocks[1].Name != "search" {
		t.Fatalf("expected tool_use block, got %+v", blocks[1])
	}
	// Verify input was parsed from JSON
	inputMap, ok := blocks[1].Input.(map[string]any)
	if !ok {
		t.Fatalf("expected input to be map, got %T", blocks[1].Input)
	}
	if inputMap["query"] != "test" {
		t.Fatalf("expected input query 'test', got %v", inputMap["query"])
	}
}

func TestMessagesToAnthropic_AssistantToolCallEmptyArgs(t *testing.T) {
	msgs := []tp.Message{
		{
			Role: tp.RoleAssistant,
			ToolCalls: []tp.ToolCall{
				{ID: "tc_2", Name: "list_files", Arguments: ""},
			},
		},
	}
	_, anthMsgs := messagesToAnthropic(msgs)
	blocks, ok := anthMsgs[0].Content.([]anthContentBlock)
	if !ok {
		t.Fatalf("expected []anthContentBlock, got %T", anthMsgs[0].Content)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block (no text, just tool_use), got %d", len(blocks))
	}
	// Empty arguments should result in empty map
	inputMap, ok := blocks[0].Input.(map[string]any)
	if !ok {
		t.Fatalf("expected input to be map[string]any, got %T", blocks[0].Input)
	}
	if len(inputMap) != 0 {
		t.Fatalf("expected empty input map, got %v", inputMap)
	}
}

func TestMessagesToAnthropic_ToolResult(t *testing.T) {
	msgs := []tp.Message{
		{Role: tp.RoleTool, Content: "file contents here", ToolCallID: "tc_1"},
	}
	_, anthMsgs := messagesToAnthropic(msgs)
	if len(anthMsgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(anthMsgs))
	}
	if anthMsgs[0].Role != "user" {
		t.Fatalf("expected role 'user' for tool result, got %q", anthMsgs[0].Role)
	}
	blocks, ok := anthMsgs[0].Content.([]anthContentBlock)
	if !ok {
		t.Fatalf("expected []anthContentBlock, got %T", anthMsgs[0].Content)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Type != "tool_result" {
		t.Fatalf("expected type 'tool_result', got %q", blocks[0].Type)
	}
	if blocks[0].ToolUseID != "tc_1" {
		t.Fatalf("expected tool_use_id 'tc_1', got %q", blocks[0].ToolUseID)
	}
	if blocks[0].IsError {
		t.Fatal("expected IsError false")
	}
}

func TestMessagesToAnthropic_ToolResultError(t *testing.T) {
	// Matches the actual format produced by agent.errorWithContext:
	// "Error (<context>): <err>" — not a bare "Error: " prefix.
	msgs := []tp.Message{
		{Role: tp.RoleTool, Content: "Error (shell $ foo): something went wrong", ToolCallID: "tc_1"},
	}
	_, anthMsgs := messagesToAnthropic(msgs)
	blocks := anthMsgs[0].Content.([]anthContentBlock)
	if !blocks[0].IsError {
		t.Fatal("expected IsError true for error content")
	}
}

func TestMessagesToAnthropic_ConsecutiveToolResults(t *testing.T) {
	msgs := []tp.Message{
		{Role: tp.RoleTool, Content: "result1", ToolCallID: "tc_1"},
		{Role: tp.RoleTool, Content: "result2", ToolCallID: "tc_2"},
	}
	_, anthMsgs := messagesToAnthropic(msgs)
	if len(anthMsgs) != 1 {
		t.Fatalf("expected 1 merged user message, got %d", len(anthMsgs))
	}
	blocks, ok := anthMsgs[0].Content.([]anthContentBlock)
	if !ok {
		t.Fatalf("expected []anthContentBlock, got %T", anthMsgs[0].Content)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 tool_result blocks, got %d", len(blocks))
	}
	if blocks[0].ToolUseID != "tc_1" || blocks[1].ToolUseID != "tc_2" {
		t.Fatalf("unexpected tool_use_ids: %q, %q", blocks[0].ToolUseID, blocks[1].ToolUseID)
	}
}

func TestMessagesToAnthropic_ToolResultWithImages(t *testing.T) {
	msgs := []tp.Message{
		{
			Role:       tp.RoleTool,
			Content:    "screenshot captured",
			ToolCallID: "tc_1",
			Images: []tp.Image{
				{MediaType: "image/png", Data: "iVBORw0KGgo="},
			},
		},
	}
	_, anthMsgs := messagesToAnthropic(msgs)
	blocks := anthMsgs[0].Content.([]anthContentBlock)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 tool_result block, got %d", len(blocks))
	}
	contentBlocks, ok := blocks[0].Content.([]anthContentBlock)
	if !ok {
		t.Fatalf("expected content to be []anthContentBlock, got %T", blocks[0].Content)
	}
	if len(contentBlocks) != 2 {
		t.Fatalf("expected 2 inner blocks (image + text), got %d", len(contentBlocks))
	}
	if contentBlocks[0].Type != "image" {
		t.Fatalf("expected first inner block to be image, got %q", contentBlocks[0].Type)
	}
	if contentBlocks[0].Source == nil {
		t.Fatal("expected non-nil image source")
	}
	if contentBlocks[0].Source.MediaType != "image/png" {
		t.Fatalf("expected media type 'image/png', got %q", contentBlocks[0].Source.MediaType)
	}
	if contentBlocks[0].Source.Data != "iVBORw0KGgo=" {
		t.Fatalf("expected base64 data, got %q", contentBlocks[0].Source.Data)
	}
	if contentBlocks[1].Type != "text" || contentBlocks[1].Text != "screenshot captured" {
		t.Fatalf("expected text block with tool content, got %+v", contentBlocks[1])
	}
}

func TestMessagesToAnthropic_FullConversation(t *testing.T) {
	msgs := []tp.Message{
		{Role: tp.RoleSystem, Content: "be concise"},
		{Role: tp.RoleUser, Content: "search for foo"},
		{
			Role: tp.RoleAssistant,
			ToolCalls: []tp.ToolCall{
				{ID: "tc_1", Name: "search", Arguments: `{"q":"foo"}`},
			},
		},
		{Role: tp.RoleTool, Content: "found foo", ToolCallID: "tc_1"},
		{Role: tp.RoleAssistant, Content: "I found foo for you"},
	}
	system, anthMsgs := messagesToAnthropic(msgs)
	if len(system) != 1 || system[0].Text != "be concise" {
		t.Fatalf("expected system 'be concise', got %v", system)
	}
	if len(anthMsgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(anthMsgs))
	}
	if anthMsgs[0].Role != "user" {
		t.Fatalf("expected first msg role 'user', got %q", anthMsgs[0].Role)
	}
	if anthMsgs[1].Role != "assistant" {
		t.Fatalf("expected second msg role 'assistant', got %q", anthMsgs[1].Role)
	}
	if anthMsgs[2].Role != "user" {
		t.Fatalf("expected third msg role 'user' (tool result), got %q", anthMsgs[2].Role)
	}
	if anthMsgs[3].Role != "assistant" {
		t.Fatalf("expected fourth msg role 'assistant', got %q", anthMsgs[3].Role)
	}
}

func TestToolDefsToAnthropic(t *testing.T) {
	tools := []tp.ToolDef{
		{
			Name:        "search",
			Description: "Search for files",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
				"required": []any{"query"},
			},
		},
		{
			Name:        "read",
			Description: "Read a file",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
			},
		},
	}
	anthTools := toolDefsToAnthropic(tools)
	if len(anthTools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(anthTools))
	}
	if anthTools[0].Name != "search" {
		t.Fatalf("expected name 'search', got %q", anthTools[0].Name)
	}
	if anthTools[0].Description != "Search for files" {
		t.Fatalf("expected description 'Search for files', got %q", anthTools[0].Description)
	}
	if anthTools[0].InputSchema == nil {
		t.Fatal("expected non-nil InputSchema")
	}
	if anthTools[1].Name != "read" {
		t.Fatalf("expected name 'read', got %q", anthTools[1].Name)
	}
}

func TestToolDefsToAnthropic_Empty(t *testing.T) {
	anthTools := toolDefsToAnthropic(nil)
	if len(anthTools) != 0 {
		t.Fatalf("expected 0 tools for nil input, got %d", len(anthTools))
	}
}

// --- SSE parsing tests ---

func TestReadSSEEvent_Basic(t *testing.T) {
	input := "event: content_block_delta\ndata: {\"type\":\"delta\"}\n\n"
	r := bufio.NewReader(strings.NewReader(input))

	eventType, data, err := readSSEEvent(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventType != "content_block_delta" {
		t.Fatalf("expected event type 'content_block_delta', got %q", eventType)
	}
	if data != `{"type":"delta"}` {
		t.Fatalf("expected data, got %q", data)
	}
}

func TestReadSSEEvent_MultipleEvents(t *testing.T) {
	input := "event: message_start\ndata: {\"id\":\"1\"}\n\nevent: content_block_start\ndata: {\"index\":0}\n\n"
	r := bufio.NewReader(strings.NewReader(input))

	// First event
	eventType, data, err := readSSEEvent(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventType != "message_start" {
		t.Fatalf("expected 'message_start', got %q", eventType)
	}
	if data != `{"id":"1"}` {
		t.Fatalf("expected data, got %q", data)
	}

	// Second event
	eventType, data, err = readSSEEvent(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventType != "content_block_start" {
		t.Fatalf("expected 'content_block_start', got %q", eventType)
	}
	if data != `{"index":0}` {
		t.Fatalf("expected data, got %q", data)
	}
}

func TestReadSSEEvent_EmptyLinesSkipped(t *testing.T) {
	input := "\n\n\nevent: ping\ndata: {}\n\n"
	r := bufio.NewReader(strings.NewReader(input))

	eventType, data, err := readSSEEvent(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventType != "ping" {
		t.Fatalf("expected 'ping', got %q", eventType)
	}
	if data != "{}" {
		t.Fatalf("expected '{}', got %q", data)
	}
}

func TestReadSSEEvent_EOFAfterEvent(t *testing.T) {
	// No trailing newline - event terminated by EOF
	input := "event: message_stop\ndata: {}"
	r := bufio.NewReader(strings.NewReader(input))

	eventType, data, err := readSSEEvent(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventType != "message_stop" {
		t.Fatalf("expected 'message_stop', got %q", eventType)
	}
	if data != "{}" {
		t.Fatalf("expected '{}', got %q", data)
	}
}

func TestReadSSEEvent_PureEOF(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(""))

	_, _, err := readSSEEvent(r)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestReadSSEEvent_MultipleDataLines(t *testing.T) {
	input := "event: content_block_delta\ndata: line1\ndata: line2\n\n"
	r := bufio.NewReader(strings.NewReader(input))

	eventType, data, err := readSSEEvent(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventType != "content_block_delta" {
		t.Fatalf("expected 'content_block_delta', got %q", eventType)
	}
	if data != "line1\nline2" {
		t.Fatalf("expected 'line1\\nline2', got %q", data)
	}
}

func TestReadSSEEvent_CarriageReturn(t *testing.T) {
	input := "event: ping\r\ndata: {}\r\n\r\n"
	r := bufio.NewReader(strings.NewReader(input))

	eventType, data, err := readSSEEvent(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventType != "ping" {
		t.Fatalf("expected 'ping', got %q", eventType)
	}
	if data != "{}" {
		t.Fatalf("expected '{}', got %q", data)
	}
}

// --- JSON round-trip verification ---

func TestAnthropicRequestJSON(t *testing.T) {
	msgs := []tp.Message{
		{Role: tp.RoleSystem, Content: "be helpful"},
		{Role: tp.RoleUser, Content: "hello"},
	}
	system, anthMsgs := messagesToAnthropic(msgs)

	req := anthRequest{
		Model:     "claude-3-opus-20240229",
		MaxTokens: anthropicMaxTokens,
		System:    system,
		Messages:  anthMsgs,
		Stream:    true,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	sysArr, ok := decoded["system"].([]any)
	if !ok || len(sysArr) != 1 {
		t.Fatalf("expected system array with 1 block, got %v", decoded["system"])
	}
	sysBlock, ok := sysArr[0].(map[string]any)
	if !ok || sysBlock["text"] != "be helpful" {
		t.Fatalf("expected system text 'be helpful', got %v", sysArr[0])
	}
	if decoded["model"] != "claude-3-opus-20240229" {
		t.Fatalf("expected model, got %v", decoded["model"])
	}
	messages, ok := decoded["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("expected 1 message in JSON, got %v", decoded["messages"])
	}
}

func TestPromptCaching_ToolsGetCacheControl(t *testing.T) {
	system := []anthSystemBlock{
		{Type: "text", Text: "be helpful"},
	}
	tools := []anthTool{
		{Name: "read", Description: "Read a file", InputSchema: map[string]any{"type": "object"}},
		{Name: "write", Description: "Write a file", InputSchema: map[string]any{"type": "object"}},
	}

	// Simulate what StreamCompletion does
	ephemeral := &anthCacheControl{Type: "ephemeral"}
	if len(tools) > 0 {
		tools[len(tools)-1].CacheControl = ephemeral
	} else if len(system) > 0 {
		system[len(system)-1].CacheControl = ephemeral
	}

	// Last tool should have cache_control
	if tools[1].CacheControl == nil || tools[1].CacheControl.Type != "ephemeral" {
		t.Fatal("expected cache_control on last tool")
	}
	// First tool should not
	if tools[0].CacheControl != nil {
		t.Fatal("expected no cache_control on first tool")
	}
	// System should not (tools take priority)
	if system[0].CacheControl != nil {
		t.Fatal("expected no cache_control on system when tools present")
	}

	// Verify JSON serialization
	data, err := json.Marshal(tools[1])
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	cc, ok := decoded["cache_control"].(map[string]any)
	if !ok || cc["type"] != "ephemeral" {
		t.Fatalf("expected cache_control.type 'ephemeral' in JSON, got %v", decoded["cache_control"])
	}
}

func TestPromptCaching_SystemGetsCacheControlWhenNoTools(t *testing.T) {
	system := []anthSystemBlock{
		{Type: "text", Text: "be helpful"},
	}
	var tools []anthTool

	ephemeral := &anthCacheControl{Type: "ephemeral"}
	if len(tools) > 0 {
		tools[len(tools)-1].CacheControl = ephemeral
	} else if len(system) > 0 {
		system[len(system)-1].CacheControl = ephemeral
	}

	if system[0].CacheControl == nil || system[0].CacheControl.Type != "ephemeral" {
		t.Fatal("expected cache_control on system block when no tools")
	}
}
