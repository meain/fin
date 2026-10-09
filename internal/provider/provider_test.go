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

func TestNew_OpenRouter_AttributionHeaders(t *testing.T) {
	t.Setenv("TEST_OR_KEY", "sk-test-key")
	p, err := New("openrouter", Config{
		APIKeyEnv: "TEST_OR_KEY",
		BaseURL:   "https://openrouter.ai/api",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	oai, ok := p.(*openaiProvider)
	if !ok {
		t.Fatalf("expected *openaiProvider for openrouter, got %T", p)
	}
	if oai.httpClient == nil {
		t.Fatal("expected httpClient with attribution headers")
	}
	tr, ok := oai.httpClient.Transport.(*headerTransport)
	if !ok {
		t.Fatalf("expected *headerTransport, got %T", oai.httpClient.Transport)
	}
	for _, h := range []string{"HTTP-Referer", "X-OpenRouter-Title", "X-OpenRouter-Categories"} {
		if tr.headers[h] == "" {
			t.Errorf("expected attribution header %q to be set, got empty", h)
		}
	}
}

func TestWithOpenRouterAttribution_UserHeadersWin(t *testing.T) {
	h := withOpenRouterAttribution(map[string]string{
		"HTTP-Referer":       "https://example.com/mine",
		"X-OpenRouter-Title": "my-app",
	})
	if h["HTTP-Referer"] != "https://example.com/mine" {
		t.Errorf("explicit HTTP-Referer should win, got %q", h["HTTP-Referer"])
	}
	if h["X-OpenRouter-Title"] != "my-app" {
		t.Errorf("explicit X-OpenRouter-Title should win, got %q", h["X-OpenRouter-Title"])
	}
	if h["X-OpenRouter-Categories"] != "cli-agent" {
		t.Errorf("expected default cli-agent category, got %q", h["X-OpenRouter-Categories"])
	}
}

func TestUsesOpenRouter(t *testing.T) {
	cases := []struct {
		name, baseURL string
		want          bool
	}{
		{"openrouter", "", true},
		{"", "https://openrouter.ai/api", true},
		{"", "https://OPENROUTER.AI/api", true},
		{"openai", "https://api.openai.com", false},
		{"ollama", "http://localhost:11434", false},
		{"", "https://example.com", false},
	}
	for _, c := range cases {
		if got := usesOpenRouter(c.name, c.baseURL); got != c.want {
			t.Errorf("usesOpenRouter(%q, %q) = %v, want %v", c.name, c.baseURL, got, c.want)
		}
	}
}
