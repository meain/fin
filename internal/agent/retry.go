package agent

import (
	"context"
	"fmt"
	"math"
	mathrand "math/rand/v2"
	"time"

	"github.com/meain/fin/internal/provider"
	t "github.com/meain/fin/internal/types"
)

const (
	maxRetries    = 3
	maxRetryDelay = 30 * time.Second
)

// baseRetryDelay is a var so tests can shorten it.
var baseRetryDelay = 1 * time.Second

// streamWithRetry opens a completion stream and consumes it into one
// assistant message, retrying with exponential backoff + jitter on
// transient failures (429, 5xx, network errors, overloaded/api_error events
// mid-stream). A failure after text or tool-call deltas have reached the UI
// is not retried, since the partial output can't be taken back.
func (a *Agent) streamWithRetry(ctx context.Context, req t.CompletionRequest) (t.Message, time.Duration, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		msg, ttft, started, err := a.streamOnce(ctx, req)
		if err == nil {
			return msg, ttft, nil
		}
		lastErr = err

		if started || !provider.IsRetryable(err) || attempt == maxRetries || ctx.Err() != nil {
			return t.Message{}, 0, err
		}

		delay := retryDelay(attempt)
		a.ui.Retry(RetryData{Attempt: attempt + 1, MaxRetries: maxRetries, Delay: delay, Err: err})

		select {
		case <-ctx.Done():
			return t.Message{}, 0, ctx.Err()
		case <-time.After(delay):
		}
	}
	return t.Message{}, 0, lastErr
}

// streamOnce makes a single completion attempt. started reports whether
// any content or tool-call delta was received before a failure.
func (a *Agent) streamOnce(ctx context.Context, req t.CompletionRequest) (msg t.Message, ttft time.Duration, started bool, err error) {
	stream, err := a.provider.StreamCompletion(ctx, req)
	if err != nil {
		return t.Message{}, 0, false, err
	}
	defer stream.Close()
	msg, ttft, started, err = a.consumeStream(stream, time.Now())
	if err != nil {
		return msg, ttft, started, fmt.Errorf("stream error: %w", err)
	}
	return msg, ttft, started, nil
}

// retryDelay returns the backoff for the given attempt. Exponential with
// jitter up to maxRetryDelay.
func retryDelay(attempt int) time.Duration {
	delayF := float64(baseRetryDelay) * math.Pow(2, float64(attempt))
	if delayF > float64(maxRetryDelay) || delayF < 0 {
		delayF = float64(maxRetryDelay)
	}
	delay := time.Duration(delayF)

	half := int64(delay / 2)
	if half <= 0 {
		return delay
	}
	jitter := time.Duration(mathrand.Int64N(half))
	return delay + jitter
}
