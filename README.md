# go-resilience

[![ci](https://github.com/shani8615/go-resilience/actions/workflows/ci.yml/badge.svg)](https://github.com/shani8615/go-resilience/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/shani8615/go-resilience.svg)](https://pkg.go.dev/github.com/shani8615/go-resilience)

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

## License

MIT
