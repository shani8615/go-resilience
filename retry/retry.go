// Package retry runs an operation until it succeeds, the context ends, or
// the attempt budget is spent, sleeping with exponential backoff and jitter
// between attempts.
package retry

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"time"
)

// Policy configures retries. The zero value is usable and means: 3 attempts,
// 100ms base delay, 5s max delay, full jitter, retry every error.
type Policy struct {
	MaxAttempts int           // total attempts including the first; <=0 means 3
	BaseDelay   time.Duration // delay before the 2nd attempt; <=0 means 100ms
	MaxDelay    time.Duration // cap per sleep; <=0 means 5s
	Multiplier  float64       // growth factor; <=1 means 2
	NoJitter    bool          // disable full jitter (useful in tests)
	// RetryIf decides if err is retryable. nil retries everything except
	// errors wrapped with Permanent.
	RetryIf func(err error) bool
	// OnRetry is called before each sleep, e.g. for logging/metrics.
	OnRetry func(attempt int, err error, wait time.Duration)
}

type permanentError struct{ err error }

func (p permanentError) Error() string { return p.err.Error() }
func (p permanentError) Unwrap() error { return p.err }

// Permanent marks err as non-retryable; Do returns it immediately.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err}
}

// ErrExhausted wraps the last error once all attempts are used.
var ErrExhausted = errors.New("retry: attempts exhausted")

// Do runs fn with the policy and returns its value on success.
func Do[T any](ctx context.Context, p Policy, fn func(ctx context.Context) (T, error)) (T, error) {
	p = p.withDefaults()
	var zero T
	var lastErr error
	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, errors.Join(err, lastErr)
		}
		v, err := fn(ctx)
		if err == nil {
			return v, nil
		}
		lastErr = err
		var perm permanentError
		if errors.As(err, &perm) {
			return zero, perm.err
		}
		if p.RetryIf != nil && !p.RetryIf(err) {
			return zero, err
		}
		if attempt == p.MaxAttempts {
			break
		}
		wait := p.Backoff(attempt)
		if p.OnRetry != nil {
			p.OnRetry(attempt, err, wait)
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return zero, errors.Join(ctx.Err(), lastErr)
		case <-t.C:
		}
	}
	return zero, errors.Join(ErrExhausted, lastErr)
}

// Run is Do for operations that return only an error.
func Run(ctx context.Context, p Policy, fn func(ctx context.Context) error) error {
	_, err := Do(ctx, p, func(ctx context.Context) (struct{}, error) { return struct{}{}, fn(ctx) })
	return err
}

// Backoff returns the sleep after the given (1-based) failed attempt.
func (p Policy) Backoff(attempt int) time.Duration {
	p = p.withDefaults()
	d := float64(p.BaseDelay) * math.Pow(p.Multiplier, float64(attempt-1))
	if d > float64(p.MaxDelay) || math.IsInf(d, 0) {
		d = float64(p.MaxDelay)
	}
	if p.NoJitter {
		return time.Duration(d)
	}
	return time.Duration(rand.Int64N(int64(d) + 1)) //nolint:gosec // jitter does not need crypto rand
}

func (p Policy) withDefaults() Policy {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 3
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = 100 * time.Millisecond
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = 5 * time.Second
	}
	if p.Multiplier <= 1 {
		p.Multiplier = 2
	}
	return p
}
