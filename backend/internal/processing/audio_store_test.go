package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDiskAudioStorePreparesResolvesAndReusesReadyAudio(t *testing.T) {
	db := openAudioStoreTestDB(t)
	body := []byte("ID3\x04managed-audio-payload")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.Header().Set("Content-Type", "audio/mpeg")
		_, _ = writer.Write(body)
	}))
	t.Cleanup(server.Close)

	root := filepath.Join(t.TempDir(), "managed-audio")
	store := newHTTPAudioStore(t, db, root, server, newMapAudioResolver(map[string][]net.IP{
		"audio.example": {net.ParseIP("8.8.8.8")},
	}))
	episode := createAudioEpisode(
		t,
		db,
		"http://audio.example/episode.mp3?token=TOP-SECRET",
		3600,
	)

	enqueued, err := store.Enqueue(context.Background(), episode.ID)
	require.NoError(t, err)
	require.False(t, enqueued.ReusedReady)
	require.Equal(t, models.EpisodeAudioAssetStatusQueued, enqueued.Asset.Status)
	require.Len(t, enqueued.Asset.SourceDigest, 64)

	claim, claimed, err := store.Claim(context.Background(), enqueued.Asset.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	ready, err := store.Prepare(context.Background(), claim)
	require.NoError(t, err)

	sum := sha256.Sum256(body)
	require.Equal(t, hex.EncodeToString(sum[:]), ready.SHA256)
	require.Equal(t, int64(len(body)), ready.SizeBytes)
	require.Equal(t, 3600, ready.DurationSeconds)
	require.Equal(t, "audio/mpeg", ready.MediaType)
	require.True(t, filepath.IsAbs(ready.Path))
	require.FileExists(t, ready.Path)
	require.Equal(t, os.FileMode(0o600), mustFileInfo(t, ready.Path).Mode().Perm())
	require.Equal(t, os.FileMode(0o700), mustFileInfo(t, root).Mode().Perm())
	require.Equal(t, os.FileMode(0o700), mustFileInfo(t, filepath.Dir(ready.Path)).Mode().Perm())

	resolved, err := store.ResolveReadyAudio(context.Background(), episode.ID)
	require.NoError(t, err)
	require.Equal(t, ready, resolved)
	reused, err := store.Enqueue(context.Background(), episode.ID)
	require.NoError(t, err)
	require.True(t, reused.ReusedReady)
	require.Equal(t, enqueued.Asset.ID, reused.Asset.ID)
	require.Equal(t, int32(1), requests.Load())

	var stored models.EpisodeAudioAsset
	require.NoError(t, db.First(&stored, enqueued.Asset.ID).Error)
	require.Equal(t, models.EpisodeAudioAssetStatusReady, stored.Status)
	require.NotEmpty(t, stored.RelativePath)
	require.False(t, filepath.IsAbs(stored.RelativePath))
	require.Empty(t, stored.ClaimToken)
	require.Nil(t, stored.ClaimExpiresAt)
	require.NotNil(t, stored.ReadyAt)
	serialized, err := json.Marshal(stored)
	require.NoError(t, err)
	require.NotContains(t, string(serialized), "source_digest")
	require.NotContains(t, string(serialized), stored.SourceDigest)
	require.NotContains(t, string(serialized), stored.RelativePath)
	require.NotContains(t, string(serialized), ready.Path)
	require.NotContains(t, string(serialized), episode.MediumURL)
}

func TestDiskAudioStoreBlocksDNSRebindingBeforeDial(t *testing.T) {
	db := openAudioStoreTestDB(t)
	resolver := &sequenceAudioResolver{answers: [][]net.IP{
		{net.ParseIP("8.8.8.8")},
		{net.ParseIP("127.0.0.1")},
	}}
	var dialed atomic.Bool
	store, err := newDiskAudioStore(
		db,
		t.TempDir(),
		withAudioResolver(resolver),
		withAudioDialIP(func(context.Context, string, string) (net.Conn, error) {
			dialed.Store(true)
			return nil, errors.New("unexpected dial")
		}),
	)
	require.NoError(t, err)
	episode := createAudioEpisode(t, db, "http://audio.example/rebind.mp3", 90)
	enqueued, err := store.Enqueue(context.Background(), episode.ID)
	require.NoError(t, err)
	claim, claimed, err := store.Claim(context.Background(), enqueued.Asset.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	_, err = store.Prepare(context.Background(), claim)
	requireAudioErrorCode(t, err, AudioErrorSourceBlocked)
	require.False(t, dialed.Load())
	require.GreaterOrEqual(t, resolver.callCount(), 2)
	assertFailedAudioAsset(t, db, enqueued.Asset.ID, AudioErrorSourceBlocked)
}

func TestDiskAudioStoreRejectsUnsafeSourceSyntaxBeforeQueueing(t *testing.T) {
	for _, testCase := range []struct {
		name string
		url  string
		code string
	}{
		{name: "non-http", url: "file:///tmp/audio.mp3", code: AudioErrorSourceInvalid},
		{name: "userinfo", url: "https://user:secret@audio.example/file.mp3", code: AudioErrorSourceBlocked},
		{name: "custom-port", url: "https://audio.example:8443/file.mp3", code: AudioErrorSourceBlocked},
		{name: "ipv4-literal", url: "http://127.0.0.1/file.mp3", code: AudioErrorSourceBlocked},
		{name: "ipv6-literal", url: "http://[::1]/file.mp3", code: AudioErrorSourceBlocked},
		{name: "unsupported-extension", url: "https://audio.example/file.exe", code: AudioErrorFormatUnsupported},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db := openAudioStoreTestDB(t)
			store, err := NewDiskAudioStore(db, t.TempDir())
			require.NoError(t, err)
			episode := createAudioEpisode(t, db, testCase.url, 60)

			_, err = store.Enqueue(context.Background(), episode.ID)
			requireAudioErrorCode(t, err, testCase.code)
			require.NotContains(t, err.Error(), testCase.url)
			var count int64
			require.NoError(t, db.Model(&models.EpisodeAudioAsset{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestDiskAudioStoreBlocksNonPublicDNSAnswersWithoutDialing(t *testing.T) {
	blocked := map[string]net.IP{
		"loopback-v4":    net.ParseIP("127.0.0.1"),
		"private-v4":     net.ParseIP("10.1.2.3"),
		"link-local-v4":  net.ParseIP("169.254.1.1"),
		"multicast-v4":   net.ParseIP("224.0.0.1"),
		"unspecified-v4": net.ParseIP("0.0.0.0"),
		"cgnat-v4":       net.ParseIP("100.64.1.1"),
		"loopback-v6":    net.ParseIP("::1"),
		"private-v6":     net.ParseIP("fc00::1"),
		"link-local-v6":  net.ParseIP("fe80::1"),
		"multicast-v6":   net.ParseIP("ff02::1"),
		"unspecified-v6": net.ParseIP("::"),
	}
	for name, ip := range blocked {
		t.Run(name, func(t *testing.T) {
			db := openAudioStoreTestDB(t)
			resolver := newMapAudioResolver(map[string][]net.IP{"blocked.example": {ip}})
			var dialed atomic.Bool
			store, err := newDiskAudioStore(
				db,
				t.TempDir(),
				withAudioResolver(resolver),
				withAudioDialIP(func(context.Context, string, string) (net.Conn, error) {
					dialed.Store(true)
					return nil, errors.New("unexpected dial")
				}),
			)
			require.NoError(t, err)
			episode := createAudioEpisode(t, db, "http://blocked.example/file.mp3", 60)
			enqueued, err := store.Enqueue(context.Background(), episode.ID)
			require.NoError(t, err)
			claim, claimed, err := store.Claim(context.Background(), enqueued.Asset.ID)
			require.NoError(t, err)
			require.True(t, claimed)

			_, err = store.Prepare(context.Background(), claim)
			requireAudioErrorCode(t, err, AudioErrorSourceBlocked)
			require.False(t, dialed.Load())
			assertFailedAudioAsset(t, db, enqueued.Asset.ID, AudioErrorSourceBlocked)
			assertNoManagedAudioFiles(t, store.root)
		})
	}
}

func TestDiskAudioStoreBlocksPrivateRedirectAndDoesNotLeakLocation(t *testing.T) {
	db := openAudioStoreTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(
			writer,
			request,
			"http://private.example/secret.mp3?token=TOP-SECRET",
			http.StatusFound,
		)
	}))
	t.Cleanup(server.Close)
	resolver := newMapAudioResolver(map[string][]net.IP{
		"audio.example":   {net.ParseIP("8.8.8.8")},
		"private.example": {net.ParseIP("10.0.0.7")},
	})
	store := newHTTPAudioStore(t, db, t.TempDir(), server, resolver)
	episode := createAudioEpisode(t, db, "http://audio.example/start.mp3", 90)
	enqueued, err := store.Enqueue(context.Background(), episode.ID)
	require.NoError(t, err)
	claim, claimed, err := store.Claim(context.Background(), enqueued.Asset.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	_, err = store.Prepare(context.Background(), claim)
	requireAudioErrorCode(t, err, AudioErrorRedirectBlocked)
	require.NotContains(t, err.Error(), "private.example")
	require.NotContains(t, err.Error(), "TOP-SECRET")
	assertFailedAudioAsset(t, db, enqueued.Asset.ID, AudioErrorRedirectBlocked)
	assertNoManagedAudioFiles(t, store.root)
}

func TestDiskAudioStoreLimitsRedirects(t *testing.T) {
	db := openAudioStoreTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "http://audio.example/loop.mp3", http.StatusFound)
	}))
	t.Cleanup(server.Close)
	store := newHTTPAudioStore(t, db, t.TempDir(), server, newMapAudioResolver(map[string][]net.IP{
		"audio.example": {net.ParseIP("8.8.8.8")},
	}))
	episode := createAudioEpisode(t, db, "http://audio.example/loop.mp3", 90)
	enqueued, err := store.Enqueue(context.Background(), episode.ID)
	require.NoError(t, err)
	claim, claimed, err := store.Claim(context.Background(), enqueued.Asset.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	_, err = store.Prepare(context.Background(), claim)
	requireAudioErrorCode(t, err, AudioErrorRedirectBlocked)
	assertFailedAudioAsset(t, db, enqueued.Asset.ID, AudioErrorRedirectBlocked)
}

func TestDiskAudioStoreRejectsContentLengthBeforeCreatingAFile(t *testing.T) {
	db := openAudioStoreTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "audio/mpeg")
		writer.Header().Set("Content-Length", "11")
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	store := newHTTPAudioStoreWithOptions(
		t,
		db,
		t.TempDir(),
		server,
		newMapAudioResolver(map[string][]net.IP{"audio.example": {net.ParseIP("8.8.8.8")}}),
		withAudioLimits(10, time.Second, 2*time.Second),
	)
	episode := createAudioEpisode(t, db, "http://audio.example/large.mp3", 90)
	enqueued, err := store.Enqueue(context.Background(), episode.ID)
	require.NoError(t, err)
	claim, claimed, err := store.Claim(context.Background(), enqueued.Asset.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	_, err = store.Prepare(context.Background(), claim)
	requireAudioErrorCode(t, err, AudioErrorTooLarge)
	assertNoManagedAudioFiles(t, store.root)
}

func TestDiskAudioStoreEnforcesStreamingLimitAndCleansPartialFile(t *testing.T) {
	db := openAudioStoreTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "audio/mpeg")
		writer.WriteHeader(http.StatusOK)
		writer.(http.Flusher).Flush()
		_, _ = writer.Write([]byte("12345678901"))
	}))
	t.Cleanup(server.Close)
	store := newHTTPAudioStoreWithOptions(
		t,
		db,
		t.TempDir(),
		server,
		newMapAudioResolver(map[string][]net.IP{"audio.example": {net.ParseIP("8.8.8.8")}}),
		withAudioLimits(10, time.Second, 2*time.Second),
	)
	episode := createAudioEpisode(t, db, "http://audio.example/stream.mp3", 90)
	enqueued, err := store.Enqueue(context.Background(), episode.ID)
	require.NoError(t, err)
	claim, claimed, err := store.Claim(context.Background(), enqueued.Asset.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	_, err = store.Prepare(context.Background(), claim)
	requireAudioErrorCode(t, err, AudioErrorTooLarge)
	assertNoManagedAudioFiles(t, store.root)
}

func TestDiskAudioStoreCleansPartialFileAfterTruncatedResponse(t *testing.T) {
	db := openAudioStoreTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "audio/mpeg")
		writer.Header().Set("Content-Length", "100")
		_, _ = writer.Write([]byte("short"))
	}))
	t.Cleanup(server.Close)
	store := newHTTPAudioStore(t, db, t.TempDir(), server, newMapAudioResolver(map[string][]net.IP{
		"audio.example": {net.ParseIP("8.8.8.8")},
	}))
	episode := createAudioEpisode(t, db, "http://audio.example/truncated.mp3", 90)
	enqueued, err := store.Enqueue(context.Background(), episode.ID)
	require.NoError(t, err)
	claim, claimed, err := store.Claim(context.Background(), enqueued.Asset.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	_, err = store.Prepare(context.Background(), claim)
	requireAudioErrorCode(t, err, AudioErrorDownloadFailed)
	assertNoManagedAudioFiles(t, store.root)
	assertFailedAudioAsset(t, db, enqueued.Asset.ID, AudioErrorDownloadFailed)
}

func TestDiskAudioStoreValidatesDurationAndContentType(t *testing.T) {
	t.Run("duration", func(t *testing.T) {
		for _, duration := range []int{0, -1, int(MaxManagedAudioDuration/time.Second) + 1} {
			db := openAudioStoreTestDB(t)
			store, err := NewDiskAudioStore(db, t.TempDir())
			require.NoError(t, err)
			episode := createAudioEpisode(t, db, "https://audio.example/file.mp3", duration)

			_, err = store.Enqueue(context.Background(), episode.ID)
			requireAudioErrorCode(t, err, AudioErrorDurationInvalid)
		}
	})

	t.Run("content-type-mismatch", func(t *testing.T) {
		db := openAudioStoreTestDB(t)
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "audio/mp4")
			_, _ = writer.Write([]byte("not-mp3"))
		}))
		t.Cleanup(server.Close)
		store := newHTTPAudioStore(t, db, t.TempDir(), server, newMapAudioResolver(map[string][]net.IP{
			"audio.example": {net.ParseIP("8.8.8.8")},
		}))
		episode := createAudioEpisode(t, db, "http://audio.example/file.mp3", 90)
		enqueued, err := store.Enqueue(context.Background(), episode.ID)
		require.NoError(t, err)
		claim, claimed, err := store.Claim(context.Background(), enqueued.Asset.ID)
		require.NoError(t, err)
		require.True(t, claimed)

		_, err = store.Prepare(context.Background(), claim)
		requireAudioErrorCode(t, err, AudioErrorContentTypeInvalid)
		assertNoManagedAudioFiles(t, store.root)
	})

	t.Run("format-whitelist", func(t *testing.T) {
		valid := map[string]string{
			"wav": "audio/wav", "mp3": "audio/mpeg", "m4a": "audio/mp4",
			"aac": "audio/aac", "ogg": "audio/ogg", "wma": "audio/x-ms-wma",
			"amr": "audio/amr", "avi": "video/x-msvideo", "wmv": "video/x-ms-wmv",
			"mov": "video/quicktime", "mp4": "video/mp4", "m4v": "video/mp4",
			"mpeg": "video/mpeg", "flv": "video/x-flv",
		}
		for extension, contentType := range valid {
			source, err := parseEpisodeAudioURL("https://audio.example/file." + extension)
			require.NoError(t, err)
			actualExtension, err := audioExtension(source)
			require.NoError(t, err)
			require.Equal(t, extension, actualExtension)
			actualContentType, err := validateAudioContentType(contentType+"; charset=binary", extension)
			require.NoError(t, err)
			require.Equal(t, contentType, actualContentType)
		}
		_, err := validateAudioContentType("application/octet-stream", "mp3")
		requireAudioErrorCode(t, err, AudioErrorContentTypeInvalid)
	})
}

func TestDiskAudioStoreDownloadTimeoutIsSafeAndDurable(t *testing.T) {
	db := openAudioStoreTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
			return
		case <-time.After(time.Second):
			writer.Header().Set("Content-Type", "audio/mpeg")
			_, _ = writer.Write([]byte("late"))
		}
	}))
	t.Cleanup(server.Close)
	store := newHTTPAudioStoreWithOptions(
		t,
		db,
		t.TempDir(),
		server,
		newMapAudioResolver(map[string][]net.IP{"audio.example": {net.ParseIP("8.8.8.8")}}),
		withAudioLimits(1024, 30*time.Millisecond, time.Second),
	)
	episode := createAudioEpisode(t, db, "http://audio.example/slow.mp3", 90)
	enqueued, err := store.Enqueue(context.Background(), episode.ID)
	require.NoError(t, err)
	claim, claimed, err := store.Claim(context.Background(), enqueued.Asset.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	_, err = store.Prepare(context.Background(), claim)
	requireAudioErrorCode(t, err, AudioErrorDownloadTimeout)
	assertFailedAudioAsset(t, db, enqueued.Asset.ID, AudioErrorDownloadTimeout)
	assertNoManagedAudioFiles(t, store.root)
}

func TestDiskAudioStoreCancellationRequeuesClaimForRestartRecovery(t *testing.T) {
	db := openAudioStoreTestDB(t)
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	t.Cleanup(server.Close)
	store := newHTTPAudioStoreWithOptions(
		t,
		db,
		t.TempDir(),
		server,
		newMapAudioResolver(map[string][]net.IP{"audio.example": {net.ParseIP("8.8.8.8")}}),
		withAudioLimits(1024, time.Second, 2*time.Second),
	)
	episode := createAudioEpisode(t, db, "http://audio.example/cancel.mp3", 90)
	enqueued, err := store.Enqueue(context.Background(), episode.ID)
	require.NoError(t, err)
	claim, claimed, err := store.Claim(context.Background(), enqueued.Asset.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, prepareErr := store.Prepare(ctx, claim)
		result <- prepareErr
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("audio request did not start")
	}
	cancel()
	select {
	case err = <-result:
	case <-time.After(time.Second):
		t.Fatal("audio preparation did not stop")
	}
	require.ErrorIs(t, err, context.Canceled)
	var asset models.EpisodeAudioAsset
	require.NoError(t, db.First(&asset, enqueued.Asset.ID).Error)
	require.Equal(t, models.EpisodeAudioAssetStatusQueued, asset.Status)
	require.Empty(t, asset.ClaimToken)
	require.Nil(t, asset.ClaimExpiresAt)
	claimable, err := store.ListClaimable(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, []uint{asset.ID}, claimable)
	assertNoManagedAudioFiles(t, store.root)
}

func TestDiskAudioStoreListsQueuedAndExpiredClaimsInStableOrder(t *testing.T) {
	db := openAudioStoreTestDB(t)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	store, err := newDiskAudioStore(
		db,
		t.TempDir(),
		withAudioClock(func() time.Time { return now }),
	)
	require.NoError(t, err)

	type candidate struct {
		id       uint
		queuedAt time.Time
		status   string
		expires  *time.Time
	}
	var candidates []candidate
	for index := range 6 {
		episode := createAudioEpisode(
			t,
			db,
			fmt.Sprintf("https://audio-%d.example/file.mp3", index),
			90,
		)
		enqueued, enqueueErr := store.Enqueue(context.Background(), episode.ID)
		require.NoError(t, enqueueErr)
		candidates = append(candidates, candidate{id: enqueued.Asset.ID})
	}
	expired := now.Add(-time.Second)
	future := now.Add(time.Minute)
	candidates[0] = candidate{
		id: candidates[0].id, queuedAt: now.Add(2 * time.Second),
		status: models.EpisodeAudioAssetStatusQueued,
	}
	candidates[1] = candidate{
		id: candidates[1].id, queuedAt: now.Add(time.Second),
		status: models.EpisodeAudioAssetStatusQueued,
	}
	candidates[2] = candidate{
		id: candidates[2].id, queuedAt: now,
		status: models.EpisodeAudioAssetStatusDownloading, expires: &expired,
	}
	candidates[3] = candidate{
		id: candidates[3].id, queuedAt: now.Add(-time.Second),
		status: models.EpisodeAudioAssetStatusDownloading, expires: &future,
	}
	candidates[4] = candidate{
		id: candidates[4].id, queuedAt: now.Add(-2 * time.Second),
		status: models.EpisodeAudioAssetStatusFailed,
	}
	candidates[5] = candidate{
		id: candidates[5].id, queuedAt: now,
		status: models.EpisodeAudioAssetStatusDownloading,
	}
	for _, item := range candidates {
		require.NoError(t, db.Model(&models.EpisodeAudioAsset{}).
			Where("id = ?", item.id).
			Updates(map[string]any{
				"queued_at":        item.queuedAt,
				"status":           item.status,
				"claim_token":      "abandoned",
				"claim_expires_at": item.expires,
			}).Error)
	}

	ids, err := store.ListClaimable(context.Background(), 3)
	require.NoError(t, err)
	require.Equal(t, []uint{candidates[2].id, candidates[5].id, candidates[1].id}, ids)
	_, err = store.ListClaimable(context.Background(), 0)
	requireAudioErrorCode(t, err, AudioErrorClaimLimitInvalid)
	_, err = store.ListClaimable(context.Background(), 1001)
	requireAudioErrorCode(t, err, AudioErrorClaimLimitInvalid)

	recovered, claimed, err := store.Claim(context.Background(), candidates[5].id)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NotEmpty(t, recovered.Token)
}

func TestDiskAudioStoreClaimIsAtomicAndExpiredClaimCanBeRecovered(t *testing.T) {
	db := openAudioStoreTestDB(t)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	var clockMu sync.Mutex
	clock := func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return now
	}
	store, err := newDiskAudioStore(
		db,
		t.TempDir(),
		withAudioClock(clock),
		withAudioLimits(1024, time.Second, 2*time.Second),
	)
	require.NoError(t, err)
	episode := createAudioEpisode(t, db, "https://audio.example/file.mp3", 90)
	enqueued, err := store.Enqueue(context.Background(), episode.ID)
	require.NoError(t, err)

	const workers = 12
	results := make(chan struct {
		claim   AudioClaim
		claimed bool
		err     error
	}, workers)
	for range workers {
		go func() {
			claim, claimed, claimErr := store.Claim(context.Background(), enqueued.Asset.ID)
			results <- struct {
				claim   AudioClaim
				claimed bool
				err     error
			}{claim: claim, claimed: claimed, err: claimErr}
		}()
	}
	var winner AudioClaim
	var claimedCount int
	for range workers {
		result := <-results
		require.NoError(t, result.err)
		if result.claimed {
			claimedCount++
			winner = result.claim
		}
	}
	require.Equal(t, 1, claimedCount)

	clockMu.Lock()
	now = now.Add(3 * time.Second)
	clockMu.Unlock()
	recovered, claimed, err := store.Claim(context.Background(), enqueued.Asset.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NotEqual(t, winner.Token, recovered.Token)
	_, err = store.Prepare(context.Background(), winner)
	requireAudioErrorCode(t, err, AudioErrorClaimLost)
}

func TestDiskAudioStoreFailsClaimWhenEpisodeSourceChanges(t *testing.T) {
	db := openAudioStoreTestDB(t)
	store, err := NewDiskAudioStore(db, t.TempDir())
	require.NoError(t, err)
	episode := createAudioEpisode(t, db, "https://audio.example/first.mp3", 90)
	enqueued, err := store.Enqueue(context.Background(), episode.ID)
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.Episode{}).
		Where("id = ?", episode.ID).
		Update("medium_url", "https://audio.example/second.mp3").Error)
	claim, claimed, err := store.Claim(context.Background(), enqueued.Asset.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	_, err = store.Prepare(context.Background(), claim)
	requireAudioErrorCode(t, err, AudioErrorSourceChanged)
	assertFailedAudioAsset(t, db, enqueued.Asset.ID, AudioErrorSourceChanged)
}

func TestDiskAudioStoreResolveRejectsEscapingAndInsecureReadyPaths(t *testing.T) {
	db := openAudioStoreTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "audio/mpeg")
		_, _ = writer.Write([]byte("managed-audio"))
	}))
	t.Cleanup(server.Close)
	root := filepath.Join(t.TempDir(), "managed")
	store := newHTTPAudioStore(t, db, root, server, newMapAudioResolver(map[string][]net.IP{
		"audio.example": {net.ParseIP("8.8.8.8")},
	}))
	episode := createAudioEpisode(t, db, "http://audio.example/path.mp3", 90)
	enqueued, err := store.Enqueue(context.Background(), episode.ID)
	require.NoError(t, err)
	claim, claimed, err := store.Claim(context.Background(), enqueued.Asset.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	ready, err := store.Prepare(context.Background(), claim)
	require.NoError(t, err)
	var stored models.EpisodeAudioAsset
	require.NoError(t, db.First(&stored, enqueued.Asset.ID).Error)

	outside := filepath.Join(filepath.Dir(root), "outside.mp3")
	require.NoError(t, os.WriteFile(outside, []byte("managed-audio"), 0o600))
	require.NoError(t, db.Model(&models.EpisodeAudioAsset{}).
		Where("id = ?", stored.ID).
		Update("relative_path", "../outside.mp3").Error)
	_, err = store.ResolveReadyAudio(context.Background(), episode.ID)
	requireAudioErrorCode(t, err, AudioErrorReadyFileInvalid)
	require.FileExists(t, outside)

	require.NoError(t, db.Model(&models.EpisodeAudioAsset{}).
		Where("id = ?", stored.ID).
		Update("relative_path", stored.RelativePath).Error)
	require.NoError(t, os.Chmod(ready.Path, 0o644))
	_, err = store.ResolveReadyAudio(context.Background(), episode.ID)
	requireAudioErrorCode(t, err, AudioErrorReadyFileInvalid)
}

func openAudioStoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "audio-store.db") +
		"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(
		&models.Podcast{},
		&models.Episode{},
		&models.EpisodeAudioAsset{},
	))
	require.NoError(t, db.Exec(models.ActiveEpisodeAudioAssetUniqueIndexSQL).Error)
	require.NoError(t, db.Exec(models.ReadyEpisodeAudioAssetUniqueIndexSQL).Error)
	return db
}

var audioEpisodeSequence atomic.Uint64

func createAudioEpisode(
	t *testing.T,
	db *gorm.DB,
	mediumURL string,
	duration int,
) models.Episode {
	t.Helper()
	sequence := audioEpisodeSequence.Add(1)
	podcast := models.Podcast{
		XYZID:   fmt.Sprintf("audio-test-%d", sequence),
		Title:   "Audio test",
		FeedURL: fmt.Sprintf("https://feeds.example/%d.xml", sequence),
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID: podcast.ID,
		Title:     "Managed audio",
		GUID:      fmt.Sprintf("audio-episode-%d", sequence),
		MediumURL: mediumURL,
		Duration:  duration,
	}
	require.NoError(t, db.Create(&episode).Error)
	return episode
}

type mapAudioResolver struct {
	mu        sync.Mutex
	addresses map[string][]net.IP
	calls     map[string]int
}

type sequenceAudioResolver struct {
	mu      sync.Mutex
	answers [][]net.IP
	calls   int
}

func (r *sequenceAudioResolver) LookupIPAddr(
	ctx context.Context,
	host string,
) ([]net.IPAddr, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	index := r.calls
	r.calls++
	if len(r.answers) == 0 {
		return nil, errors.New("host unavailable")
	}
	if index >= len(r.answers) {
		index = len(r.answers) - 1
	}
	result := make([]net.IPAddr, 0, len(r.answers[index]))
	for _, ip := range r.answers[index] {
		result = append(result, net.IPAddr{IP: append(net.IP(nil), ip...)})
	}
	return result, nil
}

func (r *sequenceAudioResolver) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func newMapAudioResolver(addresses map[string][]net.IP) *mapAudioResolver {
	return &mapAudioResolver{addresses: addresses, calls: make(map[string]int)}
}

func (r *mapAudioResolver) LookupIPAddr(
	ctx context.Context,
	host string,
) ([]net.IPAddr, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls[host]++
	ips, found := r.addresses[host]
	if !found {
		return nil, errors.New("host unavailable")
	}
	result := make([]net.IPAddr, 0, len(ips))
	for _, ip := range ips {
		result = append(result, net.IPAddr{IP: append(net.IP(nil), ip...)})
	}
	return result, nil
}

func newHTTPAudioStore(
	t *testing.T,
	db *gorm.DB,
	root string,
	server *httptest.Server,
	resolver audioDNSResolver,
) *DiskAudioStore {
	t.Helper()
	return newHTTPAudioStoreWithOptions(t, db, root, server, resolver)
}

func newHTTPAudioStoreWithOptions(
	t *testing.T,
	db *gorm.DB,
	root string,
	server *httptest.Server,
	resolver audioDNSResolver,
	options ...audioStoreOption,
) *DiskAudioStore {
	t.Helper()
	serverAddress := server.Listener.Addr().String()
	dialer := &net.Dialer{Timeout: time.Second}
	baseOptions := []audioStoreOption{
		withAudioResolver(resolver),
		withAudioDialIP(func(
			ctx context.Context,
			network string,
			_ string,
		) (net.Conn, error) {
			return dialer.DialContext(ctx, network, serverAddress)
		}),
	}
	baseOptions = append(baseOptions, options...)
	store, err := newDiskAudioStore(db, root, baseOptions...)
	require.NoError(t, err)
	return store
}

func requireAudioErrorCode(t *testing.T, err error, expected string) {
	t.Helper()
	require.Error(t, err)
	var storeError *AudioStoreError
	require.ErrorAs(t, err, &storeError)
	require.Equal(t, expected, storeError.Code)
	require.NotEmpty(t, storeError.SafeMessage)
}

func assertFailedAudioAsset(t *testing.T, db *gorm.DB, assetID uint, code string) {
	t.Helper()
	var asset models.EpisodeAudioAsset
	require.NoError(t, db.First(&asset, assetID).Error)
	require.Equal(t, models.EpisodeAudioAssetStatusFailed, asset.Status)
	require.Equal(t, code, asset.ErrorCode)
	require.NotEmpty(t, asset.ErrorMessage)
	require.Empty(t, asset.ClaimToken)
	require.Nil(t, asset.ClaimExpiresAt)
	require.NotNil(t, asset.FailedAt)
}

func assertNoManagedAudioFiles(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		require.NoError(t, err)
		if path != root {
			require.True(t, entry.IsDir(), "unexpected managed audio file: %s", filepath.Base(path))
		}
		return nil
	}))
}

func mustFileInfo(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info
}

func TestManagedAudioConstantsMatchContract(t *testing.T) {
	require.Equal(t, int64(6*1024*1024*1024), MaxManagedAudioBytes)
	require.Equal(t, 6*time.Hour, MaxManagedAudioDuration)
	require.Equal(
		t,
		[]string{"aac", "amr", "avi", "flv", "m4a", "m4v", "mov", "mp3", "mp4", "mpeg", "ogg", "wav", "wma", "wmv"},
		sortedAudioExtensions(),
	)
}

func sortedAudioExtensions() []string {
	extensions := make([]string, 0, len(audioContentTypesByExtension))
	for extension := range audioContentTypesByExtension {
		extensions = append(extensions, extension)
	}
	sort.Strings(extensions)
	return extensions
}
