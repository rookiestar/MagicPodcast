package feed

import (
	"context"
	"strings"
	"sync"
	"time"
)

// SoftRateTier is the adaptive soft-rate band for a domain that never hard-opens
// on a single 403. Local policy may only slow traffic; it never drops concurrency
// to zero.
type SoftRateTier string

const (
	SoftRateNormal   SoftRateTier = "normal"
	SoftRateCautious SoftRateTier = "cautious"
	SoftRateSlow     SoftRateTier = "slow"
)

// Default inter-request spacing for each soft-rate tier. Slow is deliberately
// non-zero but finite so the domain remains usable inside a 10-minute batch.
const (
	softRateSpacingNormal   = 0
	softRateSpacingCautious = 2 * time.Second
	softRateSpacingSlow     = 10 * time.Second
	// Consecutive live successes required before recovering one tier.
	softRateSuccessesToRecover = 2
)

// SoftRateController tracks per-domain soft-rate state for domains that opt
// into SoftRateEnabled (Xiaoyuzhou default). It is independent of the circuit
// breaker: a 403 only slows the shared queue, never OPEN-blocks siblings.
type SoftRateController struct {
	mu      sync.Mutex
	domains map[string]*softRateState
	// now is injectable for deterministic tests.
	now func() time.Time
}

type softRateState struct {
	tier          SoftRateTier
	consecutiveOK int
	lastReleaseAt time.Time
	nextAllowedAt time.Time
}

// NewSoftRateController constructs an empty controller.
func NewSoftRateController() *SoftRateController {
	return &SoftRateController{
		domains: make(map[string]*softRateState),
		now:     time.Now,
	}
}

// Tier returns the current soft-rate tier for domain (default normal).
func (c *SoftRateController) Tier(domain string) SoftRateTier {
	if c == nil {
		return SoftRateNormal
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.domains[normalizeSoftDomain(domain)]
	if state == nil {
		return SoftRateNormal
	}
	return state.tier
}

// Spacing returns the minimum inter-request spacing for the current tier.
func (c *SoftRateController) Spacing(domain string) time.Duration {
	return softRateSpacingFor(c.Tier(domain))
}

// Wait blocks until the domain's soft-rate spacing allows another request, or
// until ctx is cancelled. It never refuses admission permanently: the slowest
// tier only lengthens the wait.
func (c *SoftRateController) Wait(ctx context.Context, domain string) error {
	if c == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	domain = normalizeSoftDomain(domain)

	for {
		c.mu.Lock()
		state := c.ensureLocked(domain)
		now := c.clock()
		waitUntil := state.nextAllowedAt
		if waitUntil.IsZero() || !waitUntil.After(now) {
			// Reserve the slot immediately so concurrent waiters serialize.
			state.lastReleaseAt = now
			state.nextAllowedAt = now.Add(softRateSpacingFor(state.tier))
			c.mu.Unlock()
			return nil
		}
		delay := waitUntil.Sub(now)
		c.mu.Unlock()

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
			// Loop to re-check under the lock in case another waiter advanced.
		}
	}
}

// ObserveSuccess records a live success and may recover one tier after a
// sustained streak. Soft rate never jumps more than one tier per success burst.
func (c *SoftRateController) ObserveSuccess(domain string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.ensureLocked(normalizeSoftDomain(domain))
	state.consecutiveOK++
	if state.consecutiveOK < softRateSuccessesToRecover {
		return
	}
	state.consecutiveOK = 0
	switch state.tier {
	case SoftRateSlow:
		state.tier = SoftRateCautious
	case SoftRateCautious:
		state.tier = SoftRateNormal
	default:
		state.tier = SoftRateNormal
	}
	// Refresh spacing from the recovered tier without inventing a hard block.
	now := c.clock()
	state.nextAllowedAt = now.Add(softRateSpacingFor(state.tier))
}

// ObserveAccessDenied records a real upstream 403/401 and degrades the tier by
// one step (normal→cautious→slow). The slow tier is the floor: traffic continues.
func (c *SoftRateController) ObserveAccessDenied(domain string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.ensureLocked(normalizeSoftDomain(domain))
	state.consecutiveOK = 0
	switch state.tier {
	case SoftRateNormal:
		state.tier = SoftRateCautious
	case SoftRateCautious:
		state.tier = SoftRateSlow
	default:
		state.tier = SoftRateSlow
	}
	now := c.clock()
	state.nextAllowedAt = now.Add(softRateSpacingFor(state.tier))
}

// ObserveTransientFailure records a retryable non-403 failure without fully
// flooring the tier. A single 5xx/timeout nudges toward cautious at most.
func (c *SoftRateController) ObserveTransientFailure(domain string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.ensureLocked(normalizeSoftDomain(domain))
	state.consecutiveOK = 0
	if state.tier == SoftRateNormal {
		state.tier = SoftRateCautious
		now := c.clock()
		state.nextAllowedAt = now.Add(softRateSpacingFor(state.tier))
	}
}

func (c *SoftRateController) ensureLocked(domain string) *softRateState {
	state := c.domains[domain]
	if state == nil {
		state = &softRateState{tier: SoftRateNormal}
		c.domains[domain] = state
	}
	return state
}

func (c *SoftRateController) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func softRateSpacingFor(tier SoftRateTier) time.Duration {
	switch tier {
	case SoftRateCautious:
		return softRateSpacingCautious
	case SoftRateSlow:
		return softRateSpacingSlow
	default:
		return softRateSpacingNormal
	}
}

func normalizeSoftDomain(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return "<unknown>"
	}
	return domain
}
