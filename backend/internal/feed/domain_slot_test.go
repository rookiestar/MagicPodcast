package feed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSharedQueueDomainMapsXiaoyuzhouWebOntoFeedHost(t *testing.T) {
	require.Equal(t, XiaoyuzhouFeedDomain, SharedQueueDomain("https://www.xiaoyuzhoufm.com/api/episodes/x/video-playback"))
	require.Equal(t, XiaoyuzhouFeedDomain, SharedQueueDomain("https://web.xiaoyuzhoufm.com/episode/x"))
	require.Equal(t, XiaoyuzhouFeedDomain, SharedQueueDomain("https://feed.xyzfm.space/rss"))
	require.Equal(t, "example.com", SharedQueueDomain("https://example.com/feed.xml"))
}

func TestAcquireDomainSlotDoesNotPersistLastGood(t *testing.T) {
	var inFlight atomic.Int32
	var maxInFlight atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := inFlight.Add(1)
		for {
			prev := maxInFlight.Load()
			if current <= prev || maxInFlight.CompareAndSwap(prev, current) {
				break
			}
		}
		time.Sleep(40 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"playback":{"master":{"url":"https://video.xyzcdn.net/x/master.m3u8?auth_key=secret"}}}`))
		inFlight.Add(-1)
	}))
	t.Cleanup(server.Close)

	coordinator := NewCoordinator(DefaultCoordinatorConfig())
	start := make(chan struct{})
	done := make(chan struct{})
	for i := 0; i < 3; i++ {
		go func() {
			<-start
			release, err := coordinator.AcquireDomainSlot(context.Background(), "https://www.xiaoyuzhoufm.com/api/episodes/x/video-playback")
			require.NoError(t, err)
			req, err := http.NewRequest(http.MethodGet, server.URL, nil)
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()
			release()
			done <- struct{}{}
		}()
	}
	close(start)
	for i := 0; i < 3; i++ {
		<-done
	}
	require.Equal(t, int32(1), maxInFlight.Load())

	snapshot, ok := coordinator.LastGood(context.Background(), "https://www.xiaoyuzhoufm.com/api/episodes/x/video-playback", nil)
	require.False(t, ok)
	require.Nil(t, snapshot)
}
