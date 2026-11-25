# go-resilience

[![ci](https://github.com/shani8615/go-resilience/actions/workflows/ci.yml/badge.svg)](https://github.com/shani8615/go-resilience/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/shani8615/go-resilience.svg)](https://pkg.go.dev/github.com/shani8615/go-resilience)
[![Go Report Card](https://goreportcard.com/badge/github.com/shani8615/go-resilience)](https://goreportcard.com/report/github.com/shani8615/go-resilience)
[![Release](https://img.shields.io/github/v/release/shani8615/go-resilience?sort=semver)](https://github.com/shani8615/go-resilience/releases)
[![Go version](https://img.shields.io/github/go-mod/go-version/shani8615/go-resilience)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Small, dependency-free resilience primitives for Go services: **retry with backoff**, **circuit breaker**, and **rate limiting**. Generics-friendly, context-aware, and easy to test (every package takes an injectable clock).

```bash
go get github.com/shani8615/go-resilience
```

## retry

Exponential backoff with full jitter. Stops on success, on `retry.Permanent(err)`, when `RetryIf` says no, or when the context ends.

```go
user, err := retry.Do(ctx, retry.Policy{MaxAttempts: 5, BaseDelay: 200 * time.Millisecond},
    func(ctx context.Context) (*User, error) {
        u, err := client.GetUser(ctx, id)
        if errors.Is(err, ErrNotFound) {
            return nil, retry.Permanent(err) // don't retry a 404
        }
        return u, err
    })
```

## breaker

Closed → open after N consecutive failures → half-open after a cool-down → closed again once probes succeed.

```go
b := breaker.New(breaker.Settings{FailureThreshold: 5, OpenTimeout: 30 * time.Second})

resp, err := breaker.Execute(b, func() (*http.Response, error) {
    return http.DefaultClient.Do(req)
})
if errors.Is(err, breaker.ErrOpen) {
    // fail fast / serve fallback
}
```

## ratelimit

Token bucket, plus a keyed limiter (per IP / user / API key) that evicts idle keys.

```go
limiter := ratelimit.NewKeyed(10, 20, 10*time.Minute) // 10 req/s, burst 20

func handler(w http.ResponseWriter, r *http.Request) {
    if !limiter.Allow(clientIP(r)) {
        http.Error(w, "too many requests", http.StatusTooManyRequests)
        return
    }
    // ...
}
```

## Combine them

```go
err := retry.Run(ctx, retry.Policy{MaxAttempts: 3}, func(ctx context.Context) error {
    if err := bucket.Wait(ctx); err != nil {
        return err
    }
    err := b.Do(func() error { return callPayments(ctx) })
    if errors.Is(err, breaker.ErrOpen) {
        return retry.Permanent(err)
    }
    return err
})
```

## Why go-resilience?

Go has excellent single-purpose libraries: [cenkalti/backoff](https://github.com/cenkalti/backoff) and [avast/retry-go](https://github.com/avast/retry-go) for retries, [sony/gobreaker](https://github.com/sony/gobreaker) for circuit breaking, and [golang.org/x/time/rate](https://pkg.go.dev/golang.org/x/time/rate) for rate limiting. go-resilience puts the three patterns in one small, dependency-free module with a consistent style:

- **Generics:** `retry.Do[T]` and `breaker.Execute[T]` return your value directly.
- **`retry.Permanent(err)`** to stop retrying 4xx-style errors.
- **Injectable clocks** everywhere, so your tests don't sleep.
- **A keyed limiter** (per IP, user or API key) that evicts idle keys.

If you only need a token bucket, `x/time/rate` is the standard and you should use it.

## License

MIT
