package feed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
)

const (
	// verifiedTengineUAAclHeader is the bounded signal confirmed by the
	// same-egress experiment. No arbitrary response header is persisted or
	// copied into diagnostics.
	verifiedTengineUAAclHeader = "X-Tengine-Error"
	verifiedTengineUAAclValue  = "denied by ua acl = blacklist"
)

// BatchAccessGate is an in-memory policy gate for one workflow batch. Its key
// contains only a target domain and a one-way User-Agent fingerprint, never
// the raw User-Agent. Persistence and recovery are intentionally left for the
// later cross-job ticket.
type BatchAccessGate struct {
	mu      sync.RWMutex
	blocked map[string]struct{}
}

type batchAccessGateContextKey struct{}

// NewBatchAccessGate creates an empty gate for one workflow batch.
func NewBatchAccessGate() *BatchAccessGate {
	return &BatchAccessGate{blocked: make(map[string]struct{})}
}

// WithBatchAccessGate scopes the gate to all primary Fetcher calls made by a
// workflow batch. Alternative source calls deliberately use the same context
// but are ignored by Fetcher when checking this gate.
func WithBatchAccessGate(ctx context.Context, gate *BatchAccessGate) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if gate == nil {
		return ctx
	}
	return context.WithValue(ctx, batchAccessGateContextKey{}, gate)
}

func batchAccessGateFromContext(ctx context.Context) *BatchAccessGate {
	if ctx == nil {
		return nil
	}
	gate, _ := ctx.Value(batchAccessGateContextKey{}).(*BatchAccessGate)
	return gate
}

func (g *BatchAccessGate) gateKey(domain, userAgentFingerprint string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	userAgentFingerprint = strings.ToLower(strings.TrimSpace(userAgentFingerprint))
	if domain == "" || userAgentFingerprint == "" {
		return ""
	}
	return domain + "|" + userAgentFingerprint
}

// Block records a direct UA ACL refusal for this domain/fingerprint pair.
func (g *BatchAccessGate) Block(domain, userAgentFingerprint string) {
	if g == nil {
		return
	}
	key := g.gateKey(domain, userAgentFingerprint)
	if key == "" {
		return
	}
	g.mu.Lock()
	if g.blocked == nil {
		g.blocked = make(map[string]struct{})
	}
	g.blocked[key] = struct{}{}
	g.mu.Unlock()
}

// IsBlocked reports whether a primary request is already suppressed in this
// batch. It is safe for concurrent workflow workers.
func (g *BatchAccessGate) IsBlocked(domain, userAgentFingerprint string) bool {
	if g == nil {
		return false
	}
	key := g.gateKey(domain, userAgentFingerprint)
	if key == "" {
		return false
	}
	g.mu.RLock()
	_, blocked := g.blocked[key]
	g.mu.RUnlock()
	return blocked
}

// UserAgentFingerprint is a one-way digest used to key access policy state.
// Callers must not persist or log the raw User-Agent in place of this value.
func UserAgentFingerprint(userAgent string) string {
	sum := sha256.Sum256([]byte(userAgent))
	return hex.EncodeToString(sum[:])
}

// hasExplicitUserAgentACLSignal accepts only the verified status/header/value
// combination. Case and whitespace are normalized; similar text, extra
// suffixes, arbitrary headers, and non-401/403 statuses do not match.
func hasExplicitUserAgentACLSignal(status int, headers http.Header) bool {
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		return false
	}
	value := strings.ToLower(strings.Join(strings.Fields(headers.Get(verifiedTengineUAAclHeader)), " "))
	return value == verifiedTengineUAAclValue
}
