package provider

import (
	"context"
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
