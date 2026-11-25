// Package breaker implements a circuit breaker with closed, open and
// half-open states. It stops calling a failing dependency for a cool-down
// period, then lets a limited number of probe calls through.
package breaker

import (
	"context"
	"errors"
	"sync"
	"time"
)

// State of the breaker.
type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	default:
		return "half-open"
	}
}

// ErrOpen is returned without calling fn while the breaker is open.
var ErrOpen = errors.New("breaker: circuit open")

// Settings configures a Breaker. Zero values get sensible defaults.
type Settings struct {
	FailureThreshold int           // consecutive failures that open the circuit; default 5
	OpenTimeout      time.Duration // time spent open before probing; default 30s
	HalfOpenMax      int           // probes allowed in half-open; default 1
	// IsFailure decides if an error counts as a failure. nil counts every
	// non-nil error except context.Canceled.
	IsFailure     func(err error) bool
	OnStateChange func(from, to State)
	Now           func() time.Time // injectable clock for tests
}

// Breaker is safe for concurrent use.
type Breaker struct {
	s Settings

	mu        sync.Mutex
	state     State
	failures  int
	openedAt  time.Time
	inFlight  int
	successes int
}

// New returns a closed breaker.
func New(s Settings) *Breaker {
	if s.FailureThreshold <= 0 {
		s.FailureThreshold = 5
	}
	if s.OpenTimeout <= 0 {
		s.OpenTimeout = 30 * time.Second
	}
	if s.HalfOpenMax <= 0 {
		s.HalfOpenMax = 1
	}
	if s.IsFailure == nil {
		s.IsFailure = func(err error) bool { return err != nil && !errors.Is(err, context.Canceled) }
	}
	if s.Now == nil {
		s.Now = time.Now
	}
	return &Breaker{s: s}
}

// State reports the current state, moving open → half-open if the timeout passed.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tick()
	return b.state
}

// Execute calls fn if the breaker allows it and records the outcome.
func Execute[T any](b *Breaker, fn func() (T, error)) (T, error) {
	var zero T
	if err := b.allow(); err != nil {
		return zero, err
	}
	v, err := fn()
	b.record(err)
	return v, err
}

// Do is Execute for functions returning only an error.
func (b *Breaker) Do(fn func() error) error {
	_, err := Execute(b, func() (struct{}, error) { return struct{}{}, fn() })
	return err
}

func (b *Breaker) allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tick()
	switch b.state {
	case Open:
		return ErrOpen
	case HalfOpen:
		if b.inFlight >= b.s.HalfOpenMax {
			return ErrOpen
		}
		b.inFlight++
	}
	return nil
}

func (b *Breaker) record(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	failed := b.s.IsFailure(err)
	switch b.state {
	case Closed:
		if failed {
			b.failures++
			if b.failures >= b.s.FailureThreshold {
				b.setState(Open)
			}
		} else {
			b.failures = 0
		}
	case HalfOpen:
		b.inFlight--
		if failed {
			b.setState(Open)
			return
		}
		b.successes++
		if b.successes >= b.s.HalfOpenMax {
			b.setState(Closed)
		}
	}
}

func (b *Breaker) tick() {
	if b.state == Open && b.s.Now().Sub(b.openedAt) >= b.s.OpenTimeout {
		b.setState(HalfOpen)
	}
}

func (b *Breaker) setState(to State) {
	from := b.state
	if from == to {
		return
	}
	b.state = to
	b.failures, b.inFlight, b.successes = 0, 0, 0
	if to == Open {
		b.openedAt = b.s.Now()
	}
	if b.s.OnStateChange != nil {
		b.s.OnStateChange(from, to)
	}
}
