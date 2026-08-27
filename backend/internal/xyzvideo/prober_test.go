package xyzvideo

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
)

func TestProberMapsPlaybackStatusAndDiscardsHLSBody(t *testing.T) {
	var seenBody atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/episodes/6a734c29ab3a91c24a1067fa/video-playback", r.URL.Path)
		require.Equal(t, defaultUserAgent, r.Header.Get("User-Agent"))
		require.Empty(t, r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "application/json")
		_, err := io.WriteString(w, `{"playback":{"master":{"url":"https://video.xyzcdn.net/episode-video/x/hls/preview/master.m3u8?auth_key=secret"}}}`)
		require.NoError(t, err)
		seenBody.Store(true)
	}))
	t.Cleanup(server.Close)

	prober := NewProber(ProberConfig{BaseURL: server.URL, Getter: NewHTTPGetter(server.Client(), defaultUserAgent)})
	outcome := prober.Probe(context.Background(), "6a734c29ab3a91c24a1067fa")
	require.True(t, seenBody.Load())
	require.Equal(t, models.VideoAvailabilityAvailable, outcome.Availability)
	require.False(t, outcome.HaltBatch)
}

func TestProberMaps404UnavailableAnd403UnknownHalt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/episodes/audio-eid/video-playback" {
			http.Error(w, "Video playback not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)
	prober := NewProber(ProberConfig{BaseURL: server.URL, Getter: NewHTTPGetter(server.Client(), "")})

	missing := prober.Probe(context.Background(), "audio-eid")
	require.Equal(t, models.VideoAvailabilityUnavailable, missing.Availability)
	require.False(t, missing.HaltBatch)

	denied := prober.Probe(context.Background(), "video-eid")
	require.Equal(t, models.VideoAvailabilityUnknown, denied.Availability)
	require.True(t, denied.HaltBatch)
}

func TestProberTimeoutStaysUnknownAndHalts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := server.Client()
	client.Timeout = 20 * time.Millisecond
	prober := NewProber(ProberConfig{BaseURL: server.URL, Getter: NewHTTPGetter(client, "")})
	outcome := prober.Probe(context.Background(), "6a734c29ab3a91c24a1067fa")
	require.Equal(t, models.VideoAvailabilityUnknown, outcome.Availability)
	require.True(t, outcome.HaltBatch)
}

type statusErrorGetter struct {
	status int
	body   []byte
	err    error
}

func (g statusErrorGetter) Get(context.Context, string) (int, []byte, error) {
	return g.status, g.body, g.err
}

func TestProberReadErrorStaysUnknownForTerminalStatus(t *testing.T) {
	hlsBody := []byte(`{"playback":{"master":{"url":"https://video.xyzcdn.net/x/master.m3u8?auth_key=secret"}}}`)
	for _, test := range []struct {
		name   string
		status int
	}{
		{name: "ok", status: http.StatusOK},
		{name: "not found", status: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			prober := NewProber(ProberConfig{
				BaseURL: "https://www.xiaoyuzhoufm.com",
				Getter: statusErrorGetter{
					status: test.status,
					body:   hlsBody,
					err:    io.ErrUnexpectedEOF,
				},
			})

			outcome := prober.Probe(context.Background(), "6a734c29ab3a91c24a1067fa")
			require.Equal(t, models.VideoAvailabilityUnknown, outcome.Availability)
			require.True(t, outcome.HaltBatch)
		})
	}
}

type recordingSlotter struct {
	acquired string
	status   int
	err      error
}

func (s *recordingSlotter) AcquireDomainSlot(ctx context.Context, rawURL string) (func(), error) {
	s.acquired = rawURL
	return func() {}, nil
}

func (s *recordingSlotter) ObserveDomainProbe(rawURL string, status int, err error) {
	s.status = status
	s.err = err
	_ = rawURL
}

func TestProberUsesSharedSlotWithoutReturningHLS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"playback":{"master":{"url":"https://video.xyzcdn.net/x/master.m3u8?auth_key=secret"}}}`)
	}))
	t.Cleanup(server.Close)
	slotter := &recordingSlotter{}
	prober := NewProber(ProberConfig{
		BaseURL: server.URL,
		Getter:  NewHTTPGetter(server.Client(), ""),
		Slotter: slotter,
	})
	outcome := prober.Probe(context.Background(), "6a734c29ab3a91c24a1067fa")
	require.Equal(t, models.VideoAvailabilityAvailable, outcome.Availability)
	require.Contains(t, slotter.acquired, "/api/episodes/6a734c29ab3a91c24a1067fa/video-playback")
	require.Equal(t, http.StatusOK, slotter.status)
	require.NoError(t, slotter.err)
}
