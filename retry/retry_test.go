package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

var fast = Policy{MaxAttempts: 4, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond, NoJitter: true}

func TestSucceedsAfterFailures(t *testing.T) {
	calls := 0
	v, err := Do(context.Background(), fast, func(context.Context) (int, error) {
		calls++
		if calls < 3 {
			return 0, errors.New("flaky")
		}
		return 42, nil
	})
	if err != nil || v != 42 || calls != 3 {
		t.Fatalf("got v=%d err=%v calls=%d", v, err, calls)
	}
}

func TestExhausted(t *testing.T) {
	boom := errors.New("boom")
	calls := 0
	err := Run(context.Background(), fast, func(context.Context) error { calls++; return boom })
	if !errors.Is(err, ErrExhausted) || !errors.Is(err, boom) || calls != 4 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestPermanentStopsImmediately(t *testing.T) {
	bad := errors.New("bad request")
	calls := 0
	err := Run(context.Background(), fast, func(context.Context) error { calls++; return Permanent(bad) })
	if !errors.Is(err, bad) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestRetryIf(t *testing.T) {
	calls := 0
	p := fast
	p.RetryIf = func(error) bool { return false }
	_ = Run(context.Background(), p, func(context.Context) error { calls++; return errors.New("x") })
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := Policy{MaxAttempts: 10, BaseDelay: time.Hour, NoJitter: true}
	go func() { time.Sleep(5 * time.Millisecond); cancel() }()
	err := Run(ctx, p, func(context.Context) error { return errors.New("x") })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	p := Policy{BaseDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond, NoJitter: true}
	want := []time.Duration{10, 20, 40, 50, 50}
	for i, w := range want {
		if got := p.Backoff(i + 1); got != w*time.Millisecond {
			t.Fatalf("attempt %d: got %v want %v", i+1, got, w*time.Millisecond)
		}
	}
}
