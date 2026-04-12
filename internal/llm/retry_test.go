package llm

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errorProvider returns an error for the first N calls, then succeeds.
type errorProvider struct {
	errCount   int
	err        error
	calls      int
	successMsg string
}

func (p *errorProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
	p.calls++
	if p.calls <= p.errCount {
		return nil, p.err
	}
	return &Response{
		Content:    []ContentBlock{NewTextBlock(p.successMsg)},
		StopReason: StopEndTurn,
		Usage:      Usage{InputTokens: 10, OutputTokens: 5},
	}, nil
}

// alwaysErrorProvider always returns the same error.
type alwaysErrorProvider struct {
	err   error
	calls int
}

func (p *alwaysErrorProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
	p.calls++
	return nil, p.err
}

func TestRetryProvider_SuccessNoRetry(t *testing.T) {
	inner := &errorProvider{errCount: 0, successMsg: "ok"}
	provider := NewRetryProvider(inner, DefaultRetryConfig())

	resp, err := provider.Complete(context.Background(), &Request{Model: "test"})
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.TextContent())
	assert.Equal(t, 1, inner.calls)
}

func TestRetryProvider_RetryOnTransientError(t *testing.T) {
	// Fail twice with 429, then succeed
	inner := &errorProvider{
		errCount:   2,
		err:        fmt.Errorf("anthropic API: 429 rate limit exceeded"),
		successMsg: "recovered",
	}

	config := RetryConfig{
		MaxRetries: 3,
		BaseDelay:  10 * time.Millisecond, // fast for testing
		MaxDelay:   50 * time.Millisecond,
		Jitter:     0,
	}
	provider := NewRetryProvider(inner, config)

	resp, err := provider.Complete(context.Background(), &Request{Model: "test"})
	require.NoError(t, err)
	assert.Equal(t, "recovered", resp.TextContent())
	assert.Equal(t, 3, inner.calls) // 2 failures + 1 success
}

func TestRetryProvider_ExhaustsRetries(t *testing.T) {
	inner := &alwaysErrorProvider{
		err: fmt.Errorf("anthropic API: 500 internal server error"),
	}

	config := RetryConfig{
		MaxRetries: 2,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   50 * time.Millisecond,
		Jitter:     0,
	}
	provider := NewRetryProvider(inner, config)

	_, err := provider.Complete(context.Background(), &Request{Model: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max retries (2) exhausted")
	assert.Equal(t, 3, inner.calls) // initial + 2 retries
}

func TestRetryProvider_NoRetryOnNonTransient(t *testing.T) {
	inner := &alwaysErrorProvider{
		err: fmt.Errorf("anthropic API: 400 bad request: invalid model"),
	}

	config := RetryConfig{
		MaxRetries: 3,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   50 * time.Millisecond,
	}
	provider := NewRetryProvider(inner, config)

	_, err := provider.Complete(context.Background(), &Request{Model: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400 bad request")
	assert.Equal(t, 1, inner.calls) // no retries for non-transient
}

func TestRetryProvider_RespectsContextCancellation(t *testing.T) {
	inner := &alwaysErrorProvider{
		err: fmt.Errorf("anthropic API: 429 rate limit exceeded"),
	}

	ctx, cancel := context.WithCancel(context.Background())

	config := RetryConfig{
		MaxRetries: 10,
		BaseDelay:  1 * time.Second, // long delay
		MaxDelay:   5 * time.Second,
	}
	provider := NewRetryProvider(inner, config)

	// Cancel after a short time
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := provider.Complete(ctx, &Request{Model: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "canceled")
}

func TestRetryProvider_RetryOn529Overloaded(t *testing.T) {
	inner := &errorProvider{
		errCount:   1,
		err:        fmt.Errorf("anthropic API: 529 overloaded"),
		successMsg: "ok",
	}

	config := RetryConfig{
		MaxRetries: 2,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   50 * time.Millisecond,
		Jitter:     0,
	}
	provider := NewRetryProvider(inner, config)

	resp, err := provider.Complete(context.Background(), &Request{Model: "test"})
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.TextContent())
	assert.Equal(t, 2, inner.calls)
}

func TestRetryProvider_RetryOnConnectionError(t *testing.T) {
	inner := &errorProvider{
		errCount:   1,
		err:        fmt.Errorf("connection refused"),
		successMsg: "ok",
	}

	config := RetryConfig{
		MaxRetries: 2,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   50 * time.Millisecond,
		Jitter:     0,
	}
	provider := NewRetryProvider(inner, config)

	resp, err := provider.Complete(context.Background(), &Request{Model: "test"})
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.TextContent())
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    bool
	}{
		{"nil error", nil, false},
		{"429 rate limit", fmt.Errorf("429 Too Many Requests"), true},
		{"500 server error", fmt.Errorf("500 Internal Server Error"), true},
		{"502 bad gateway", fmt.Errorf("502 Bad Gateway"), true},
		{"503 unavailable", fmt.Errorf("503 Service Unavailable"), true},
		{"504 gateway timeout", fmt.Errorf("504 Gateway Timeout"), true},
		{"529 overloaded", fmt.Errorf("529 overloaded"), true},
		{"connection refused", fmt.Errorf("connection refused"), true},
		{"connection reset", fmt.Errorf("connection reset by peer"), true},
		{"timeout", fmt.Errorf("request timeout"), true},
		{"400 bad request", fmt.Errorf("400 Bad Request"), false},
		{"401 unauthorized", fmt.Errorf("401 Unauthorized"), false},
		{"403 forbidden", fmt.Errorf("403 Forbidden"), false},
		{"random error", fmt.Errorf("something went wrong"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isRetryable(tt.err))
		})
	}
}

func TestBackoffDelay(t *testing.T) {
	config := RetryConfig{
		MaxRetries: 5,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   5 * time.Second,
		Jitter:     0, // no jitter for deterministic tests
	}
	provider := NewRetryProvider(nil, config)

	// Without jitter, delays should be: 100ms, 200ms, 400ms, 800ms, 1600ms
	assert.Equal(t, 100*time.Millisecond, provider.backoffDelay(0))
	assert.Equal(t, 200*time.Millisecond, provider.backoffDelay(1))
	assert.Equal(t, 400*time.Millisecond, provider.backoffDelay(2))
	assert.Equal(t, 800*time.Millisecond, provider.backoffDelay(3))
	assert.Equal(t, 1600*time.Millisecond, provider.backoffDelay(4))

	// Should cap at MaxDelay
	config2 := RetryConfig{
		MaxRetries: 5,
		BaseDelay:  1 * time.Second,
		MaxDelay:   2 * time.Second,
		Jitter:     0,
	}
	provider2 := NewRetryProvider(nil, config2)
	assert.Equal(t, 2*time.Second, provider2.backoffDelay(5)) // 32s capped to 2s
}
