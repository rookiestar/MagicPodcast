package feed

import (
	"testing"
	"time"

	"github.com/mmcdole/gofeed/rss"
	"github.com/stretchr/testify/assert"
)

func TestParseTTLMinutes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{"plain minutes", "60", time.Hour},
		{"zero is no hint", "0", 0},
		{"negative ignored", "-5", 0},
		{"garbage ignored", "soon", 0},
		{"empty ignored", "", 0},
		{"whitespace trimmed", "  30  ", 30 * time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseTTLMinutes(tc.raw))
		})
	}
}

func TestParseSkipHoursRangeFiltering(t *testing.T) {
	hours := parseSkipHours([]string{"0", "23", "24", "-1", "x", "12"})
	assert.Len(t, hours, 3)
	assert.Contains(t, hours, 0)
	assert.Contains(t, hours, 23)
	assert.Contains(t, hours, 12)
	// All-drop yields nil so Present() treats skipHours as absent.
	assert.Nil(t, parseSkipHours([]string{"24", "x"}))
}

func TestParseSkipDaysCaseInsensitive(t *testing.T) {
	days := parseSkipDays([]string{"Monday", "SUNDAY", "funday"})
	assert.Len(t, days, 2)
	assert.Contains(t, days, time.Monday)
	assert.Contains(t, days, time.Sunday)
}

func TestParseCacheMaxAgeWithAge(t *testing.T) {
	// max-age=600, Age=100 → residual 500s.
	assert.Equal(t, 500*time.Second, parseCacheMaxAge("max-age=600", "100"))
	// s-maxage wins over max-age for the shared cache.
	assert.Equal(t, 120*time.Second, parseCacheMaxAge("public, max-age=10, s-maxage=120", "0"))
	// Age >= max-age collapses to zero (already stale → no floor contribution).
	assert.Equal(t, time.Duration(0), parseCacheMaxAge("max-age=30", "60"))
	// No directives → zero.
	assert.Equal(t, time.Duration(0), parseCacheMaxAge("public", ""))
	// Quoted value tolerated.
	assert.Equal(t, 45*time.Second, parseCacheMaxAge(`max-age="45"`, ""))
}

func TestParseExpiresFutureAndPast(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	// 1h in the future.
	exp := parseExpires("Mon, 21 Jul 2026 13:00:00 GMT", now)
	assert.True(t, exp.Equal(now.Add(time.Hour)))
	// Past → zero (no hint).
	assert.True(t, parseExpires("Mon, 21 Jul 2026 11:00:00 GMT", now).IsZero())
	// Garbage → zero.
	assert.True(t, parseExpires("soon", now).IsZero())
	// Empty → zero.
	assert.True(t, parseExpires("", now).IsZero())
}

func TestHintsFromFetchCombinesAllSignals(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	rssFeed := &rss.Feed{
		TTL:       "60",
		SkipHours: []string{"3", "4"},
		SkipDays:  []string{"Saturday"},
	}
	hints := HintsFromFetch(rssFeed, "max-age=300", "Mon, 21 Jul 2026 13:00:00 GMT", "0", now)
	assert.Equal(t, time.Hour, hints.TTL)
	assert.Contains(t, hints.SkipHours, 3)
	assert.Contains(t, hints.SkipHours, 4)
	assert.Contains(t, hints.SkipDays, time.Saturday)
	assert.Equal(t, 5*time.Minute, hints.CacheMaxAge)
	assert.True(t, hints.ExpiresAt.Equal(now.Add(time.Hour)))
	assert.True(t, hints.LearnedAt.Equal(now))
	assert.True(t, hints.Present())
}

func TestHintsFromFetchNilRSSIsSafe(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	hints := HintsFromFetch(nil, "", "", "", now)
	assert.False(t, hints.Present())
	assert.Equal(t, now, hints.LearnedAt)
}

func TestMinIntervalTakesMostConservative(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	policyFloor := 5 * time.Minute

	// No hints → just the policy floor.
	assert.Equal(t, policyFloor, RefreshHints{LearnedAt: now}.MinInterval(policyFloor))

	// TTL (60m) dominates a 5m floor.
	hints := RefreshHints{LearnedAt: now, TTL: time.Hour}
	assert.Equal(t, time.Hour, hints.MinInterval(policyFloor))

	// Cache max-age (10m) dominates TTL (3m) but not the cap.
	hints = RefreshHints{LearnedAt: now, TTL: 3 * time.Minute, CacheMaxAge: 10 * time.Minute}
	assert.Equal(t, 10*time.Minute, hints.MinInterval(policyFloor))

	// Expires residual wins: captured at now, expires +2h.
	hints = RefreshHints{LearnedAt: now, ExpiresAt: now.Add(2 * time.Hour)}
	assert.Equal(t, 2*time.Hour, hints.MinInterval(policyFloor))

	// Already-expired at capture contributes nothing (residual <= 0).
	hints = RefreshHints{LearnedAt: now, ExpiresAt: now.Add(-time.Hour)}
	assert.Equal(t, policyFloor, hints.MinInterval(policyFloor))

	// Each hint capped at maxRefreshHintFloor.
	hints = RefreshHints{LearnedAt: now, TTL: 48 * time.Hour}
	assert.Equal(t, maxRefreshHintFloor, hints.MinInterval(policyFloor))
}

func TestInSkipWindowHoursAndDays(t *testing.T) {
	hints := RefreshHints{
		SkipHours: map[int]struct{}{3: {}},
		SkipDays:  map[time.Weekday]struct{}{time.Saturday: {}},
	}
	// 2026-07-21 is a Tuesday. 03:00 UTC is in skipHours.
	inHour := time.Date(2026, 7, 21, 3, 30, 0, 0, time.UTC)
	assert.True(t, hints.InSkipWindow(inHour))
	// 05:00 UTC Tuesday — clear.
	clear := time.Date(2026, 7, 21, 5, 0, 0, 0, time.UTC)
	assert.False(t, hints.InSkipWindow(clear))
	// 2026-07-25 is a Saturday at 12:00 — skipDays.
	saturday := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	assert.True(t, hints.InSkipWindow(saturday))
	// Empty hints never block.
	assert.False(t, RefreshHints{}.InSkipWindow(inHour))
}

func TestRefreshAllowedDurationFloor(t *testing.T) {
	learnedAt := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	hints := RefreshHints{LearnedAt: learnedAt, TTL: time.Hour}
	policyFloor := 5 * time.Minute

	// 30s after fetch: within the 60m TTL floor → not allowed.
	assert.False(t, hints.RefreshAllowed(policyFloor, learnedAt.Add(30*time.Second)))
	// Exactly at the floor boundary → allowed.
	assert.True(t, hints.RefreshAllowed(policyFloor, learnedAt.Add(time.Hour)))
	// Well past → allowed.
	assert.True(t, hints.RefreshAllowed(policyFloor, learnedAt.Add(2*time.Hour)))
}

func TestRefreshAllowedBlockedBySkipWindow(t *testing.T) {
	learnedAt := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	// No duration hint but a skipHours window at hour 3. Even long after fetch,
	// landing in the skip window blocks refresh.
	hints := RefreshHints{LearnedAt: learnedAt, SkipHours: map[int]struct{}{3: {}}}
	later := time.Date(2026, 7, 22, 3, 15, 0, 0, time.UTC) // next day 03:15
	assert.False(t, hints.RefreshAllowed(5*time.Minute, later))
	// Outside the window → allowed.
	clear := time.Date(2026, 7, 22, 5, 0, 0, 0, time.UTC)
	assert.True(t, hints.RefreshAllowed(5*time.Minute, clear))
}

func TestRefreshAllowedNoHintsFallsBackToPolicyFloor(t *testing.T) {
	learnedAt := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	hints := RefreshHints{LearnedAt: learnedAt} // Present()==false
	policyFloor := 10 * time.Minute
	assert.False(t, hints.RefreshAllowed(policyFloor, learnedAt.Add(5*time.Minute)))
	assert.True(t, hints.RefreshAllowed(policyFloor, learnedAt.Add(11*time.Minute)))
}

func TestRefreshAllowedCrossesDaylightBoundaries(t *testing.T) {
	// skipHours around midnight (23 and 0) must block on both sides of the day
	// boundary under the injected UTC clock.
	learnedAt := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	hints := RefreshHints{LearnedAt: learnedAt, SkipHours: map[int]struct{}{23: {}, 0: {}}}
	before := time.Date(2026, 7, 22, 23, 30, 0, 0, time.UTC) // 23:30 next day
	mid := time.Date(2026, 7, 23, 0, 30, 0, 0, time.UTC)     // 00:30
	after := time.Date(2026, 7, 23, 1, 30, 0, 0, time.UTC)   // 01:30 clear
	assert.False(t, hints.RefreshAllowed(time.Hour, before))
	assert.False(t, hints.RefreshAllowed(time.Hour, mid))
	assert.True(t, hints.RefreshAllowed(time.Hour, after))
}
