package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

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
	case 429, 500, 502, 503, 529:
		return true
	}
	return false
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
