package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
)

// Provider sends messages to an LLM and returns a streaming response.
type Provider interface {
	StreamCompletion(ctx context.Context, req CompletionRequest) (Stream, error)
}

// Stream yields deltas from a streaming completion.
type Stream interface {
	Recv() (StreamDelta, error)
	Close()
}

// NewProvider creates the appropriate provider for the given provider name.
func NewProvider(name string, cfg ProviderConfig) (Provider, error) {
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
		// Everything else is OpenAI-compatible
		return newOpenAIProvider(apiKey, cfg.BaseURL, httpClient), nil
	}
}

type headerTransport struct {
	headers map[string]string
	base    http.RoundTripper
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	return t.base.RoundTrip(req)
}
