package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"

	t "github.com/meain/fin/internal/types"
)

// Provider sends messages to an LLM and returns a streaming response.
type Provider interface {
	StreamCompletion(ctx context.Context, req t.CompletionRequest) (Stream, error)
}

// Stream yields deltas from a streaming completion.
type Stream interface {
	Recv() (t.StreamDelta, error)
	Close()
}

// Config holds provider connection settings.
type Config struct {
	BaseURL   string
	APIKeyEnv string
	Headers   map[string]string
}

// New creates the appropriate provider for the given provider name.
func New(name string, cfg Config) (Provider, error) {
	apiKey := os.Getenv(cfg.APIKeyEnv)
	if cfg.APIKeyEnv != "" && apiKey == "" {
		return nil, fmt.Errorf("env var %s not set", cfg.APIKeyEnv)
	}

	if usesOpenRouter(name, cfg.BaseURL) {
		cfg.Headers = withOpenRouterAttribution(cfg.Headers)
	}

	var httpClient *http.Client
	if len(cfg.Headers) > 0 {
		httpClient = &http.Client{
			Transport: &headerTransport{
				headers: cfg.Headers,
				base:    http.DefaultTransport,
			},
		}
	}

	switch name {
	case "anthropic":
		return newAnthropicProvider(apiKey, cfg.BaseURL, httpClient), nil
	default:
		return newOpenAIProvider(apiKey, cfg.BaseURL, httpClient), nil
	}
}

// APIError represents a non-200 response from an LLM provider.
// It parses known JSON error formats to extract a human-readable message.
type APIError struct {
	Provider   string
	StatusCode int
	Body       []byte
}

func (e *APIError) Error() string {
	if msg := e.extractMessage(); msg != "" {
		return fmt.Sprintf("%s: %s (status %d)", e.Provider, msg, e.StatusCode)
	}
	body := string(e.Body)
	if len(body) > 200 {
		body = body[:200] + "…"
	}
	if body == "" {
		body = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("%s: %s (status %d)", e.Provider, body, e.StatusCode)
}

func (e *APIError) extractMessage() string {
	// Both OpenAI and Anthropic use {"error":{"message":"..."}}
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(e.Body, &parsed) == nil && parsed.Error.Message != "" {
		return parsed.Error.Message
	}
	return ""
}

// Retryable returns true if the error is worth retrying.
func (e *APIError) Retryable() bool {
	switch e.StatusCode {
	case 429, 500, 502, 503, 504, 529:
		return true
	}
	return false
}

// StreamError is an error event received after a stream has started (e.g.
// Anthropic's SSE "error" event with type "overloaded_error").
type StreamError struct {
	Provider string
	Type     string
	Message  string
}

func (e *StreamError) Error() string {
	return fmt.Sprintf("%s error: %s: %s", e.Provider, e.Type, e.Message)
}

// Retryable returns true for transient server-side error types.
func (e *StreamError) Retryable() bool {
	switch e.Type {
	case "overloaded_error", "api_error", "rate_limit_error":
		return true
	}
	return false
}

// IsRetryable reports whether err is a transient failure worth retrying:
// a retryable API status or stream error event, or a network-level error
// (connection reset, dropped connection, timeout). Context cancellation is
// never retryable.
func IsRetryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable()
	}
	var streamErr *StreamError
	if errors.As(err, &streamErr) {
		return streamErr.Retryable()
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE)
}

type headerTransport struct {
	headers map[string]string
	base    http.RoundTripper
}

func (tr *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range tr.headers {
		req.Header.Set(k, v)
	}
	return tr.base.RoundTrip(req)
}

// openRouterHost is the base URL fragment that identifies OpenRouter's API.
const openRouterHost = "openrouter.ai"

// usesOpenRouter reports whether the provider talks to OpenRouter, matching
// either the provider name or the base URL.
func usesOpenRouter(name, baseURL string) bool {
	return name == "openrouter" || strings.Contains(strings.ToLower(baseURL), openRouterHost)
}

// withOpenRouterAttribution returns headers with OpenRouter app-attribution
// defaults filled in for any header the caller didn't already set. OpenRouter
// identifies the app via HTTP-Referer (required, becomes the app's URL) and
// X-OpenRouter-Title (display name); X-OpenRouter-Categories places the app in
// the marketplace. Explicit config headers win over these defaults.
func withOpenRouterAttribution(headers map[string]string) map[string]string {
	if headers == nil {
		headers = map[string]string{}
	}
	attribution := map[string]string{
		"HTTP-Referer":            "https://github.com/meain/fin",
		"X-OpenRouter-Title":      "fin",
		"X-OpenRouter-Categories": "cli-agent",
	}
	for k, v := range attribution {
		if _, set := headers[k]; !set {
			headers[k] = v
		}
	}
	return headers
}
