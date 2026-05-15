package agent

import (
	"context"
	"errors"
	"math"
	mathrand "math/rand/v2"
	"time"

	"github.com/meain/fin/internal/provider"
	t "github.com/meain/fin/internal/types"
)

const (
	maxRetries     = 3
	baseRetryDelay = 1 * time.Second
	maxRetryDelay  = 30 * time.Second
)

// streamWithRetry calls StreamCompletion with exponential backoff + jitter
// on retryable errors (429, 5xx, transient network). Non-retryable errors
// short-circuit immediately.
func (a *Agent) streamWithRetry(ctx context.Context, req t.CompletionRequest) (provider.Stream, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		stream, err := a.provider.StreamCompletion(ctx, req)
		if err == nil {
			return stream, nil
		}

		lastErr = err

		var apiErr *provider.APIError
		retryable := errors.As(err, &apiErr) && apiErr.Retryable()
		if !retryable || attempt == maxRetries {
			return nil, err
		}

		delay := retryDelay(attempt)
		a.ui.Retry(RetryData{Attempt: attempt + 1, MaxRetries: maxRetries, Delay: delay, Err: err})

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, lastErr
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
