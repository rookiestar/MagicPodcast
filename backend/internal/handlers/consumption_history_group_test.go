package handlers_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"magicpodcast/internal/models"
	"magicpodcast/internal/services"

	"github.com/stretchr/testify/require"
)

func TestConsumptionHandler_CompletionHistoryGroupsBeforePagingAndAfterSearch(t *testing.T) {
	db, router, a := setupConsumptionHandler(t)
	b := models.Podcast{Title: a.Title, FeedURL: "https://example.com/other.xml", XYZID: "other-history"}
	require.NoError(t, db.Create(&b).Error)
	now := time.Now().UTC().Add(-24 * time.Hour)
	add := func(podcastID uint, title string, at time.Time) uint {
		episode := createConsumptionHandlerEpisode(t, db, podcastID, title, now)
		require.NoError(t, db.Create(&models.EpisodeCompletion{EpisodeID: episode.ID, CompletedAt: at}).Error)
		return episode.ID
	}
	aNew := add(a.ID, "A newest", now)
	bNew := add(b.ID, "match B newest", now.Add(-time.Hour))
	aOld := add(a.ID, "match A old", now.Add(-10*time.Hour))
	bOld := add(b.ID, "match B old", now.Add(-4*time.Hour))
	expected := []uint{aNew, aOld}
	for i := 0; i < 52; i++ {
		expected = append(expected, add(a.ID, fmt.Sprintf("A archive %d", i), now.Add(-time.Duration(20+i)*time.Hour)))
	}
	expected = append(expected, bNew, bOld)
	read := func(path string) services.CompletionHistorySnapshot {
		response := performJSONRequest(t, router, http.MethodGet, path, "")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var result struct {
			Data services.CompletionHistorySnapshot `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		return result.Data
	}
	first := read("/api/v1/consumption/completions")
	require.Len(t, first.Items, 50)
	require.True(t, first.HasMore)
	second := read("/api/v1/consumption/completions?cursor=" + first.NextCursor)
	require.False(t, second.HasMore)
	require.Equal(t, expected, append(completionHandlerHistoryItemIDs(first.Items), completionHandlerHistoryItemIDs(second.Items)...))
	search := read("/api/v1/consumption/completions?q=match")
	require.Equal(t, []uint{bNew, bOld, aOld}, completionHandlerHistoryItemIDs(search.Items))
	require.Equal(t, int64(3), search.MatchCount)
	require.Equal(t, int64(56), search.TotalCount)
	legacy := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"episode_id":%d,"completed_at":"2026-09-19T12:00:00Z","query":""}`, aNew)))
	response := performJSONRequest(t, router, http.MethodGet, "/api/v1/consumption/completions?cursor="+legacy, "")
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "INVALID_CURSOR")
	invalidTime := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"episode_id":%d,"podcast_id":%d,"completed_at":"2026-09-19T12:00:00Z","group_completed_at":"not-a-date","query":""}`, aNew, a.ID)))
	invalidResponse := performJSONRequest(t, router, http.MethodGet, "/api/v1/consumption/completions?cursor="+invalidTime, "")
	require.Equal(t, http.StatusBadRequest, invalidResponse.Code)
	require.Contains(t, invalidResponse.Body.String(), "INVALID_CURSOR")

	response = performJSONRequest(t, router, http.MethodPut, fmt.Sprintf("/api/v1/consumption/episodes/%d/queue", bOld), `{"queue_state":"inbox"}`)
	require.Equal(t, http.StatusOK, response.Code)
	retained := read("/api/v1/consumption/completions?q=match")
	require.Equal(t, []uint{bNew, bOld, aOld}, completionHandlerHistoryItemIDs(retained.Items))
	require.True(t, retained.Items[1].CompletedAt.Equal(now.Add(-4*time.Hour)))
	response = performJSONRequest(t, router, http.MethodPut, fmt.Sprintf("/api/v1/consumption/episodes/%d/queue", bOld), `{"queue_state":"done"}`)
	require.Equal(t, http.StatusOK, response.Code)
	refreshed := read("/api/v1/consumption/completions")
	require.Equal(t, []uint{bOld, bNew}, completionHandlerHistoryItemIDs(refreshed.Items[:2]))
	require.Equal(t, int64(56), refreshed.TotalCount)
	// Same-time records use episode ID as the page tie-breaker.
	tieFirst := add(b.ID, "tie first", now)
	tieSecond := add(b.ID, "tie second", now)
	tied := read("/api/v1/consumption/completions?q=tie&limit=1")
	require.Equal(t, []uint{tieSecond}, completionHandlerHistoryItemIDs(tied.Items))
	tiedNext := read("/api/v1/consumption/completions?q=tie&limit=1&cursor=" + tied.NextCursor)
	require.Equal(t, []uint{tieFirst}, completionHandlerHistoryItemIDs(tiedNext.Items))
	require.False(t, tiedNext.HasMore)

}
