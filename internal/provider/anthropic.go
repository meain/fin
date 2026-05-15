package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	t "github.com/meain/fin/internal/types"
)

const (
	anthropicAPIVersion = "2023-06-01"
	anthropicMaxTokens  = 8192
)

type anthropicProvider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func newAnthropicProvider(apiKey, baseURL string, httpClient *http.Client) Provider {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &anthropicProvider{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

// --- Anthropic wire types ---

type anthRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	System    []anthSystemBlock `json:"system,omitempty"`
	Messages  []anthMessage    `json:"messages"`
	Tools     []anthTool       `json:"tools,omitempty"`
	Stream    bool             `json:"stream"`
}

type anthCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

type anthSystemBlock struct {
	Type         string            `json:"type"`
	Text         string            `json:"text"`
	CacheControl *anthCacheControl `json:"cache_control,omitempty"`
}

type anthMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []anthContentBlock
}

type anthContentBlock struct {
	Type      string           `json:"type"`
	Text      string           `json:"text,omitempty"`
	ID        string           `json:"id,omitempty"`
	Name      string           `json:"name,omitempty"`
	Input     any              `json:"input,omitempty"`
	ToolUseID string           `json:"tool_use_id,omitempty"`
	Content   any              `json:"content,omitempty"`
	IsError   bool             `json:"is_error,omitempty"`
	Source    *anthImageSource `json:"source,omitempty"`
}

type anthImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // e.g. "image/png"
	Data      string `json:"data"`       // base64-encoded
}

type anthTool struct {
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	InputSchema  any               `json:"input_schema"`
	CacheControl *anthCacheControl `json:"cache_control,omitempty"`
}

// SSE event types
type anthContentBlockStart struct {
	Index        int `json:"index"`
	ContentBlock struct {
		Type  string `json:"type"`
		ID    string `json:"id,omitempty"`
		Name  string `json:"name,omitempty"`
		Text  string `json:"text,omitempty"`
		Input any    `json:"input,omitempty"`
	} `json:"content_block"`
}

type anthContentBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
	} `json:"delta"`
}

type anthMessageStart struct {
	Message struct {
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type anthMessageDelta struct {
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthErrorEvent struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// --- Conversion: Message -> anthMessage ---

func messagesToAnthropic(msgs []t.Message) (system []anthSystemBlock, anthMsgs []anthMessage) {
	for _, m := range msgs {
		switch m.Role {
		case t.RoleSystem:
			system = append(system, anthSystemBlock{
				Type: "text",
				Text: m.Content,
			})

		case t.RoleUser:
			anthMsgs = append(anthMsgs, anthMessage{Role: "user", Content: m.Content})

		case t.RoleAssistant:
			if len(m.ToolCalls) > 0 {
				var blocks []anthContentBlock
				if m.Content != "" {
					blocks = append(blocks, anthContentBlock{Type: "text", Text: m.Content})
				}
				for _, tc := range m.ToolCalls {
					var input any
					if tc.Arguments != "" {
						_ = json.Unmarshal([]byte(tc.Arguments), &input)
					}
					if input == nil {
						input = map[string]any{}
					}
					blocks = append(blocks, anthContentBlock{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Name,
						Input: input,
					})
				}
				anthMsgs = append(anthMsgs, anthMessage{Role: "assistant", Content: blocks})
			} else {
				anthMsgs = append(anthMsgs, anthMessage{Role: "assistant", Content: m.Content})
			}

		case t.RoleTool:
			var block anthContentBlock
			if len(m.Images) > 0 {
				// Tool result with images: use content array with image blocks
				var contentBlocks []anthContentBlock
				for _, img := range m.Images {
					contentBlocks = append(contentBlocks, anthContentBlock{
						Type: "image",
						Source: &anthImageSource{
							Type:      "base64",
							MediaType: img.MediaType,
							Data:      img.Data,
						},
					})
				}
				if m.Content != "" {
					contentBlocks = append(contentBlocks, anthContentBlock{
						Type: "text",
						Text: m.Content,
					})
				}
				block = anthContentBlock{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   contentBlocks,
					IsError:   strings.HasPrefix(m.Content, "Error: "),
				}
			} else {
				block = anthContentBlock{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   &m.Content,
					IsError:   strings.HasPrefix(m.Content, "Error: "),
				}
			}
			// Merge consecutive tool results into one user message
			if len(anthMsgs) > 0 {
				last := &anthMsgs[len(anthMsgs)-1]
				if last.Role == "user" {
					if blocks, ok := last.Content.([]anthContentBlock); ok {
						last.Content = append(blocks, block)
						continue
					}
				}
			}
			anthMsgs = append(anthMsgs, anthMessage{
				Role:    "user",
				Content: []anthContentBlock{block},
			})
		}
	}
	return
}

func toolDefsToAnthropic(tools []t.ToolDef) []anthTool {
	out := make([]anthTool, len(tools))
	for i, td := range tools {
		out[i] = anthTool{
			Name:        td.Name,
			Description: td.Description,
			InputSchema: td.Parameters,
		}
	}
	return out
}

// --- StreamCompletion ---

func (p *anthropicProvider) StreamCompletion(ctx context.Context, req t.CompletionRequest) (Stream, error) {
	system, anthMsgs := messagesToAnthropic(req.Messages)
	anthTools := toolDefsToAnthropic(req.Tools)

	// Mark the last system block and last tool for prompt caching
	ephemeral := &anthCacheControl{Type: "ephemeral"}
	if len(anthTools) > 0 {
		anthTools[len(anthTools)-1].CacheControl = ephemeral
	} else if len(system) > 0 {
		system[len(system)-1].CacheControl = ephemeral
	}

	body := anthRequest{
		Model:     req.Model,
		MaxTokens: anthropicMaxTokens,
		System:    system,
		Messages:  anthMsgs,
		Tools:     anthTools,
		Stream:    true,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	url := strings.TrimSuffix(p.baseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, &APIError{Provider: "anthropic", StatusCode: resp.StatusCode, Body: b}
	}

	return &anthropicStream{
		reader: bufio.NewReader(resp.Body),
		body:   resp.Body,
	}, nil
}

// --- Stream ---

type anthropicStream struct {
	reader     *bufio.Reader
	body       io.ReadCloser
	done       bool
	toolCallID string // current tool call being accumulated
}

func (s *anthropicStream) Close() {
	if s.body != nil {
		s.body.Close()
	}
}

func (s *anthropicStream) Recv() (t.StreamDelta, error) {
	for {
		if s.done {
			return t.StreamDelta{}, io.EOF
		}

		eventType, data, err := readSSEEvent(s.reader)
		if err != nil {
			if err == io.EOF {
				s.done = true
			}
			return t.StreamDelta{}, err
		}
		if eventType == "" || data == "" {
			continue
		}

		delta, emit, err := s.handleEvent(eventType, data)
		if err != nil {
			return t.StreamDelta{}, err
		}
		if emit {
			return delta, nil
		}
	}
}

// handleEvent dispatches one SSE event to the matching handler. emit=false
// means the event was consumed but produced no observable delta (advance
// the loop). err is fatal and stops the stream.
func (s *anthropicStream) handleEvent(eventType, data string) (t.StreamDelta, bool, error) {
	switch eventType {
	case "content_block_start":
		return s.handleContentBlockStart(data)
	case "content_block_delta":
		return s.handleContentBlockDelta(data)
	case "message_start":
		return handleAnthropicMessageStart(data)
	case "message_delta":
		return handleAnthropicMessageDelta(data)
	case "content_block_stop":
		s.toolCallID = ""
		return t.StreamDelta{}, false, nil
	case "message_stop":
		s.done = true
		return t.StreamDelta{}, false, io.EOF
	case "error":
		return t.StreamDelta{}, false, handleAnthropicErrorEvent(data)
	}
	return t.StreamDelta{}, false, nil
}

func (s *anthropicStream) handleContentBlockStart(data string) (t.StreamDelta, bool, error) {
	ev, ok := decodeOrSkip[anthContentBlockStart](data)
	if !ok || ev.ContentBlock.Type != "tool_use" {
		return t.StreamDelta{}, false, nil
	}
	s.toolCallID = ev.ContentBlock.ID
	return t.StreamDelta{
		ToolCalls: []t.ToolCallDelta{{
			Index: ev.Index,
			ID:    ev.ContentBlock.ID,
			Name:  ev.ContentBlock.Name,
		}},
	}, true, nil
}

func (s *anthropicStream) handleContentBlockDelta(data string) (t.StreamDelta, bool, error) {
	ev, ok := decodeOrSkip[anthContentBlockDelta](data)
	if !ok {
		return t.StreamDelta{}, false, nil
	}
	switch ev.Delta.Type {
	case "text_delta":
		return t.StreamDelta{Content: ev.Delta.Text}, true, nil
	case "input_json_delta":
		return t.StreamDelta{
			ToolCalls: []t.ToolCallDelta{{
				Index:     ev.Index,
				Arguments: ev.Delta.PartialJSON,
			}},
		}, true, nil
	}
	return t.StreamDelta{}, false, nil
}

func handleAnthropicMessageStart(data string) (t.StreamDelta, bool, error) {
	ev, ok := decodeOrSkip[anthMessageStart](data)
	if !ok {
		return t.StreamDelta{}, false, nil
	}
	u := ev.Message.Usage
	return t.StreamDelta{
		Usage: &t.Usage{
			InputTokens:              u.InputTokens,
			CacheCreationInputTokens: u.CacheCreationInputTokens,
			CacheReadInputTokens:     u.CacheReadInputTokens,
		},
	}, true, nil
}

func handleAnthropicMessageDelta(data string) (t.StreamDelta, bool, error) {
	ev, ok := decodeOrSkip[anthMessageDelta](data)
	if !ok || ev.Usage.OutputTokens == 0 {
		return t.StreamDelta{}, false, nil
	}
	return t.StreamDelta{Usage: &t.Usage{OutputTokens: ev.Usage.OutputTokens}}, true, nil
}

func handleAnthropicErrorEvent(data string) error {
	ev, ok := decodeOrSkip[anthErrorEvent](data)
	if !ok {
		return fmt.Errorf("anthropic stream error: %s", data)
	}
	return fmt.Errorf("anthropic error: %s: %s", ev.Error.Type, ev.Error.Message)
}

// decodeOrSkip unmarshals data into a value of type T. Returns false when
// the payload is malformed — callers treat that as "skip this event".
func decodeOrSkip[T any](data string) (*T, bool) {
	var v T
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		return nil, false
	}
	return &v, true
}

// readSSEEvent reads a single SSE event (event: + data: lines).
func readSSEEvent(r *bufio.Reader) (eventType, data string, err error) {
	var dataLines []string

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF && line != "" {
				// process last line
			} else if err == io.EOF {
				if eventType != "" || len(dataLines) > 0 {
					return eventType, strings.Join(dataLines, "\n"), nil
				}
				return "", "", io.EOF
			} else {
				return "", "", err
			}
		}

		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if eventType != "" || len(dataLines) > 0 {
				return eventType, strings.Join(dataLines, "\n"), nil
			}
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}
}
