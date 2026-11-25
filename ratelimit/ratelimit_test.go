package ratelimit

import (
	"context"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func TestBurstThenRefill(t *testing.T) {
	c := &clock{t: time.Unix(0, 0)}
	b := newBucket(2, 3, c.now) // 2 tokens/s, burst 3
	for i := 0; i < 3; i++ {
		if !b.Allow() {
			t.Fatalf("burst token %d denied", i)
		}
	}
	if b.Allow() {
		t.Fatal("4th call should be denied")
	}
	c.t = c.t.Add(500 * time.Millisecond) // +1 token
	if !b.Allow() {
		t.Fatal("refilled token denied")
	}
	if b.Allow() {
		t.Fatal("should be empty again")
	}
	c.t = c.t.Add(time.Hour)
	n := 0
	for b.Allow() {
		n++
	}
	if n != 3 {
		t.Fatalf("refill must cap at burst, got %d", n)
	}
}

func TestWait(t *testing.T) {
	b := NewBucket(200, 1)
	b.Allow()
	start := time.Now()
	if err := b.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("waited too long")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	slow := NewBucket(0.001, 1)
	slow.Allow()
	if err := slow.Wait(ctx); err == nil {
		t.Fatal("expected context error")
	}
}

func TestKeyedIsolationAndEviction(t *testing.T) {
	c := &clock{t: time.Unix(0, 0)}
	k := NewKeyed(1, 1, time.Minute)
	k.now = c.now
	if !k.Allow("a") || k.Allow("a") {
		t.Fatal("key a: want allow then deny")
	}
	if !k.Allow("b") {
		t.Fatal("key b must have its own bucket")
	}
	c.t = c.t.Add(2 * time.Minute)
	k.Allow("c")
	if k.Len() != 1 {
		t.Fatalf("idle keys should be evicted, have %d", k.Len())
	}
}
