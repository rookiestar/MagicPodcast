package xyzvideo

import (
	"net/http"
	"testing"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
)

func TestParseEpisodeID(t *testing.T) {
	id, ok := ParseEpisodeID("https://www.xiaoyuzhoufm.com/episode/6a734c29ab3a91c24a1067fa?utm_source=rss")
	require.True(t, ok)
	require.Equal(t, "6a734c29ab3a91c24a1067fa", id)

	id, ok = ParseEpisodeID("https://web.xiaoyuzhoufm.com/episode/6a8cf80a1352af56ff3b7e2d/")
	require.True(t, ok)
	require.Equal(t, "6a8cf80a1352af56ff3b7e2d", id)

	_, ok = ParseEpisodeID("https://www.xiaoyuzhoufm.com/podcast/68981df29e7bcd326eb91d88")
	require.False(t, ok)
	_, ok = ParseEpisodeID("https://www.xiaoyuzhoufm.com/episode/6a8cf80a1352af56ff3b7e2d/comments")
	require.False(t, ok)
	_, ok = ParseEpisodeID("https://example.invalid/episode/6a734c29ab3a91c24a1067fa")
	require.False(t, ok)
	_, ok = ParseEpisodeID("javascript:alert(1)")
	require.False(t, ok)
}

func TestShouldProbe(t *testing.T) {
	link := "https://www.xiaoyuzhoufm.com/episode/6a734c29ab3a91c24a1067fa"
	require.True(t, ShouldProbe(link, "", false, true))
	require.True(t, ShouldProbe(link, models.VideoAvailabilityUnknown, false, false))
	require.True(t, ShouldProbe(link, "", false, false))
	require.False(t, ShouldProbe(link, models.VideoAvailabilityAvailable, false, false))
	require.False(t, ShouldProbe(link, models.VideoAvailabilityUnavailable, false, false))
	require.True(t, ShouldProbe(link, models.VideoAvailabilityAvailable, true, false))
	require.False(t, ShouldProbe("https://example.invalid/episodes/1", "", false, true))
}

func TestParsePlaybackResponse(t *testing.T) {
	hlsBody := []byte(`{"playback":{"master":{"url":"https://video.xyzcdn.net/episode-video/x/hls/preview/master.m3u8?auth_key=secret"}}}`)
	require.Equal(t, models.VideoAvailabilityAvailable, ParsePlaybackResponse(http.StatusOK, hlsBody))
	require.Equal(t, models.VideoAvailabilityUnavailable, ParsePlaybackResponse(http.StatusNotFound, []byte("Video playback not found")))
	require.Equal(t, models.VideoAvailabilityUnknown, ParsePlaybackResponse(http.StatusForbidden, nil))
	require.Equal(t, models.VideoAvailabilityUnknown, ParsePlaybackResponse(http.StatusGatewayTimeout, nil))
	require.Equal(t, models.VideoAvailabilityUnknown, ParsePlaybackResponse(0, nil))
}
