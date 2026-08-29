package processing

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

const (
	MaxManagedAudioBytes    int64 = 6 * 1024 * 1024 * 1024
	MaxManagedAudioDuration       = 6 * time.Hour

	defaultAudioDownloadTimeout = 30 * time.Minute
	defaultAudioClaimTTL        = 35 * time.Minute
	defaultAudioMaxRedirects    = 5
)

const (
	AudioErrorAssetNotFound      = "audio_asset_not_found"
	AudioErrorClaimLimitInvalid  = "audio_claim_limit_invalid"
	AudioErrorNotReady           = "audio_not_ready"
	AudioErrorClaimLost          = "audio_claim_lost"
	AudioErrorSourceMissing      = "audio_source_missing"
	AudioErrorSourceInvalid      = "audio_source_invalid"
	AudioErrorSourceBlocked      = "audio_source_blocked"
	AudioErrorSourceChanged      = "audio_source_changed"
	AudioErrorSourceUnavailable  = "audio_source_unavailable"
	AudioErrorDurationInvalid    = "audio_duration_invalid"
	AudioErrorFormatUnsupported  = "audio_format_unsupported"
	AudioErrorContentTypeInvalid = "audio_content_type_invalid"
	AudioErrorRedirectBlocked    = "audio_redirect_blocked"
	AudioErrorHTTPStatus         = "audio_http_status"
	AudioErrorTooLarge           = "audio_too_large"
	AudioErrorEmpty              = "audio_empty"
	AudioErrorDownloadTimeout    = "audio_download_timeout"
	AudioErrorDownloadFailed     = "audio_download_failed"
	AudioErrorStorageFailed      = "audio_storage_failed"
	AudioErrorReadyFileInvalid   = "audio_ready_file_invalid"
)

// ReadyAudio is the process-local handoff to transcription and processing.
// Path is an absolute path verified to remain inside the managed audio root.
// It is deliberately absent from EpisodeAudioAsset's JSON representation.
type ReadyAudio struct {
	Path            string
	SHA256          string
	SizeBytes       int64
	DurationSeconds int
	MediaType       string
}

type AudioEnqueueResult struct {
	Asset       models.EpisodeAudioAsset
	ReusedReady bool
}

// AudioClaim proves ownership of one bounded preparation attempt.
// Token is secret process state and must not be logged or serialized.
type AudioClaim struct {
	AssetID   uint   `json:"-"`
	EpisodeID uint   `json:"-"`
	Token     string `json:"-"`
}

type AudioPreparer interface {
	Enqueue(context.Context, uint) (AudioEnqueueResult, error)
	ListClaimable(context.Context, int) ([]uint, error)
	Claim(context.Context, uint) (AudioClaim, bool, error)
	Prepare(context.Context, AudioClaim) (ReadyAudio, error)
	GetReady(context.Context, uint) (models.EpisodeAudioAsset, error)
	ResolveReadyAudio(context.Context, uint) (ReadyAudio, error)
}

// AudioStoreError carries a stable code and a source-safe message. The
// original URL, response URL, and managed path are never included.
type AudioStoreError struct {
	Code        string
	SafeMessage string
	Retryable   bool
}

func (e *AudioStoreError) Error() string {
	if e == nil {
		return ""
	}
	return e.SafeMessage
}

func newAudioStoreError(code, message string, retryable bool) *AudioStoreError {
	return &AudioStoreError{Code: code, SafeMessage: message, Retryable: retryable}
}

type audioDNSResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type audioStoreOption func(*DiskAudioStore)

func withAudioResolver(resolver audioDNSResolver) audioStoreOption {
	return func(store *DiskAudioStore) {
		if resolver != nil {
			store.resolver = resolver
		}
	}
}

func withAudioDialIP(
	dial func(context.Context, string, string) (net.Conn, error),
) audioStoreOption {
	return func(store *DiskAudioStore) {
		if dial != nil {
			store.dialIP = dial
		}
	}
}

func withAudioLimits(maxBytes int64, timeout, claimTTL time.Duration) audioStoreOption {
	return func(store *DiskAudioStore) {
		if maxBytes > 0 {
			store.maxBytes = maxBytes
		}
		if timeout > 0 {
			store.timeout = timeout
		}
		if claimTTL > 0 {
			store.claimTTL = claimTTL
		}
	}
}

func withAudioClock(now func() time.Time) audioStoreOption {
	return func(store *DiskAudioStore) {
		if now != nil {
			store.now = now
		}
	}
}

type DiskAudioStore struct {
	db           *gorm.DB
	root         string
	resolver     audioDNSResolver
	dialIP       func(context.Context, string, string) (net.Conn, error)
	maxBytes     int64
	timeout      time.Duration
	claimTTL     time.Duration
	maxRedirects int
	now          func() time.Time
	client       *http.Client
}

func NewDiskAudioStore(db *gorm.DB, root string) (*DiskAudioStore, error) {
	return newDiskAudioStore(db, root)
}

func newDiskAudioStore(
	db *gorm.DB,
	root string,
	options ...audioStoreOption,
) (*DiskAudioStore, error) {
	if db == nil {
		return nil, newAudioStoreError(
			AudioErrorStorageFailed,
			"managed audio storage is unavailable",
			true,
		)
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, newAudioStoreError(
			AudioErrorStorageFailed,
			"managed audio storage is unavailable",
			false,
		)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, newAudioStoreError(
			AudioErrorStorageFailed,
			"managed audio storage is unavailable",
			false,
		)
	}
	if err := ensureProtectedDirectory(absolute); err != nil {
		return nil, newAudioStoreError(
			AudioErrorStorageFailed,
			"managed audio storage is unavailable",
			true,
		)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, newAudioStoreError(
			AudioErrorStorageFailed,
			"managed audio storage is unavailable",
			false,
		)
	}

	netDialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	store := &DiskAudioStore{
		db:           db,
		root:         filepath.Clean(canonical),
		resolver:     net.DefaultResolver,
		dialIP:       netDialer.DialContext,
		maxBytes:     MaxManagedAudioBytes,
		timeout:      defaultAudioDownloadTimeout,
		claimTTL:     defaultAudioClaimTTL,
		maxRedirects: defaultAudioMaxRedirects,
		now:          time.Now,
	}
	for _, option := range options {
		option(store)
	}
	if store.claimTTL <= store.timeout {
		store.claimTTL = store.timeout + 5*time.Minute
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           store.dialContext,
		ForceAttemptHTTP2:     true,
		DisableCompression:    true,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       30 * time.Second,
	}
	store.client = &http.Client{
		Transport: transport,
		Timeout:   store.timeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) > store.maxRedirects {
				return newAudioStoreError(
					AudioErrorRedirectBlocked,
					"audio source exceeded the redirect limit",
					false,
				)
			}
			if _, err := store.validateRemoteURL(request.Context(), request.URL); err != nil {
				return newAudioStoreError(
					AudioErrorRedirectBlocked,
					"audio source redirect was blocked",
					false,
				)
			}
			return nil
		},
	}
	return store, nil
}

func (s *DiskAudioStore) Enqueue(
	ctx context.Context,
	episodeID uint,
) (AudioEnqueueResult, error) {
	if err := ctx.Err(); err != nil {
		return AudioEnqueueResult{}, err
	}
	episode, sourceDigest, err := s.loadAndValidateEpisode(ctx, episodeID)
	if err != nil {
		return AudioEnqueueResult{}, err
	}

	if ready, found, findErr := s.findReadyAsset(ctx, episodeID, sourceDigest); findErr != nil {
		return AudioEnqueueResult{}, findErr
	} else if found {
		if _, resolveErr := s.resolveReadyAsset(ready); resolveErr == nil {
			return AudioEnqueueResult{Asset: ready, ReusedReady: true}, nil
		}
		if err := s.invalidateReadyAsset(ctx, ready.ID); err != nil {
			return AudioEnqueueResult{}, err
		}
	}

	var active models.EpisodeAudioAsset
	err = s.db.WithContext(ctx).
		Where("episode_id = ? AND status IN ?", episodeID, []string{
			models.EpisodeAudioAssetStatusQueued,
			models.EpisodeAudioAssetStatusDownloading,
		}).
		Order("id DESC").
		First(&active).Error
	if err == nil {
		return AudioEnqueueResult{Asset: active}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return AudioEnqueueResult{}, newAudioStoreError(
			AudioErrorStorageFailed,
			"managed audio state is unavailable",
			true,
		)
	}

	now := s.now().UTC()
	asset := models.EpisodeAudioAsset{
		EpisodeID:       episode.ID,
		SourceDigest:    sourceDigest,
		Status:          models.EpisodeAudioAssetStatusQueued,
		DurationSeconds: episode.Duration,
		QueuedAt:        now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.db.WithContext(ctx).Create(&asset).Error; err == nil {
		return AudioEnqueueResult{Asset: asset}, nil
	}

	// A concurrent enqueue may have won either partial unique index.
	if ready, found, findErr := s.findReadyAsset(ctx, episodeID, sourceDigest); findErr == nil && found {
		if _, resolveErr := s.resolveReadyAsset(ready); resolveErr == nil {
			return AudioEnqueueResult{Asset: ready, ReusedReady: true}, nil
		}
	}
	err = s.db.WithContext(ctx).
		Where("episode_id = ? AND status IN ?", episodeID, []string{
			models.EpisodeAudioAssetStatusQueued,
			models.EpisodeAudioAssetStatusDownloading,
		}).
		Order("id DESC").
		First(&active).Error
	if err == nil {
		return AudioEnqueueResult{Asset: active}, nil
	}
	return AudioEnqueueResult{}, newAudioStoreError(
		AudioErrorStorageFailed,
		"managed audio state could not be queued",
		true,
	)
}

func (s *DiskAudioStore) ListClaimable(
	ctx context.Context,
	limit int,
) ([]uint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 1000 {
		return nil, newAudioStoreError(
			AudioErrorClaimLimitInvalid,
			"managed audio claim batch limit must be between 1 and 1000",
			false,
		)
	}
	now := s.now().UTC()
	ids := make([]uint, 0, limit)
	err := s.db.WithContext(ctx).
		Model(&models.EpisodeAudioAsset{}).
		Where(
			"status = ? OR (status = ? AND (claim_expires_at IS NULL OR claim_expires_at <= ?))",
			models.EpisodeAudioAssetStatusQueued,
			models.EpisodeAudioAssetStatusDownloading,
			now,
		).
		Order("queued_at ASC").
		Order("id ASC").
		Limit(limit).
		Pluck("id", &ids).Error
	if err != nil {
		return nil, newAudioStoreError(
			AudioErrorStorageFailed,
			"claimable managed audio state is unavailable",
			true,
		)
	}
	return ids, nil
}

func (s *DiskAudioStore) Claim(
	ctx context.Context,
	assetID uint,
) (AudioClaim, bool, error) {
	if err := ctx.Err(); err != nil {
		return AudioClaim{}, false, err
	}
	if assetID == 0 {
		return AudioClaim{}, false, newAudioStoreError(
			AudioErrorAssetNotFound,
			"managed audio asset was not found",
			false,
		)
	}
	token, err := randomAudioClaimToken()
	if err != nil {
		return AudioClaim{}, false, newAudioStoreError(
			AudioErrorStorageFailed,
			"managed audio claim could not be created",
			true,
		)
	}
	now := s.now().UTC()
	expiresAt := now.Add(s.claimTTL)
	update := s.db.WithContext(ctx).
		Model(&models.EpisodeAudioAsset{}).
		Where(
			"id = ? AND (status = ? OR (status = ? AND (claim_expires_at IS NULL OR claim_expires_at <= ?)))",
			assetID,
			models.EpisodeAudioAssetStatusQueued,
			models.EpisodeAudioAssetStatusDownloading,
			now,
		).
		Updates(map[string]any{
			"status":           models.EpisodeAudioAssetStatusDownloading,
			"claim_token":      token,
			"claim_expires_at": expiresAt,
			"downloading_at":   now,
			"error_code":       "",
			"error_message":    "",
			"failed_at":        nil,
			"updated_at":       now,
		})
	if update.Error != nil {
		return AudioClaim{}, false, newAudioStoreError(
			AudioErrorStorageFailed,
			"managed audio claim could not be stored",
			true,
		)
	}
	if update.RowsAffected == 0 {
		var existing models.EpisodeAudioAsset
		err := s.db.WithContext(ctx).First(&existing, assetID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AudioClaim{}, false, newAudioStoreError(
				AudioErrorAssetNotFound,
				"managed audio asset was not found",
				false,
			)
		}
		if err != nil {
			return AudioClaim{}, false, newAudioStoreError(
				AudioErrorStorageFailed,
				"managed audio state is unavailable",
				true,
			)
		}
		return AudioClaim{}, false, nil
	}

	var claimed models.EpisodeAudioAsset
	if err := s.db.WithContext(ctx).
		Where("id = ? AND claim_token = ?", assetID, token).
		First(&claimed).Error; err != nil {
		return AudioClaim{}, false, newAudioStoreError(
			AudioErrorClaimLost,
			"managed audio claim was lost",
			true,
		)
	}
	return AudioClaim{
		AssetID:   claimed.ID,
		EpisodeID: claimed.EpisodeID,
		Token:     token,
	}, true, nil
}

func (s *DiskAudioStore) Prepare(
	ctx context.Context,
	claim AudioClaim,
) (ReadyAudio, error) {
	if claim.AssetID == 0 || claim.EpisodeID == 0 || len(claim.Token) != sha256.Size*2 {
		return ReadyAudio{}, newAudioStoreError(
			AudioErrorClaimLost,
			"managed audio claim is invalid",
			false,
		)
	}
	if err := ctx.Err(); err != nil {
		return ReadyAudio{}, s.releasePreparationClaim(ctx, claim, err)
	}
	var asset models.EpisodeAudioAsset
	err := s.db.WithContext(ctx).
		Where(
			"id = ? AND episode_id = ? AND status = ? AND claim_token = ?",
			claim.AssetID,
			claim.EpisodeID,
			models.EpisodeAudioAssetStatusDownloading,
			claim.Token,
		).
		First(&asset).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ReadyAudio{}, newAudioStoreError(
			AudioErrorClaimLost,
			"managed audio claim was lost",
			true,
		)
	}
	if err != nil {
		return ReadyAudio{}, newAudioStoreError(
			AudioErrorStorageFailed,
			"managed audio state is unavailable",
			true,
		)
	}

	episode, sourceDigest, err := s.loadAndValidateEpisode(ctx, asset.EpisodeID)
	if err != nil {
		return ReadyAudio{}, s.failPreparation(ctx, claim, err)
	}
	if sourceDigest != asset.SourceDigest {
		return ReadyAudio{}, s.failPreparation(
			ctx,
			claim,
			newAudioStoreError(
				AudioErrorSourceChanged,
				"episode audio source changed before preparation",
				false,
			),
		)
	}
	sourceURL, err := parseEpisodeAudioURL(episode.MediumURL)
	if err != nil {
		return ReadyAudio{}, s.failPreparation(ctx, claim, err)
	}
	if _, err := s.validateRemoteURL(ctx, sourceURL); err != nil {
		return ReadyAudio{}, s.failPreparation(ctx, claim, err)
	}

	downloadContext, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(downloadContext, http.MethodGet, sourceURL.String(), nil)
	if err != nil {
		return ReadyAudio{}, s.failPreparation(
			ctx,
			claim,
			newAudioStoreError(
				AudioErrorSourceInvalid,
				"episode audio source is invalid",
				false,
			),
		)
	}
	request.Header.Set("Accept", strings.Join(allowedAudioMediaTypes(), ", "))
	request.Header.Set("User-Agent", "MagicPodcast/managed-audio")

	response, err := s.client.Do(request)
	if err != nil {
		return ReadyAudio{}, s.failPreparation(ctx, claim, classifyAudioRequestError(err))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ReadyAudio{}, s.failPreparation(
			ctx,
			claim,
			newAudioStoreError(
				AudioErrorHTTPStatus,
				"audio source returned an unsupported response status",
				response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests,
			),
		)
	}
	extension, err := audioExtension(response.Request.URL)
	if err != nil {
		return ReadyAudio{}, s.failPreparation(ctx, claim, err)
	}
	mediaType, err := validateAudioContentType(response.Header.Get("Content-Type"), extension)
	if err != nil {
		return ReadyAudio{}, s.failPreparation(ctx, claim, err)
	}
	if response.ContentLength > s.maxBytes {
		return ReadyAudio{}, s.failPreparation(
			ctx,
			claim,
			newAudioStoreError(
				AudioErrorTooLarge,
				"episode audio exceeds the managed size limit",
				false,
			),
		)
	}

	destinationDirectory, err := s.episodeDirectory(asset.EpisodeID)
	if err != nil {
		return ReadyAudio{}, s.failPreparation(ctx, claim, err)
	}
	temporary, err := os.CreateTemp(
		destinationDirectory,
		fmt.Sprintf(".asset-%d-*", asset.ID),
	)
	if err != nil {
		return ReadyAudio{}, s.failPreparation(
			ctx,
			claim,
			newAudioStoreError(
				AudioErrorStorageFailed,
				"managed audio file could not be created",
				true,
			),
		)
	}
	temporaryPath := temporary.Name()
	keepFile := false
	defer func() {
		_ = temporary.Close()
		if !keepFile {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return ReadyAudio{}, s.failPreparation(
			ctx,
			claim,
			newAudioStoreError(
				AudioErrorStorageFailed,
				"managed audio file could not be protected",
				true,
			),
		)
	}

	hasher := sha256.New()
	written, copyErr := io.Copy(
		io.MultiWriter(temporary, hasher),
		io.LimitReader(response.Body, s.maxBytes+1),
	)
	if copyErr != nil {
		return ReadyAudio{}, s.failPreparation(
			ctx,
			claim,
			classifyAudioRequestError(copyErr),
		)
	}
	if written > s.maxBytes {
		return ReadyAudio{}, s.failPreparation(
			ctx,
			claim,
			newAudioStoreError(
				AudioErrorTooLarge,
				"episode audio exceeds the managed size limit",
				false,
			),
		)
	}
	if written == 0 {
		return ReadyAudio{}, s.failPreparation(
			ctx,
			claim,
			newAudioStoreError(
				AudioErrorEmpty,
				"episode audio response was empty",
				true,
			),
		)
	}
	if response.ContentLength >= 0 && written != response.ContentLength {
		return ReadyAudio{}, s.failPreparation(
			ctx,
			claim,
			newAudioStoreError(
				AudioErrorDownloadFailed,
				"episode audio download was incomplete",
				true,
			),
		)
	}
	if err := temporary.Close(); err != nil {
		return ReadyAudio{}, s.failPreparation(
			ctx,
			claim,
			newAudioStoreError(
				AudioErrorStorageFailed,
				"managed audio file could not be finalized",
				true,
			),
		)
	}

	relativePath := filepath.Join(
		"episodes",
		strconv.FormatUint(uint64(asset.EpisodeID), 10),
		fmt.Sprintf("asset-%d-%s.%s", asset.ID, claim.Token[:16], extension),
	)
	finalPath, err := s.managedPath(relativePath)
	if err != nil {
		return ReadyAudio{}, s.failPreparation(ctx, claim, err)
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return ReadyAudio{}, s.failPreparation(
			ctx,
			claim,
			newAudioStoreError(
				AudioErrorStorageFailed,
				"managed audio file could not be published",
				true,
			),
		)
	}
	temporaryPath = finalPath
	digest := hex.EncodeToString(hasher.Sum(nil))
	now := s.now().UTC()
	update := s.db.WithContext(ctx).
		Model(&models.EpisodeAudioAsset{}).
		Where(
			"id = ? AND status = ? AND claim_token = ?",
			asset.ID,
			models.EpisodeAudioAssetStatusDownloading,
			claim.Token,
		).
		Updates(map[string]any{
			"status":           models.EpisodeAudioAssetStatusReady,
			"relative_path":    filepath.ToSlash(relativePath),
			"sha256":           digest,
			"size_bytes":       written,
			"duration_seconds": episode.Duration,
			"media_type":       mediaType,
			"extension":        extension,
			"error_code":       "",
			"error_message":    "",
			"claim_token":      "",
			"claim_expires_at": nil,
			"ready_at":         now,
			"failed_at":        nil,
			"updated_at":       now,
		})
	if update.Error != nil || update.RowsAffected != 1 {
		_ = os.Remove(finalPath)
		code := AudioErrorStorageFailed
		message := "managed audio state could not be finalized"
		if update.Error == nil {
			code = AudioErrorClaimLost
			message = "managed audio claim was lost"
		}
		return ReadyAudio{}, newAudioStoreError(code, message, true)
	}
	keepFile = true
	return ReadyAudio{
		Path:            finalPath,
		SHA256:          digest,
		SizeBytes:       written,
		DurationSeconds: episode.Duration,
		MediaType:       mediaType,
	}, nil
}

func (s *DiskAudioStore) GetReady(
	ctx context.Context,
	episodeID uint,
) (models.EpisodeAudioAsset, error) {
	_, sourceDigest, err := s.loadAndValidateEpisode(ctx, episodeID)
	if err != nil {
		return models.EpisodeAudioAsset{}, err
	}
	asset, found, err := s.findReadyAsset(ctx, episodeID, sourceDigest)
	if err != nil {
		return models.EpisodeAudioAsset{}, err
	}
	if !found {
		return models.EpisodeAudioAsset{}, newAudioStoreError(
			AudioErrorNotReady,
			"managed episode audio is not ready",
			true,
		)
	}
	if _, err := s.resolveReadyAsset(asset); err != nil {
		return models.EpisodeAudioAsset{}, err
	}
	return asset, nil
}

func (s *DiskAudioStore) ResolveReadyAudio(
	ctx context.Context,
	episodeID uint,
) (ReadyAudio, error) {
	asset, err := s.GetReady(ctx, episodeID)
	if err != nil {
		return ReadyAudio{}, err
	}
	return s.resolveReadyAsset(asset)
}

func (s *DiskAudioStore) loadAndValidateEpisode(
	ctx context.Context,
	episodeID uint,
) (models.Episode, string, error) {
	if episodeID == 0 {
		return models.Episode{}, "", newAudioStoreError(
			AudioErrorAssetNotFound,
			"episode was not found",
			false,
		)
	}
	var episode models.Episode
	err := s.db.WithContext(ctx).First(&episode, episodeID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.Episode{}, "", newAudioStoreError(
			AudioErrorAssetNotFound,
			"episode was not found",
			false,
		)
	}
	if err != nil {
		return models.Episode{}, "", newAudioStoreError(
			AudioErrorStorageFailed,
			"episode audio state is unavailable",
			true,
		)
	}
	if episode.Duration <= 0 ||
		episode.Duration > int(MaxManagedAudioDuration/time.Second) {
		return models.Episode{}, "", newAudioStoreError(
			AudioErrorDurationInvalid,
			"episode duration must be between one second and six hours",
			false,
		)
	}
	sourceURL, err := parseEpisodeAudioURL(episode.MediumURL)
	if err != nil {
		return models.Episode{}, "", err
	}
	if _, err := audioExtension(sourceURL); err != nil {
		return models.Episode{}, "", err
	}
	sourceURL.Fragment = ""
	sum := sha256.Sum256([]byte(sourceURL.String()))
	return episode, hex.EncodeToString(sum[:]), nil
}

func parseEpisodeAudioURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, newAudioStoreError(
			AudioErrorSourceMissing,
			"episode audio source is missing",
			false,
		)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.Opaque != "" {
		return nil, newAudioStoreError(
			AudioErrorSourceInvalid,
			"episode audio source is invalid",
			false,
		)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, newAudioStoreError(
			AudioErrorSourceInvalid,
			"episode audio source must use HTTP or HTTPS",
			false,
		)
	}
	parsed.Scheme = scheme
	if parsed.User != nil {
		return nil, newAudioStoreError(
			AudioErrorSourceBlocked,
			"episode audio source credentials are not allowed",
			false,
		)
	}
	host := parsed.Hostname()
	if host == "" || net.ParseIP(strings.Trim(host, "[]")) != nil {
		return nil, newAudioStoreError(
			AudioErrorSourceBlocked,
			"episode audio source host is not allowed",
			false,
		)
	}
	if port := parsed.Port(); port != "" {
		expected := "80"
		if scheme == "https" {
			expected = "443"
		}
		if port != expected {
			return nil, newAudioStoreError(
				AudioErrorSourceBlocked,
				"episode audio source port is not allowed",
				false,
			)
		}
	}
	return parsed, nil
}

func (s *DiskAudioStore) validateRemoteURL(
	ctx context.Context,
	candidate *url.URL,
) ([]net.IP, error) {
	parsed, err := parseEpisodeAudioURL(candidate.String())
	if err != nil {
		return nil, err
	}
	if _, err := audioExtension(parsed); err != nil {
		return nil, err
	}
	return s.safeHostIPs(ctx, parsed.Hostname())
}

func (s *DiskAudioStore) safeHostIPs(
	ctx context.Context,
	host string,
) ([]net.IP, error) {
	addresses, err := s.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, newAudioStoreError(
			AudioErrorSourceUnavailable,
			"episode audio source could not be resolved",
			true,
		)
	}
	if len(addresses) == 0 {
		return nil, newAudioStoreError(
			AudioErrorSourceUnavailable,
			"episode audio source did not resolve to an address",
			true,
		)
	}
	ips := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		if !isPublicAudioIP(address.IP) {
			return nil, newAudioStoreError(
				AudioErrorSourceBlocked,
				"episode audio source resolved to a blocked network",
				false,
			)
		}
		ips = append(ips, append(net.IP(nil), address.IP...))
	}
	return ips, nil
}

func (s *DiskAudioStore) dialContext(
	ctx context.Context,
	network string,
	address string,
) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || port == "" {
		return nil, newAudioStoreError(
			AudioErrorSourceInvalid,
			"episode audio source address is invalid",
			false,
		)
	}
	if net.ParseIP(strings.Trim(host, "[]")) != nil {
		return nil, newAudioStoreError(
			AudioErrorSourceBlocked,
			"episode audio source host is not allowed",
			false,
		)
	}
	ips, err := s.safeHostIPs(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ip := range ips {
		connection, dialErr := s.dialIP(
			ctx,
			network,
			net.JoinHostPort(ip.String(), port),
		)
		if dialErr == nil {
			return connection, nil
		}
		lastErr = dialErr
	}
	_ = lastErr
	return nil, newAudioStoreError(
		AudioErrorSourceUnavailable,
		"episode audio source could not be reached",
		true,
	)
}

func isPublicAudioIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	if address.IsUnspecified() ||
		address.IsLoopback() ||
		address.IsPrivate() ||
		address.IsLinkLocalUnicast() ||
		address.IsLinkLocalMulticast() ||
		address.IsMulticast() {
		return false
	}
	cgnat := netip.MustParsePrefix("100.64.0.0/10")
	return !cgnat.Contains(address)
}

func audioExtension(sourceURL *url.URL) (string, error) {
	extension := strings.TrimPrefix(
		strings.ToLower(filepath.Ext(sourceURL.Path)),
		".",
	)
	if _, found := audioContentTypesByExtension[extension]; !found {
		return "", newAudioStoreError(
			AudioErrorFormatUnsupported,
			"episode audio URL has an unsupported format",
			false,
		)
	}
	return extension, nil
}

var audioContentTypesByExtension = map[string]map[string]struct{}{
	"wav": {
		"audio/wav": {}, "audio/wave": {}, "audio/x-wav": {}, "audio/vnd.wave": {},
	},
	"mp3": {
		"audio/mpeg": {}, "audio/mp3": {}, "audio/x-mp3": {},
	},
	"m4a": {
		"audio/mp4": {}, "audio/x-m4a": {},
	},
	"aac": {
		"audio/aac": {}, "audio/x-aac": {},
	},
	"ogg": {
		"audio/ogg": {}, "application/ogg": {}, "video/ogg": {},
	},
	"wma": {
		"audio/x-ms-wma": {}, "video/x-ms-asf": {},
	},
	"amr": {
		"audio/amr": {}, "audio/amr-wb": {},
	},
	"avi": {
		"video/x-msvideo": {}, "video/avi": {},
	},
	"wmv": {
		"video/x-ms-wmv": {},
	},
	"mov": {
		"video/quicktime": {},
	},
	"mp4": {
		"audio/mp4": {}, "video/mp4": {},
	},
	"m4v": {
		"video/mp4": {}, "video/x-m4v": {},
	},
	"mpeg": {
		"audio/mpeg": {}, "video/mpeg": {},
	},
	"flv": {
		"video/x-flv": {},
	},
}

func allowedAudioMediaTypes() []string {
	seen := make(map[string]struct{})
	var mediaTypes []string
	for _, byMediaType := range audioContentTypesByExtension {
		for mediaType := range byMediaType {
			if _, found := seen[mediaType]; found {
				continue
			}
			seen[mediaType] = struct{}{}
			mediaTypes = append(mediaTypes, mediaType)
		}
	}
	return mediaTypes
}

func validateAudioContentType(header string, extension string) (string, error) {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(header))
	if err != nil || mediaType == "" {
		return "", newAudioStoreError(
			AudioErrorContentTypeInvalid,
			"audio response Content-Type is missing or invalid",
			false,
		)
	}
	mediaType = strings.ToLower(mediaType)
	allowed, found := audioContentTypesByExtension[extension]
	if !found {
		return "", newAudioStoreError(
			AudioErrorFormatUnsupported,
			"episode audio format is unsupported",
			false,
		)
	}
	if _, found := allowed[mediaType]; !found {
		return "", newAudioStoreError(
			AudioErrorContentTypeInvalid,
			"audio response Content-Type does not match the URL format",
			false,
		)
	}
	return mediaType, nil
}

func classifyAudioRequestError(err error) error {
	if err == nil {
		return nil
	}
	var storeError *AudioStoreError
	if errors.As(err, &storeError) {
		return storeError
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return newAudioStoreError(
			AudioErrorDownloadTimeout,
			"episode audio download timed out",
			true,
		)
	}
	return newAudioStoreError(
		AudioErrorDownloadFailed,
		"episode audio download failed",
		true,
	)
}

func (s *DiskAudioStore) findReadyAsset(
	ctx context.Context,
	episodeID uint,
	sourceDigest string,
) (models.EpisodeAudioAsset, bool, error) {
	var asset models.EpisodeAudioAsset
	err := s.db.WithContext(ctx).
		Where(
			"episode_id = ? AND source_digest = ? AND status = ?",
			episodeID,
			sourceDigest,
			models.EpisodeAudioAssetStatusReady,
		).
		Order("id DESC").
		First(&asset).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.EpisodeAudioAsset{}, false, nil
	}
	if err != nil {
		return models.EpisodeAudioAsset{}, false, newAudioStoreError(
			AudioErrorStorageFailed,
			"managed audio state is unavailable",
			true,
		)
	}
	return asset, true, nil
}

func (s *DiskAudioStore) resolveReadyAsset(
	asset models.EpisodeAudioAsset,
) (ReadyAudio, error) {
	extension := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(asset.Extension)), ".")
	mediaType := strings.ToLower(strings.TrimSpace(asset.MediaType))
	allowedMediaTypes, extensionAllowed := audioContentTypesByExtension[extension]
	_, mediaTypeAllowed := allowedMediaTypes[mediaType]
	if asset.Status != models.EpisodeAudioAssetStatusReady ||
		len(asset.SHA256) != sha256.Size*2 ||
		asset.SizeBytes <= 0 ||
		asset.DurationSeconds <= 0 ||
		asset.DurationSeconds > int(MaxManagedAudioDuration/time.Second) ||
		!extensionAllowed ||
		!mediaTypeAllowed {
		return ReadyAudio{}, newAudioStoreError(
			AudioErrorReadyFileInvalid,
			"managed episode audio metadata is invalid",
			false,
		)
	}
	if _, err := hex.DecodeString(asset.SHA256); err != nil {
		return ReadyAudio{}, newAudioStoreError(
			AudioErrorReadyFileInvalid,
			"managed episode audio digest is invalid",
			false,
		)
	}
	path, err := s.managedPath(filepath.FromSlash(asset.RelativePath))
	if err != nil {
		return ReadyAudio{}, err
	}
	info, err := os.Lstat(path)
	if err != nil ||
		!info.Mode().IsRegular() ||
		info.Mode()&os.ModeSymlink != 0 ||
		info.Mode().Perm() != 0o600 ||
		info.Size() != asset.SizeBytes {
		return ReadyAudio{}, newAudioStoreError(
			AudioErrorReadyFileInvalid,
			"managed episode audio file is unavailable",
			true,
		)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || filepath.Clean(canonical) != filepath.Clean(path) {
		return ReadyAudio{}, newAudioStoreError(
			AudioErrorReadyFileInvalid,
			"managed episode audio file is unavailable",
			false,
		)
	}
	return ReadyAudio{
		Path:            path,
		SHA256:          asset.SHA256,
		SizeBytes:       asset.SizeBytes,
		DurationSeconds: asset.DurationSeconds,
		MediaType:       mediaType,
	}, nil
}

func (s *DiskAudioStore) invalidateReadyAsset(
	ctx context.Context,
	assetID uint,
) error {
	now := s.now().UTC()
	update := s.db.WithContext(ctx).
		Model(&models.EpisodeAudioAsset{}).
		Where("id = ? AND status = ?", assetID, models.EpisodeAudioAssetStatusReady).
		Updates(map[string]any{
			"status":        models.EpisodeAudioAssetStatusFailed,
			"relative_path": "",
			"error_code":    AudioErrorReadyFileInvalid,
			"error_message": "managed episode audio file is unavailable",
			"failed_at":     now,
			"updated_at":    now,
		})
	if update.Error != nil {
		return newAudioStoreError(
			AudioErrorStorageFailed,
			"invalid managed audio state could not be recorded",
			true,
		)
	}
	return nil
}

func (s *DiskAudioStore) failPreparation(
	ctx context.Context,
	claim AudioClaim,
	cause error,
) error {
	if contextError := ctx.Err(); contextError != nil {
		return s.releasePreparationClaim(ctx, claim, contextError)
	}
	classified := classifyAudioRequestError(cause)
	var storeError *AudioStoreError
	if !errors.As(classified, &storeError) {
		storeError = newAudioStoreError(
			AudioErrorDownloadFailed,
			"episode audio preparation failed",
			true,
		)
	}
	durableContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	now := s.now().UTC()
	update := s.db.WithContext(durableContext).
		Model(&models.EpisodeAudioAsset{}).
		Where(
			"id = ? AND status = ? AND claim_token = ?",
			claim.AssetID,
			models.EpisodeAudioAssetStatusDownloading,
			claim.Token,
		).
		Updates(map[string]any{
			"status":           models.EpisodeAudioAssetStatusFailed,
			"relative_path":    "",
			"sha256":           "",
			"size_bytes":       0,
			"media_type":       "",
			"extension":        "",
			"error_code":       storeError.Code,
			"error_message":    storeError.SafeMessage,
			"claim_token":      "",
			"claim_expires_at": nil,
			"failed_at":        now,
			"updated_at":       now,
		})
	if update.Error != nil {
		return newAudioStoreError(
			AudioErrorStorageFailed,
			"managed audio failure state could not be recorded",
			true,
		)
	}
	if update.RowsAffected == 0 {
		return newAudioStoreError(
			AudioErrorClaimLost,
			"managed audio claim was lost",
			true,
		)
	}
	return storeError
}

func (s *DiskAudioStore) releasePreparationClaim(
	ctx context.Context,
	claim AudioClaim,
	contextError error,
) error {
	durableContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	now := s.now().UTC()
	update := s.db.WithContext(durableContext).
		Model(&models.EpisodeAudioAsset{}).
		Where(
			"id = ? AND status = ? AND claim_token = ?",
			claim.AssetID,
			models.EpisodeAudioAssetStatusDownloading,
			claim.Token,
		).
		Updates(map[string]any{
			"status":           models.EpisodeAudioAssetStatusQueued,
			"claim_token":      "",
			"claim_expires_at": nil,
			"updated_at":       now,
		})
	if update.Error != nil {
		return errors.Join(
			contextError,
			newAudioStoreError(
				AudioErrorStorageFailed,
				"managed audio claim could not be released",
				true,
			),
		)
	}
	return contextError
}

func (s *DiskAudioStore) episodeDirectory(episodeID uint) (string, error) {
	episodesRoot := filepath.Join(s.root, "episodes")
	if err := ensureProtectedDirectory(episodesRoot); err != nil {
		return "", newAudioStoreError(
			AudioErrorStorageFailed,
			"managed audio directory is unavailable",
			true,
		)
	}
	directory := filepath.Join(episodesRoot, strconv.FormatUint(uint64(episodeID), 10))
	if err := ensureProtectedDirectory(directory); err != nil {
		return "", newAudioStoreError(
			AudioErrorStorageFailed,
			"managed episode audio directory is unavailable",
			true,
		)
	}
	return directory, nil
}

func ensureProtectedDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("managed path is not a directory")
	}
	return os.Chmod(path, 0o700)
}

func (s *DiskAudioStore) managedPath(relativePath string) (string, error) {
	if relativePath == "" || filepath.IsAbs(relativePath) {
		return "", newAudioStoreError(
			AudioErrorReadyFileInvalid,
			"managed episode audio path is invalid",
			false,
		)
	}
	cleanRelative := filepath.Clean(relativePath)
	if cleanRelative == "." ||
		cleanRelative == ".." ||
		strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) {
		return "", newAudioStoreError(
			AudioErrorReadyFileInvalid,
			"managed episode audio path is invalid",
			false,
		)
	}
	fullPath := filepath.Join(s.root, cleanRelative)
	relativeToRoot, err := filepath.Rel(s.root, fullPath)
	if err != nil ||
		relativeToRoot == "." ||
		relativeToRoot == ".." ||
		strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) {
		return "", newAudioStoreError(
			AudioErrorReadyFileInvalid,
			"managed episode audio path is invalid",
			false,
		)
	}
	return filepath.Clean(fullPath), nil
}

func randomAudioClaimToken() (string, error) {
	value := make([]byte, sha256.Size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
