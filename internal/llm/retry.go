package llm

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/xmh1011/glaude/internal/telemetry"
)

// Retry configuration defaults, inspired by Claude Code's withRetry.ts.
const (
	DefaultMaxRetries   = 5
	DefaultBaseDelay    = 1 * time.Second
	DefaultMaxDelay     = 30 * time.Second
	DefaultJitterFactor = 0.2 // ±20% jitter
)

// RetryConfig controls the retry behavior for API calls.
type RetryConfig struct {
	MaxRetries int           // maximum number of retry attempts
	BaseDelay  time.Duration // initial backoff delay
	MaxDelay   time.Duration // maximum backoff delay cap
	Jitter     float64       // jitter factor (0.0 to 1.0)
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: DefaultMaxRetries,
		BaseDelay:  DefaultBaseDelay,
		MaxDelay:   DefaultMaxDelay,
		Jitter:     DefaultJitterFactor,
	}
}

// RetryProvider wraps a Provider with automatic retry logic for transient errors.
// It implements exponential backoff with jitter, matching the pattern from
// Claude Code's withRetry.ts.
type RetryProvider struct {
	inner  Provider
	config RetryConfig
}

// NewRetryProvider creates a retrying wrapper around the given provider.
func NewRetryProvider(inner Provider, config RetryConfig) *RetryProvider {
	if config.MaxRetries <= 0 {
		config.MaxRetries = DefaultMaxRetries
	}
	if config.BaseDelay <= 0 {
		config.BaseDelay = DefaultBaseDelay
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = DefaultMaxDelay
	}
	return &RetryProvider{inner: inner, config: config}
}

// Complete sends the request, retrying on transient/retryable errors.
func (r *RetryProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
	var lastErr error

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		resp, err := r.inner.Complete(ctx, req)
		if err == nil {
			return resp, nil
		}

		if !isRetryable(err) {
			return nil, err
		}

		lastErr = err

		if attempt < r.config.MaxRetries {
			delay := r.backoffDelay(attempt)

			telemetry.Log.
				WithField("attempt", attempt+1).
				WithField("max_retries", r.config.MaxRetries).
				WithField("delay_ms", delay.Milliseconds()).
				WithField("error", err.Error()).
				Warn("retrying API call")

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				// continue to next attempt
			}
		}
	}

	return nil, fmt.Errorf("max retries (%d) exhausted: %w", r.config.MaxRetries, lastErr)
}

// CompleteStream wraps the inner provider's streaming method with retry logic.
// Unlike non-streaming Complete, only the initial connection is retried.
// Once a stream is successfully established, it is returned as-is.
func (r *RetryProvider) CompleteStream(ctx context.Context, req *Request) (<-chan StreamEvent, error) {
	inner, ok := r.inner.(StreamingProvider)
	if !ok {
		return nil, fmt.Errorf("retry: inner provider does not support streaming")
	}

	var lastErr error
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		ch, err := inner.CompleteStream(ctx, req)
		if err == nil {
			return ch, nil
		}

		if !isRetryable(err) {
			return nil, err
		}

		lastErr = err

		if attempt < r.config.MaxRetries {
			delay := r.backoffDelay(attempt)

			telemetry.Log.
				WithField("attempt", attempt+1).
				WithField("max_retries", r.config.MaxRetries).
				WithField("delay_ms", delay.Milliseconds()).
				WithField("error", err.Error()).
				Warn("retrying stream API call")

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				// continue to next attempt
			}
		}
	}

	return nil, fmt.Errorf("max retries (%d) exhausted: %w", r.config.MaxRetries, lastErr)
}

// backoffDelay calculates exponential backoff with jitter for the given attempt.
// Formula: min(base * 2^attempt + jitter, maxDelay)
func (r *RetryProvider) backoffDelay(attempt int) time.Duration {
	// Exponential: base * 2^attempt
	delay := float64(r.config.BaseDelay) * math.Pow(2, float64(attempt))

	// Cap at max delay
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}

	// Apply jitter: delay * (1 ± jitter)
	if r.config.Jitter > 0 {
		jitter := delay * r.config.Jitter
		delay += (rand.Float64()*2 - 1) * jitter
	}

	if delay < 0 {
		delay = float64(r.config.BaseDelay)
	}

	return time.Duration(delay)
}

// isRetryable determines if an error is transient and worth retrying.
// Matches Claude Code's classification: 429, 529, 5xx errors.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Anthropic SDK wraps HTTP errors with status codes in the message.
	// Check for known retryable status codes.

	// 429 Too Many Requests (rate limit)
	if containsStatusCode(errStr, http.StatusTooManyRequests) {
		return true
	}

	// 529 Overloaded (Anthropic-specific)
	if strings.Contains(errStr, "529") && strings.Contains(errStr, "overloaded") {
		return true
	}

	// 5xx Server Errors (500, 502, 503, 504)
	if containsStatusCode(errStr, http.StatusInternalServerError) ||
		containsStatusCode(errStr, http.StatusBadGateway) ||
		containsStatusCode(errStr, http.StatusServiceUnavailable) ||
		containsStatusCode(errStr, http.StatusGatewayTimeout) {
		return true
	}

	// Connection/timeout errors
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "temporary failure") ||
		strings.Contains(errStr, "ECONNRESET") {
		return true
	}

	return false
}

// containsStatusCode checks if an error string mentions a specific HTTP status code.
func containsStatusCode(errStr string, code int) bool {
	codeStr := fmt.Sprintf("%d", code)
	return strings.Contains(errStr, codeStr)
}
