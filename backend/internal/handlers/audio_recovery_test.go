package handlers_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"magicpodcast/internal/database"
	"magicpodcast/internal/handlers"
	"magicpodcast/internal/models"
	"magicpodcast/internal/processing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type httpFakeDriveDownloader struct {
	mu      sync.Mutex
	payload []byte
	calls   int
}

func (f *httpFakeDriveDownloader) Download(
	_ context.Context,
	_ string,
	directory string,
	fileName string,
) error {
	f.mu.Lock()
	f.calls++
	payload := append([]byte(nil), f.payload...)
	f.mu.Unlock()
	return os.WriteFile(filepath.Join(directory, fileName), payload, 0o600)
}

func (f *httpFakeDriveDownloader) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type recoveryHTTPArtifactReader struct{}

func (recoveryHTTPArtifactReader) ReadText(
	_ context.Context,
	_ models.EpisodeArtifactSet,
	kind string,
) (processing.ArtifactContent, error) {
	if kind != "transcript" {
		return processing.ArtifactContent{}, processing.ErrInvalidArtifact
	}
	return processing.ArtifactContent{
		Kind:    "transcript",
		Content: "# 逐字稿\n\n恢复合同测试\n",
		SHA256:  strings.Repeat("t", 64),
		Segments: []processing.TranscriptSegment{{
			Order: 1, Speaker: "主持人", StartMS: 0, Text: "恢复合同测试",
		}},
	}, nil
}

func TestArtifactAudioRecoveryHTTPContractPreservesMediaContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio-recovery-handler.db")
	db, err := gorm.Open(sqlite.Open(path+"?_foreign_keys=on&_busy_timeout=5000"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.ApplyMigrations(db))

	now := time.Date(2026, 9, 1, 6, 0, 0, 0, time.UTC)
	podcast := models.Podcast{
		Title: "Recovery contract", FeedURL: "https://example.com/recovery.xml",
		XYZID: "recovery-contract", IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID: podcast.ID, Title: "Recovery contract episode",
		GUID: "recovery-contract-episode", MediumURL: "https://audio.example/recovery.mp3",
		Duration: 60, PublishedDate: now,
	}
	require.NoError(t, db.Create(&episode).Error)

	payload := []byte("ID3 HTTP recovery contract audio")
	digestBytes := sha256.Sum256(payload)
	digest := hex.EncodeToString(digestBytes[:])
	audioRoot := t.TempDir()
	audio, err := processing.NewDiskAudioStore(db, audioRoot)
	require.NoError(t, err)
	canonicalAudioRoot, err := filepath.EvalSymlinks(audioRoot)
	require.NoError(t, err)
	relativeAudioPath := filepath.Join("episodes", strconv.FormatUint(uint64(episode.ID), 10), "managed.mp3")
	audioPath := filepath.Join(canonicalAudioRoot, relativeAudioPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(audioPath), 0o700))
	require.NoError(t, os.Chmod(filepath.Dir(audioPath), 0o700))
	asset := models.EpisodeAudioAsset{
		EpisodeID: episode.ID, SourceDigest: strings.Repeat("s", 64),
		Status: models.EpisodeAudioAssetStatusReady, RelativePath: filepath.ToSlash(relativeAudioPath),
		SHA256: digest, SizeBytes: int64(len(payload)), DurationSeconds: 60,
		MediaType: "audio/mpeg", Extension: ".mp3", QueuedAt: now,
		ReadyAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(&asset).Error)
	run := models.EpisodeProcessingRun{
		EpisodeID: episode.ID, ProcessingKey: strings.Repeat("k", 64), AudioDigest: digest,
		PipelineVersion: processing.NativeMinutesPipelineVersion, TriggerSource: models.ProcessingTriggerManual,
		Status: models.ProcessingRunStatusCompleted, CurrentStep: processing.StepArtifactPublish,
		AttemptCount: 1, MaxAttempts: 3, RetryDeadlineAt: now.Add(24 * time.Hour),
		FinishedAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(&run).Error)
	checkpointState := []byte(fmt.Sprintf(
		`{"version":1,"phase":"transcript_stored","audio_digest":"%s","file_token":"boxcn_http_recovery_1234"}`,
		digest,
	))
	checkpointHash := sha256.Sum256(checkpointState)
	require.NoError(t, db.Create(&models.ProcessingCheckpoint{
		RunID: run.ID, Step: processing.StepTranscription, Adapter: "feishu-minutes-cli",
		AdapterVersion: "feishu-minutes-cli-v1", Status: processing.ExternalProgressCompleted,
		StateJSON: string(checkpointState), StateHash: hex.EncodeToString(checkpointHash[:]),
		CreatedAt: now, UpdatedAt: now,
	}).Error)
	artifact := models.EpisodeArtifactSet{
		RunID: run.ID, EpisodeID: episode.ID, PipelineVersion: processing.NativeMinutesPipelineVersion,
		RootPath: filepath.Join(t.TempDir(), "artifact"), ManifestPath: "manifest.json",
		ManifestSHA256: strings.Repeat("m", 64), AudioSHA256: digest,
		MinutesSummarySHA256: strings.Repeat("u", 64), TranscriptSHA256: strings.Repeat("t", 64),
		TranscriptTimelineSHA256: strings.Repeat("l", 64), NotesSHA256: strings.Repeat("n", 64),
		IsCurrent: true, CreatedAt: now,
	}
	require.NoError(t, db.Create(&artifact).Error)

	downloader := &httpFakeDriveDownloader{payload: payload}
	recovery, err := processing.NewAudioRecoveryStore(db, audio, downloader)
	require.NoError(t, err)
	service := processing.NewService(
		db,
		processing.WithArtifactReader(recoveryHTTPArtifactReader{}),
		processing.WithAudioPreparer(audio),
		processing.WithAudioRecovery(recovery),
	)
	handler := handlers.NewProcessingHandler(service, nil)
	router := gin.New()
	router.GET("/api/v1/artifact-sets/:id/:kind", handler.GetArtifactContent)
	router.POST("/api/v1/artifact-sets/:id/audio/recovery", handler.RecoverArtifactAudio)
	router.GET("/api/v1/artifact-sets/:id/audio", handler.GetArtifactAudio)
	router.HEAD("/api/v1/artifact-sets/:id/audio", handler.GetArtifactAudio)

	transcriptPath := fmt.Sprintf("/api/v1/artifact-sets/%d/transcript", artifact.ID)
	response := processingRequest(router, http.MethodGet, transcriptPath, "")
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"media_available":false`)
	require.Contains(t, response.Body.String(), `"recoverable":true`)
	require.NotContains(t, response.Body.String(), "boxcn_http_recovery_1234")
	require.NotContains(t, response.Body.String(), canonicalAudioRoot)
	require.NotContains(t, response.Body.String(), digest)

	recoveryPath := fmt.Sprintf("/api/v1/artifact-sets/%d/audio/recovery", artifact.ID)
	response = processingRequest(router, http.MethodPost, recoveryPath, "")
	require.Equal(t, http.StatusAccepted, response.Code)
	require.Contains(t, response.Body.String(), `"status":"queued"`)
	require.NotContains(t, response.Body.String(), "boxcn_http_recovery_1234")
	require.NotContains(t, response.Body.String(), canonicalAudioRoot)
	require.NotContains(t, response.Body.String(), digest)

	response = processingRequest(router, http.MethodPost, recoveryPath, "")
	require.Equal(t, http.StatusAccepted, response.Code)
	require.Contains(t, response.Body.String(), `"reused":true`)
	var recoveryCount int64
	require.NoError(t, db.Model(&models.EpisodeArtifactAudioRecovery{}).
		Where("artifact_set_id = ?", artifact.ID).Count(&recoveryCount).Error)
	require.Equal(t, int64(1), recoveryCount)

	ids, err := recovery.ListClaimable(context.Background(), 4)
	require.NoError(t, err)
	require.Len(t, ids, 1)
	claim, claimed, err := recovery.Claim(context.Background(), ids[0])
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, recovery.Recover(context.Background(), claim))

	response = processingRequest(router, http.MethodGet, transcriptPath, "")
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"media_available":true`)
	require.Contains(t, response.Body.String(), "恢复合同测试")
	require.NotContains(t, response.Body.String(), digest)

	mediaPath := fmt.Sprintf("/api/v1/artifact-sets/%d/audio", artifact.ID)
	response = processingRequest(router, http.MethodGet, mediaPath, "")
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, payload, response.Body.Bytes())
	require.Equal(t, "audio/mpeg", response.Header().Get("Content-Type"))
	require.Equal(t, "bytes", response.Header().Get("Accept-Ranges"))
	require.Equal(t, fmt.Sprint(len(payload)), response.Header().Get("Content-Length"))

	response = processingRequest(router, http.MethodHead, mediaPath, "")
	require.Equal(t, http.StatusOK, response.Code)
	require.Empty(t, response.Body.Bytes())
	require.Equal(t, fmt.Sprint(len(payload)), response.Header().Get("Content-Length"))

	rangeRequest := httptest.NewRequest(http.MethodGet, mediaPath, nil)
	rangeRequest.Header.Set("Range", "bytes=0-5")
	rangeRecorder := httptest.NewRecorder()
	router.ServeHTTP(rangeRecorder, rangeRequest)
	require.Equal(t, http.StatusPartialContent, rangeRecorder.Code)
	require.Equal(t, payload[:6], rangeRecorder.Body.Bytes())
	require.Equal(t, fmt.Sprintf("bytes 0-5/%d", len(payload)), rangeRecorder.Header().Get("Content-Range"))
	require.Equal(t, 1, downloader.Calls())
}
