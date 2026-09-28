package agent

import (
	"context"
	"testing"
	"time"

	"github.com/meain/fin/internal/provider"
	tp "github.com/meain/fin/internal/types"
)

// errStream yields the given deltas, then returns err.
type errStream struct {
	deltas []tp.StreamDelta
	err    error
}

func (s *errStream) Recv() (tp.StreamDelta, error) {
	if len(s.deltas) > 0 {
		d := s.deltas[0]
		s.deltas = s.deltas[1:]
		return d, nil
	}
	return tp.StreamDelta{}, s.err
}
func (s *errStream) Close() {}

func shortRetryDelay(t *testing.T) {
	old := baseRetryDelay
	baseRetryDelay = time.Millisecond
	t.Cleanup(func() { baseRetryDelay = old })
}

func TestRetry_OverloadedMidStreamIsRetried(t *testing.T) {
	shortRetryDelay(t)
	overloaded := &provider.StreamError{Provider: "anthropic", Type: "overloaded_error", Message: "Overloaded"}
	fp := &fakeProvider{streams: []provider.Stream{
		&errStream{err: overloaded},
		streamWithText("hello"),
	}}
	agent := newTestAgent(fp, nil, nil)
	if err := agent.AddUserMessage(context.Background(), "hi"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := agent.Messages()[2].Content; got != "hello" {
		t.Errorf("assistant content = %q, want hello", got)
	}
}

func TestRetry_NotRetriedAfterOutputStarted(t *testing.T) {
	shortRetryDelay(t)
	overloaded := &provider.StreamError{Provider: "anthropic", Type: "overloaded_error", Message: "Overloaded"}
	fp := &fakeProvider{streams: []provider.Stream{
		&errStream{deltas: []tp.StreamDelta{{Content: "partial"}}, err: overloaded},
		streamWithText("should not be requested"),
	}}
	agent := newTestAgent(fp, nil, nil)
	if err := agent.AddUserMessage(context.Background(), "hi"); err == nil {
		t.Fatal("expected an error")
	}
	if fp.idx != 1 {
		t.Errorf("provider called %d times, want 1", fp.idx)
	}
}

func TestIsRetryable(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{&provider.APIError{StatusCode: 504}, true},
		{&provider.APIError{StatusCode: 400}, false},
		{&provider.StreamError{Type: "overloaded_error"}, true},
		{&provider.StreamError{Type: "invalid_request_error"}, false},
		{context.Canceled, false},
	}
	for _, c := range cases {
		if got := provider.IsRetryable(c.err); got != c.want {
			t.Errorf("IsRetryable(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}
