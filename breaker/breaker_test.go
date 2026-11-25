package breaker

import (
	"errors"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

var errDown = errors.New("down")

func TestOpensAfterThresholdAndRecovers(t *testing.T) {
	c := &clock{t: time.Unix(0, 0)}
	var transitions []string
	b := New(Settings{
		FailureThreshold: 3, OpenTimeout: time.Second, Now: c.now,
		OnStateChange: func(from, to State) { transitions = append(transitions, from.String()+">"+to.String()) },
	})

	for i := 0; i < 3; i++ {
		_ = b.Do(func() error { return errDown })
	}
	if b.State() != Open {
		t.Fatalf("want open, got %v", b.State())
	}
	called := false
	if err := b.Do(func() error { called = true; return nil }); !errors.Is(err, ErrOpen) || called {
		t.Fatalf("open breaker must short-circuit: err=%v called=%v", err, called)
	}

	c.advance(time.Second)
	if b.State() != HalfOpen {
		t.Fatalf("want half-open, got %v", b.State())
	}
	if err := b.Do(func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if b.State() != Closed {
		t.Fatalf("want closed, got %v", b.State())
	}
	want := []string{"closed>open", "open>half-open", "half-open>closed"}
	if len(transitions) != len(want) {
		t.Fatalf("transitions=%v", transitions)
	}
	for i := range want {
		if transitions[i] != want[i] {
			t.Fatalf("transitions=%v", transitions)
		}
	}
}

func TestHalfOpenFailureReopens(t *testing.T) {
	c := &clock{t: time.Unix(0, 0)}
	b := New(Settings{FailureThreshold: 1, OpenTimeout: time.Second, Now: c.now})
	_ = b.Do(func() error { return errDown })
	c.advance(time.Second)
	_ = b.Do(func() error { return errDown })
	if b.State() != Open {
		t.Fatalf("want open, got %v", b.State())
	}
}

func TestSuccessResetsFailureCount(t *testing.T) {
	b := New(Settings{FailureThreshold: 2})
	_ = b.Do(func() error { return errDown })
	_ = b.Do(func() error { return nil })
	_ = b.Do(func() error { return errDown })
	if b.State() != Closed {
		t.Fatalf("non-consecutive failures must not open, got %v", b.State())
	}
}

func TestExecuteReturnsValue(t *testing.T) {
	b := New(Settings{})
	v, err := Execute(b, func() (string, error) { return "ok", nil })
	if err != nil || v != "ok" {
		t.Fatalf("v=%q err=%v", v, err)
	}
}
