package feed

import (
	"context"
	"sync"
	"time"
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
	mu           sync.Mutex
	outcomes     []AccessOutcome
	observations []AttemptObservation
}

// AttemptObservation pairs a bounded Feed outcome with the time the fetch
// result was observed. It prevents later PodcastIndex/episode processing from
// rewriting the attempt timestamp.
type AttemptObservation struct {
	Outcome    AccessOutcome
	ObservedAt time.Time
}

func (c *AttemptCollector) Record(outcome AccessOutcome) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.outcomes = append(c.outcomes, outcome)
	c.observations = append(c.observations, AttemptObservation{Outcome: outcome, ObservedAt: time.Now()})
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

// Observations returns a copy in fetch-completion order.
func (c *AttemptCollector) Observations() []AttemptObservation {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]AttemptObservation(nil), c.observations...)
}
