// Package ratelimit provides a token-bucket limiter and a keyed variant
// (per user, per IP, per API key) with idle-bucket eviction.
package ratelimit

import (
	"context"
	"sync"
	"time"
)

// Bucket is a token bucket: it holds up to Burst tokens and refills at Rate
// tokens per second. Safe for concurrent use.
type Bucket struct {
	rate  float64
	burst float64
	now   func() time.Time

	mu     sync.Mutex
	tokens float64
	last   time.Time
}

// NewBucket returns a full bucket.
func NewBucket(ratePerSec float64, burst int) *Bucket {
	return newBucket(ratePerSec, burst, time.Now)
}

func newBucket(rate float64, burst int, now func() time.Time) *Bucket {
	if burst < 1 {
		burst = 1
	}
	return &Bucket{rate: rate, burst: float64(burst), now: now, tokens: float64(burst), last: now()}
}

func (b *Bucket) refill(t time.Time) {
	if el := t.Sub(b.last).Seconds(); el > 0 {
		b.tokens = min(b.burst, b.tokens+el*b.rate)
		b.last = t
	}
}

// Allow takes one token if available.
func (b *Bucket) Allow() bool { ok, _ := b.reserve(); return ok }

// reserve takes a token if available, otherwise returns how long until one is.
func (b *Bucket) reserve() (bool, time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refill(b.now())
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	if b.rate <= 0 {
		return false, time.Duration(1<<63 - 1)
	}
	return false, time.Duration((1 - b.tokens) / b.rate * float64(time.Second))
}

// Wait blocks until a token is available or ctx ends.
func (b *Bucket) Wait(ctx context.Context) error {
	for {
		ok, d := b.reserve()
		if ok {
			return nil
		}
		t := time.NewTimer(d)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

// Keyed holds one bucket per key and evicts buckets idle for longer than TTL.
type Keyed struct {
	rate  float64
	burst int
	ttl   time.Duration
	now   func() time.Time

	mu      sync.Mutex
	buckets map[string]*entry
	lastGC  time.Time
}

type entry struct {
	b    *Bucket
	seen time.Time
}

// NewKeyed creates a keyed limiter. ttl<=0 defaults to 10 minutes.
func NewKeyed(ratePerSec float64, burst int, ttl time.Duration) *Keyed {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &Keyed{rate: ratePerSec, burst: burst, ttl: ttl, now: time.Now, buckets: map[string]*entry{}}
}

// Allow reports whether a request for key may proceed.
func (k *Keyed) Allow(key string) bool {
	k.mu.Lock()
	now := k.now()
	if now.Sub(k.lastGC) > k.ttl {
		for key, e := range k.buckets {
			if now.Sub(e.seen) > k.ttl {
				delete(k.buckets, key)
			}
		}
		k.lastGC = now
	}
	e, ok := k.buckets[key]
	if !ok {
		e = &entry{b: newBucket(k.rate, k.burst, k.now)}
		k.buckets[key] = e
	}
	e.seen = now
	k.mu.Unlock()
	return e.b.Allow()
}

// Len returns the number of tracked keys.
func (k *Keyed) Len() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.buckets)
}
