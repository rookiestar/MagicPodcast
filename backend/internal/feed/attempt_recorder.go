package feed

import (
	"context"
	"sync"
)

type attemptRecorderContextKey struct{}

// AttemptRecorder collects bounded AccessOutcome values for one logical
// workflow Feed operation. It never receives bodies, credentials, or headers.
type AttemptRecorder func(AccessOutcome)

// WithAttemptRecorder attaches a recorder to all Feed fetches using ctx.
func WithAttemptRecorder(ctx context.Context, recorder AttemptRecorder) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, attemptRecorderContextKey{}, recorder)
}

func recordAttempt(ctx context.Context, result *FetchResult) {
	if ctx == nil || result == nil {
		return
	}
	recorder, _ := ctx.Value(attemptRecorderContextKey{}).(AttemptRecorder)
	if recorder != nil {
		recorder(result.Access)
	}
}

// AttemptCollector is a concurrency-safe recorder used by workflow workers.
type AttemptCollector struct {
	mu       sync.Mutex
	outcomes []AccessOutcome
}

func (c *AttemptCollector) Record(outcome AccessOutcome) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.outcomes = append(c.outcomes, outcome)
	c.mu.Unlock()
}

func (c *AttemptCollector) Outcomes() []AccessOutcome {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]AccessOutcome(nil), c.outcomes...)
}
