package provider

import (
	"testing"
)

func TestNew_Anthropic(t *testing.T) {
	t.Setenv("TEST_ANTHROPIC_KEY", "sk-test-key")
	p, err := New("anthropic", Config{APIKeyEnv: "TEST_ANTHROPIC_KEY"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if _, ok := p.(*anthropicProvider); !ok {
		t.Fatalf("expected *anthropicProvider, got %T", p)
	}
}

func TestNew_OpenAI(t *testing.T) {
	t.Setenv("TEST_OPENAI_KEY", "sk-test-key")
	p, err := New("openai", Config{APIKeyEnv: "TEST_OPENAI_KEY"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if _, ok := p.(*openaiProvider); !ok {
		t.Fatalf("expected *openaiProvider, got %T", p)
	}
}

func TestNew_UnknownName_ReturnsOpenAI(t *testing.T) {
	t.Setenv("TEST_UNKNOWN_KEY", "sk-test-key")
	p, err := New("some-custom-provider", Config{APIKeyEnv: "TEST_UNKNOWN_KEY"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if _, ok := p.(*openaiProvider); !ok {
		t.Fatalf("expected *openaiProvider for unknown provider name, got %T", p)
	}
}

func TestNew_EmptyAPIKey(t *testing.T) {
	t.Setenv("TEST_EMPTY_KEY", "")
	_, err := New("anthropic", Config{APIKeyEnv: "TEST_EMPTY_KEY"})
	if err == nil {
		t.Fatal("expected error for empty API key env var, got nil")
	}
}

func TestNew_NoAPIKeyEnv(t *testing.T) {
	// When APIKeyEnv is not set in config, no error should occur
	p, err := New("openai", Config{})
	if err != nil {
		t.Fatalf("expected no error when APIKeyEnv is empty, got %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNew_CustomHeaders(t *testing.T) {
	t.Setenv("TEST_HDR_KEY", "sk-test-key")
	p, err := New("openai", Config{
		APIKeyEnv: "TEST_HDR_KEY",
		Headers:   map[string]string{"X-Custom": "value"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	oai, ok := p.(*openaiProvider)
	if !ok {
		t.Fatalf("expected *openaiProvider, got %T", p)
	}
	// Verify the httpClient is not the default (custom transport was set)
	if oai.httpClient == nil {
		t.Fatal("expected non-nil httpClient with custom headers")
	}
}
