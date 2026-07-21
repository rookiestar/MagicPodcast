package feed

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/gofeed/rss"
)

// maxRefreshHintFloor caps any single upstream refresh hint so a hostile or
// misconfigured feed cannot use a giant <ttl> or max-age to effectively freeze
// a Feed out of refresh forever. The local MinRefreshInterval and the global
// refresh scheduling remain the ultimate authority; this just bounds the hint
// contribution. 24h mirrors the robots cache ceiling and is far above any sane
// feed publishing cadence.
const maxRefreshHintFloor = 24 * time.Hour

// RefreshHints captures the upstream-advised refresh signals learned from one
// successful Feed fetch: the RSS channel-level <ttl> (minutes) and the
// <skipHours>/<skipDays> windows, plus the HTTP cache lifetime derived from
// Cache-Control max-age (minus Age) and the Expires header. All fields are
// optional (zero = absent); the merger takes the most conservative (latest)
// next-allowed-refresh across them together with the local MinRefreshInterval.
//
// Hints are advisory ONLY. They can only ever lengthen the wait between
// refreshes (never shorten it below the local floor), and each contribution is
// capped at maxRefreshHintFloor. When no usable hint is present the merger
// falls back to the local MinRefreshInterval + MaxJitter semantics unchanged.
type RefreshHints struct {
	// TTL is the RSS <ttl> value as a duration. Zero when absent/invalid.
	TTL time.Duration
	// SkipHours are the hours-of-day (0-23, treated as UTC) the feed asks not to
	// be fetched. Empty when absent. RSS leaves the timezone ambiguous; UTC is
	// the conservative, deterministic default an operator can reason about.
	SkipHours map[int]struct{}
	// SkipDays are the weekdays the feed asks not to be fetched. Empty when absent.
	SkipDays map[time.Weekday]struct{}
	// CacheMaxAge is the freshness lifetime from Cache-Control max-age reduced
	// by any Age header (so a cached/aged response shortens the floor). Zero
	// when absent/invalid.
	CacheMaxAge time.Duration
	// ExpiresAt is the absolute HTTP Expires time. Zero when absent, in the past
	// at capture, or unparseable.
	ExpiresAt time.Time
	// LearnedAt is the instant these hints were captured (the fetch time). It is
	// the anchor for the duration-based floor and is always set by the Fetcher.
	LearnedAt time.Time
}

// Present reports whether any usable upstream hint was captured. When false the
// merger behaves exactly as the pre-hint local policy.
func (h RefreshHints) Present() bool {
	return h.TTL > 0 || len(h.SkipHours) > 0 || len(h.SkipDays) > 0 ||
		h.CacheMaxAge > 0 || !h.ExpiresAt.IsZero()
}

// HintsFromFetch extracts refresh hints from a parsed RSS channel and the HTTP
// cache headers of a successful fetch. now is the capture instant (also stored
// as LearnedAt). It never returns an error: any unparseable field is dropped so
// one bad hint cannot disable the others or the local floor.
func HintsFromFetch(rssFeed *rss.Feed, cacheControl, expires, age string, now time.Time) RefreshHints {
	hints := RefreshHints{LearnedAt: now}
	if rssFeed != nil {
		hints.TTL = parseTTLMinutes(rssFeed.TTL)
		hints.SkipHours = parseSkipHours(rssFeed.SkipHours)
		hints.SkipDays = parseSkipDays(rssFeed.SkipDays)
	}
	hints.CacheMaxAge = parseCacheMaxAge(cacheControl, age)
	hints.ExpiresAt = parseExpires(expires, now)
	return hints
}

// MinInterval returns the most conservative DURATION floor between refreshes,
// i.e. the max of the local policy floor and every usable hint contribution
// (TTL, Cache-Control max-age, and the residual Expires lifetime). Each hint is
// capped at maxRefreshHintFloor. Skip windows are time-of-day and handled
// separately by InSkipWindow because they do not map to a single duration.
func (h RefreshHints) MinInterval(policyFloor time.Duration) time.Duration {
	floor := policyFloor
	floor = maxDuration(floor, capHint(h.TTL))
	floor = maxDuration(floor, capHint(h.CacheMaxAge))
	if !h.ExpiresAt.IsZero() {
		// Residual lifetime relative to the capture instant. Already-expired at
		// capture contributes nothing (and never goes negative).
		residual := h.ExpiresAt.Sub(h.LearnedAt)
		floor = maxDuration(floor, capHint(residual))
	}
	return floor
}

// InSkipWindow reports whether now falls inside an RSS skipHours/skipDays
// window. now's hour is read in UTC (matching how HintsFromFetch stores hours).
func (h RefreshHints) InSkipWindow(now time.Time) bool {
	if len(h.SkipHours) == 0 && len(h.SkipDays) == 0 {
		return false
	}
	utc := now.UTC()
	if _, hourBlocked := h.SkipHours[utc.Hour()]; hourBlocked {
		return true
	}
	if _, dayBlocked := h.SkipDays[utc.Weekday()]; dayBlocked {
		return true
	}
	return false
}

// RefreshAllowed reports whether a new fetch is permitted at now given the
// learned hints and the local policy floor. It is false (i.e. the cached result
// should be served) when now is still within the merged duration floor OR now
// falls inside a skip window. Callers still require an actual cached/snapshot
// result to serve; with none, a fetch proceeds regardless.
func (h RefreshHints) RefreshAllowed(policyFloor time.Duration, now time.Time) bool {
	if h.InSkipWindow(now) {
		return false
	}
	if !h.LearnedAt.IsZero() && now.Sub(h.LearnedAt) < h.MinInterval(policyFloor) {
		return false
	}
	return true
}

func capHint(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	if d > maxRefreshHintFloor {
		return maxRefreshHintFloor
	}
	return d
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// parseTTLMinutes parses the RSS <ttl> (minutes) string. Non-positive or
// non-integer values yield zero.
func parseTTLMinutes(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	minutes, err := strconv.Atoi(raw)
	if err != nil || minutes <= 0 {
		return 0
	}
	return time.Duration(minutes) * time.Minute
}

// parseSkipHours parses <skipHours><hour> entries into a set of 0-23 hours.
// Out-of-range or non-integer entries are dropped.
func parseSkipHours(entries []string) map[int]struct{} {
	out := make(map[int]struct{})
	for _, raw := range entries {
		h, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || h < 0 || h > 23 {
			continue
		}
		out[h] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseSkipDays parses <skipDays><day> entries (English weekday names) into a
// set. Unrecognized names are dropped. Matching is case-insensitive.
func parseSkipDays(entries []string) map[time.Weekday]struct{} {
	out := make(map[time.Weekday]struct{})
	for _, raw := range entries {
		day := parseWeekday(strings.TrimSpace(raw))
		if day < 0 {
			continue
		}
		out[day] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseWeekday(name string) time.Weekday {
	switch strings.ToLower(name) {
	case "sunday":
		return time.Sunday
	case "monday":
		return time.Monday
	case "tuesday":
		return time.Tuesday
	case "wednesday":
		return time.Wednesday
	case "thursday":
		return time.Thursday
	case "friday":
		return time.Friday
	case "saturday":
		return time.Saturday
	default:
		return -1
	}
}

// parseCacheMaxAge extracts the freshness lifetime from a Cache-Control header
// (max-age or s-maxage, the shared-cache directive) reduced by the Age header.
// Non-positive or absent values yield zero. Age is clamped at the parsed
// max-age so a stale/oversized Age never produces a negative floor.
func parseCacheMaxAge(cacheControl, age string) time.Duration {
	maxAge := -1
	for _, directive := range strings.Split(cacheControl, ",") {
		directive = strings.TrimSpace(directive)
		if directive == "" {
			continue
		}
		if name, value, ok := splitCacheDirective(directive); ok {
			lower := strings.ToLower(name)
			if (lower == "s-maxage" || lower == "max-age") && value != "" {
				if seconds, err := strconv.Atoi(value); err == nil && seconds > maxAge {
					maxAge = seconds
				}
			}
		}
	}
	if maxAge <= 0 {
		return 0
	}
	lifetime := time.Duration(maxAge) * time.Second
	ageSeconds, _ := strconv.Atoi(strings.TrimSpace(age))
	if ageSeconds > 0 {
		aged := lifetime - time.Duration(ageSeconds)*time.Second
		if aged < 0 {
			return 0
		}
		return aged
	}
	return lifetime
}

func splitCacheDirective(directive string) (name, value string, ok bool) {
	idx := strings.IndexByte(directive, '=')
	if idx < 0 {
		return directive, "", true
	}
	return directive[:idx], strings.Trim(directive[idx+1:], `"`), true
}

// parseExpires parses an HTTP Expires date relative to now. A valid future date
// is returned; an absent, invalid, or already-past value yields the zero time
// (meaning "no hint"), matching HTTP semantics where Expires: 0 / past means
// "always stale" and thus contributes no refresh floor.
func parseExpires(raw string, now time.Time) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	t, err := http.ParseTime(raw)
	if err != nil {
		return time.Time{}
	}
	if !t.After(now) {
		return time.Time{}
	}
	return t.UTC()
}
