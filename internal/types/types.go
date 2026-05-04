package types

import (
	"os"
	"path/filepath"
	"time"
)

// Role represents the sender of a message in a conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Image is a base64-encoded image attached to a message.
type Image struct {
	MediaType string `json:"media_type"` // e.g. "image/png"
	Data      string `json:"data"`       // base64-encoded
}

// Message is a provider-agnostic conversation message.
type Message struct {
	Role        Role       `json:"role"`
	Content     string     `json:"content,omitempty"`
	Images      []Image    `json:"images,omitempty"`
	ToolCalls   []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID  string     `json:"tool_call_id,omitempty"`
	Timestamp   time.Time  `json:"timestamp,omitempty"`
	SubMessages []Message  `json:"sub_messages,omitempty"` // subagent conversation
	Usage       *Usage     `json:"usage,omitempty"`
}

// ToolCall represents a tool invocation requested by the model.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // raw JSON string
}

// ToolCallDelta is a streaming fragment of a tool call.
type ToolCallDelta struct {
	Index     int
	ID        string // non-empty signals a new tool call
	Name      string
	Arguments string // partial JSON fragment
}

// Usage tracks token consumption for a single response.
type Usage struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
}

// StreamDelta is a single chunk from a streaming response.
type StreamDelta struct {
	Content   string
	ToolCalls []ToolCallDelta
	Usage     *Usage // non-nil when usage data is available
}

// CompletionRequest is the provider-agnostic request for a chat completion.
type CompletionRequest struct {
	Model    string
	Messages []Message
	Tools    []ToolDef
}

// ToolDef describes a tool for the LLM.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema object
}

// ToolResult is the return value from a tool execution.
type ToolResult struct {
	Content     string
	Images      []Image
	SubMessages []Message // subagent conversation (for export)
}

// ExpandHome expands ~/... paths to the user's home directory.
func ExpandHome(path string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
