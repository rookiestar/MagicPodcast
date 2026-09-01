package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

const (
	defaultAudioRecoveryClaimTTL    = 35 * time.Minute
	defaultAudioRecoveryDownloadTTL = 30 * time.Minute
	defaultAudioRecoveryMaxAttempts = 3
	defaultAudioRecoveryMaxElapsed  = 24 * time.Hour
	defaultAudioRecoveryBaseDelay   = 5 * time.Second
)

const (
	AudioRecoveryErrorUnavailable       = "audio_recovery_unavailable"
	AudioRecoveryErrorArtifactInvalid   = "audio_recovery_artifact_invalid"
	AudioRecoveryErrorCheckpointMissing = "audio_recovery_checkpoint_missing"
	AudioRecoveryErrorCheckpointInvalid = "audio_recovery_checkpoint_invalid"
	AudioRecoveryErrorAdapterMismatch   = "audio_recovery_adapter_mismatch"
	AudioRecoveryErrorFileTokenMissing  = "audio_recovery_file_token_missing"
	AudioRecoveryErrorManagedAudio      = "audio_recovery_managed_audio_invalid"
	AudioRecoveryErrorPathInvalid       = "audio_recovery_path_invalid"
	AudioRecoveryErrorSymlink           = "audio_recovery_symlink_blocked"
	AudioRecoveryErrorPermission        = "audio_recovery_permission_invalid"
	AudioRecoveryErrorRemoteNotFound    = "audio_recovery_remote_not_found"
	AudioRecoveryErrorAuth              = "audio_recovery_auth_required"
	AudioRecoveryErrorRemotePermission  = "audio_recovery_permission_denied"
	AudioRecoveryErrorFormat            = "audio_recovery_format_unsupported"
	AudioRecoveryErrorTooLarge          = "audio_recovery_too_large"
	AudioRecoveryErrorEmpty             = "audio_recovery_empty"
	AudioRecoveryErrorSizeMismatch      = "audio_recovery_size_mismatch"
	AudioRecoveryErrorDigestMismatch    = "audio_recovery_digest_mismatch"
	AudioRecoveryErrorDownloadTimeout   = "audio_recovery_download_timeout"
	AudioRecoveryErrorDownloadFailed    = "audio_recovery_download_failed"
	AudioRecoveryErrorStorageFailed     = "audio_recovery_storage_failed"
	AudioRecoveryErrorClaimLost         = "audio_recovery_claim_lost"
)

var ErrAudioRecoveryUnavailable = errors.New("audio recovery is unavailable")

// AudioRecoveryError is the provider-neutral, safe error recorded for a
// recovery attempt. It never contains a token, path, digest, URL, or command
// output.
type AudioRecoveryError struct {
	Code        string
	SafeMessage string
	Retryable   bool
}

func (e *AudioRecoveryError) Error() string {
	if e == nil {
		return ""
	}
	return e.SafeMessage
}

func newAudioRecoveryError(code, message string, retryable bool) *AudioRecoveryError {
	return &AudioRecoveryError{
		Code:        code,
		SafeMessage: message,
		Retryable:   retryable,
	}
}

// DriveAudioDownloader is the supplier boundary for recovery. Production
// uses the user-scoped lark-cli Drive download; tests can replace it with a
// bounded fake without touching external services.
type DriveAudioDownloader interface {
	Download(context.Context, string, string, string) error
}

type feishuDriveAudioDownloader struct {
	runner larkCommandRunner
}

func NewFeishuDriveAudioDownloader(command string) (DriveAudioDownloader, error) {
	runner, err := newExecLarkCLI(command)
	if err != nil {
		return nil, err
	}
	return newFeishuDriveAudioDownloaderWithRunner(runner)
}

func newFeishuDriveAudioDownloaderWithRunner(
	runner larkCommandRunner,
) (DriveAudioDownloader, error) {
	if runner == nil {
		return nil, fmt.Errorf("Feishu Drive downloader requires a command runner")
	}
	return &feishuDriveAudioDownloader{runner: runner}, nil
}

func (d *feishuDriveAudioDownloader) Download(
	ctx context.Context,
	fileToken string,
	directory string,
	fileName string,
) error {
	if !larkTokenPattern.MatchString(strings.TrimSpace(fileToken)) {
		return newAudioRecoveryError(
			AudioRecoveryErrorFileTokenMissing,
			"飞书 Drive 文件身份无效，无法恢复音频",
			false,
		)
	}
	if !filepathIsCanonicalDirectory(directory) || !isProtectedDirectory(directory) {
		return newAudioRecoveryError(
			AudioRecoveryErrorPathInvalid,
			"恢复临时目录不安全，未写入受管音频",
			false,
		)
	}
	if fileName == "" || filepath.Base(fileName) != fileName ||
		strings.ContainsAny(fileName, `/\\`) || fileName == "." || fileName == ".." {
		return newAudioRecoveryError(
			AudioRecoveryErrorPathInvalid,
			"恢复文件路径无效，未写入受管音频",
			false,
		)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	outputPath := filepath.Join(directory, fileName)
	_, err := d.runner.Run(
		ctx,
		directory,
		"drive",
		"+download",
		"--file-token",
		fileToken,
		"--output",
		"./"+fileName,
		"--overwrite",
		"--as",
		"user",
		"--format",
		"json",
	)
	if err != nil {
		return classifyDriveDownloadError(err)
	}
	if err := protectDownloadedRecoveryFile(outputPath); err != nil {
		return err
	}
	return nil
}

// lark-cli creates the file, so the downloader closes the permission gap
// before the content validator opens it. Symlinks and non-regular outputs are
// rejected before chmod; chmod must never follow an attacker-controlled link.
func protectDownloadedRecoveryFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return newAudioRecoveryError(
			AudioRecoveryErrorDownloadFailed,
			"飞书 Drive 未返回完整音频文件",
			true,
		)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return newAudioRecoveryError(
			AudioRecoveryErrorSymlink,
			"下载文件是软链接，已拒绝恢复",
			false,
		)
	}
	if !info.Mode().IsRegular() {
		return newAudioRecoveryError(
			AudioRecoveryErrorPathInvalid,
			"下载结果不是普通文件，已拒绝恢复",
			false,
		)
	}
	file, err := os.OpenFile(
		path,
		os.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return newAudioRecoveryError(
			AudioRecoveryErrorPermission,
			"下载文件无法以受保护方式打开，未执行恢复",
			true,
		)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(opened, info) {
		return newAudioRecoveryError(
			AudioRecoveryErrorSymlink,
			"下载文件在保护前发生变化，已拒绝恢复",
			false,
		)
	}
	if err := file.Chmod(0o600); err != nil {
		return newAudioRecoveryError(
			AudioRecoveryErrorPermission,
			"下载文件权限无法保护，未执行恢复",
			true,
		)
	}
	protected, err := file.Stat()
	pathInfo, pathErr := os.Lstat(path)
	if err != nil ||
		!protected.Mode().IsRegular() ||
		protected.Mode().Perm() != 0o600 ||
		pathErr != nil ||
		pathInfo.Mode()&os.ModeSymlink != 0 ||
		!os.SameFile(protected, pathInfo) {
		return newAudioRecoveryError(
			AudioRecoveryErrorPermission,
			"下载文件权限不安全，未执行恢复",
			false,
		)
	}
	return nil
}

func classifyDriveDownloadError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var recoveryErr *AudioRecoveryError
	if errors.As(err, &recoveryErr) {
		return recoveryErr
	}

	mapped, known := classifyLarkCommandError(err)
	if !known {
		return newAudioRecoveryError(
			AudioRecoveryErrorDownloadFailed,
			"飞书 Drive 音频下载失败，稍后可重试",
			true,
		)
	}
	var adapterErr *AdapterError
	if !errors.As(mapped, &adapterErr) {
		return newAudioRecoveryError(
			AudioRecoveryErrorDownloadFailed,
			"飞书 Drive 音频下载失败，稍后可重试",
			true,
		)
	}
	switch adapterErr.ErrorCode {
	case "lark_auth_expired":
		return newAudioRecoveryError(
			AudioRecoveryErrorAuth,
			"飞书用户登录已过期，请重新登录后重试",
			false,
		)
	case "lark_permission_denied":
		return newAudioRecoveryError(
			AudioRecoveryErrorRemotePermission,
			"飞书用户没有读取该文件的权限，请检查权限后重试",
			false,
		)
	case "lark_resource_not_found":
		return newAudioRecoveryError(
			AudioRecoveryErrorRemoteNotFound,
			"飞书 Drive 原始音频不存在或已被删除",
			false,
		)
	case "lark_request_rejected":
		return newAudioRecoveryError(
			AudioRecoveryErrorDownloadFailed,
			"飞书 Drive 拒绝了音频读取请求",
			false,
		)
	default:
		return newAudioRecoveryError(
			AudioRecoveryErrorDownloadFailed,
			"飞书 Drive 音频下载失败，稍后可重试",
			adapterErr.CanRetry || adapterErr.ResultUnknown,
		)
	}
}

type AudioRecoveryPolicy struct {
	MaxAttempts     int
	MaxElapsed      time.Duration
	BaseDelay       time.Duration
	ClaimTTL        time.Duration
	DownloadTimeout time.Duration
}

func DefaultAudioRecoveryPolicy() AudioRecoveryPolicy {
	return AudioRecoveryPolicy{
		MaxAttempts:     defaultAudioRecoveryMaxAttempts,
		MaxElapsed:      defaultAudioRecoveryMaxElapsed,
		BaseDelay:       defaultAudioRecoveryBaseDelay,
		ClaimTTL:        defaultAudioRecoveryClaimTTL,
		DownloadTimeout: defaultAudioRecoveryDownloadTTL,
	}
}

type AudioRecoveryOption func(*AudioRecoveryStore)

func WithAudioRecoveryPolicy(policy AudioRecoveryPolicy) AudioRecoveryOption {
	return func(store *AudioRecoveryStore) {
		if policy.MaxAttempts > 0 {
			store.maxAttempts = policy.MaxAttempts
		}
		if policy.MaxElapsed > 0 {
			store.maxElapsed = policy.MaxElapsed
		}
		if policy.BaseDelay >= 0 {
			store.baseDelay = policy.BaseDelay
		}
		if policy.ClaimTTL > 0 {
			store.claimTTL = policy.ClaimTTL
		}
		if policy.DownloadTimeout > 0 {
			store.downloadTimeout = policy.DownloadTimeout
		}
	}
}

func WithAudioRecoveryClock(now func() time.Time) AudioRecoveryOption {
	return func(store *AudioRecoveryStore) {
		if now != nil {
			store.now = now
		}
	}
}

type AudioRecoverySummary struct {
	Recoverable   bool       `json:"recoverable"`
	Status        string     `json:"status,omitempty"`
	ErrorCode     string     `json:"error_code,omitempty"`
	ErrorMessage  string     `json:"error_message,omitempty"`
	CanRetry      bool       `json:"can_retry"`
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}

type ArtifactAudioRecoverySummary = AudioRecoverySummary

type AudioRecoveryEnqueueResult struct {
	ArtifactSetID    uint                 `json:"artifact_set_id"`
	AudioRecovery    AudioRecoverySummary `json:"audio_recovery"`
	Queued           bool                 `json:"queued"`
	Reused           bool                 `json:"reused"`
	AlreadyAvailable bool                 `json:"already_available"`
}

type AudioRecoveryClaim struct {
	ID            uint   `json:"-"`
	ArtifactSetID uint   `json:"-"`
	EpisodeID     uint   `json:"-"`
	Token         string `json:"-"`
}

type AudioRecovery interface {
	Summary(context.Context, models.EpisodeArtifactSet) (AudioRecoverySummary, error)
	Enqueue(context.Context, uint) (AudioRecoveryEnqueueResult, error)
	ListClaimable(context.Context, int) ([]uint, error)
	Claim(context.Context, uint) (AudioRecoveryClaim, bool, error)
	Recover(context.Context, AudioRecoveryClaim) error
}

type AudioRecoveryStore struct {
	db              *gorm.DB
	audio           *DiskAudioStore
	downloader      DriveAudioDownloader
	maxAttempts     int
	maxElapsed      time.Duration
	baseDelay       time.Duration
	claimTTL        time.Duration
	downloadTimeout time.Duration
	now             func() time.Time
}

func NewAudioRecoveryStore(
	db *gorm.DB,
	audio *DiskAudioStore,
	downloader DriveAudioDownloader,
	options ...AudioRecoveryOption,
) (*AudioRecoveryStore, error) {
	if db == nil || audio == nil || downloader == nil {
		return nil, fmt.Errorf("audio recovery dependencies are required")
	}
	policy := DefaultAudioRecoveryPolicy()
	store := &AudioRecoveryStore{
		db:              db,
		audio:           audio,
		downloader:      downloader,
		maxAttempts:     policy.MaxAttempts,
		maxElapsed:      policy.MaxElapsed,
		baseDelay:       policy.BaseDelay,
		claimTTL:        policy.ClaimTTL,
		downloadTimeout: policy.DownloadTimeout,
		now:             time.Now,
	}
	for _, option := range options {
		if option != nil {
			option(store)
		}
	}
	if store.maxAttempts < 1 {
		store.maxAttempts = defaultAudioRecoveryMaxAttempts
	}
	if store.maxElapsed <= 0 {
		store.maxElapsed = defaultAudioRecoveryMaxElapsed
	}
	if store.baseDelay < 0 {
		store.baseDelay = 0
	}
	if store.claimTTL <= 0 {
		store.claimTTL = defaultAudioRecoveryClaimTTL
	}
	if store.downloadTimeout <= 0 {
		store.downloadTimeout = defaultAudioRecoveryDownloadTTL
	}
	if store.downloadTimeout > store.claimTTL {
		store.downloadTimeout = store.claimTTL
	}
	return store, nil
}

func NewFeishuDriveAudioRecovery(
	db *gorm.DB,
	audio *DiskAudioStore,
	command string,
	options ...AudioRecoveryOption,
) (*AudioRecoveryStore, error) {
	downloader, err := NewFeishuDriveAudioDownloader(command)
	if err != nil {
		return nil, err
	}
	return NewAudioRecoveryStore(db, audio, downloader, options...)
}

type audioRecoverySource struct {
	artifact  models.EpisodeArtifactSet
	run       models.EpisodeProcessingRun
	asset     models.EpisodeAudioAsset
	target    string
	fileToken string
}

func (s *AudioRecoveryStore) Summary(
	ctx context.Context,
	artifact models.EpisodeArtifactSet,
) (AudioRecoverySummary, error) {
	// A healthy local cache is the normal path and remains authoritative even
	// if the historical recovery source is no longer readable. Recovery is a
	// repair capability; it must not turn an already playable artifact into an
	// apparent failure.
	if artifact.EpisodeID != 0 && sha256Pattern.MatchString(artifact.AudioSHA256) {
		if _, err := s.audio.ResolveReadyAudioByDigest(
			ctx,
			artifact.EpisodeID,
			artifact.AudioSHA256,
		); err == nil {
			return AudioRecoverySummary{
				Status: models.EpisodeArtifactAudioRecoveryStatusCompleted,
			}, nil
		}
	}
	source, err := s.resolveSource(ctx, artifact)
	if err != nil {
		var recoveryErr *AudioRecoveryError
		if errors.As(err, &recoveryErr) {
			return AudioRecoverySummary{
				ErrorCode:    recoveryErr.Code,
				ErrorMessage: recoveryErr.SafeMessage,
			}, nil
		}
		return AudioRecoverySummary{}, err
	}
	recovery, found, err := s.loadRecovery(ctx, artifact.ID)
	if err != nil {
		return AudioRecoverySummary{}, err
	}
	if !found {
		return AudioRecoverySummary{Recoverable: true}, nil
	}
	if recovery.EpisodeID != source.artifact.EpisodeID ||
		recovery.AudioAssetID != source.asset.ID ||
		recovery.AudioSHA256 != source.artifact.AudioSHA256 {
		return AudioRecoverySummary{
			ErrorCode:    AudioRecoveryErrorUnavailable,
			ErrorMessage: "恢复记录与当前产物不一致，无法安全恢复",
		}, nil
	}
	return audioRecoverySummaryFromRecord(recovery), nil
}

func (s *AudioRecoveryStore) Enqueue(
	ctx context.Context,
	artifactSetID uint,
) (AudioRecoveryEnqueueResult, error) {
	if artifactSetID == 0 {
		return AudioRecoveryEnqueueResult{}, ErrInvalidArtifact
	}
	var artifact models.EpisodeArtifactSet
	if err := s.db.WithContext(ctx).First(&artifact, artifactSetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AudioRecoveryEnqueueResult{}, ErrArtifactNotFound
		}
		return AudioRecoveryEnqueueResult{}, newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复状态暂时不可用",
			true,
		)
	}
	if artifact.EpisodeID != 0 && sha256Pattern.MatchString(artifact.AudioSHA256) {
		if _, err := s.audio.ResolveReadyAudioByDigest(
			ctx,
			artifact.EpisodeID,
			artifact.AudioSHA256,
		); err == nil {
			return AudioRecoveryEnqueueResult{
				ArtifactSetID: artifactSetID,
				AudioRecovery: AudioRecoverySummary{
					Status: models.EpisodeArtifactAudioRecoveryStatusCompleted,
				},
				AlreadyAvailable: true,
			}, nil
		}
	}
	source, err := s.resolveSource(ctx, artifact)
	if err != nil {
		return AudioRecoveryEnqueueResult{}, err
	}
	now := s.now().UTC()
	result := AudioRecoveryEnqueueResult{ArtifactSetID: artifactSetID}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var recovery models.EpisodeArtifactAudioRecovery
		readErr := tx.Where("artifact_set_id = ?", artifactSetID).First(&recovery).Error
		switch {
		case errors.Is(readErr, gorm.ErrRecordNotFound):
			recovery = models.EpisodeArtifactAudioRecovery{
				ArtifactSetID:   artifactSetID,
				EpisodeID:       source.artifact.EpisodeID,
				AudioAssetID:    source.asset.ID,
				AudioSHA256:     source.artifact.AudioSHA256,
				Status:          models.EpisodeArtifactAudioRecoveryStatusQueued,
				MaxAttempts:     s.maxAttempts,
				RetryDeadlineAt: now.Add(s.maxElapsed),
				NextAttemptAt:   timePtr(now),
				QueuedAt:        now,
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			if createErr := tx.Create(&recovery).Error; createErr != nil {
				if !isUniqueConstraintError(createErr) {
					return newAudioRecoveryError(
						AudioRecoveryErrorStorageFailed,
						"音频恢复状态暂时不可用",
						true,
					)
				}
				if retryErr := tx.Where("artifact_set_id = ?", artifactSetID).First(&recovery).Error; retryErr != nil {
					return newAudioRecoveryError(
						AudioRecoveryErrorStorageFailed,
						"音频恢复状态暂时不可用",
						true,
					)
				}
				result.Reused = true
			} else {
				result.Queued = true
			}
		case readErr != nil:
			return newAudioRecoveryError(
				AudioRecoveryErrorStorageFailed,
				"音频恢复状态暂时不可用",
				true,
			)
		default:
			if recovery.EpisodeID != source.artifact.EpisodeID ||
				recovery.AudioAssetID != source.asset.ID ||
				recovery.AudioSHA256 != source.artifact.AudioSHA256 {
				return newAudioRecoveryError(
					AudioRecoveryErrorUnavailable,
					"恢复记录与当前产物不一致，无法安全恢复",
					false,
				)
			}
			switch recovery.Status {
			case models.EpisodeArtifactAudioRecoveryStatusQueued:
				result.Reused = true
			case models.EpisodeArtifactAudioRecoveryStatusDownloading:
				if recovery.ClaimExpiresAt != nil && recovery.ClaimExpiresAt.After(now) {
					result.Reused = true
					break
				}
				if err := requeueExpiredAudioRecovery(tx, &recovery, now); err != nil {
					return err
				}
				result.Queued = true
			case models.EpisodeArtifactAudioRecoveryStatusCompleted,
				models.EpisodeArtifactAudioRecoveryStatusFailed:
				if err := resetAudioRecoveryForManualRetry(tx, &recovery, now, s.maxAttempts, s.maxElapsed); err != nil {
					return err
				}
				result.Queued = true
			default:
				return newAudioRecoveryError(
					AudioRecoveryErrorUnavailable,
					"恢复状态无效，无法安全恢复",
					false,
				)
			}
		}
		if err := tx.Where("artifact_set_id = ?", artifactSetID).First(&recovery).Error; err != nil {
			return newAudioRecoveryError(
				AudioRecoveryErrorStorageFailed,
				"音频恢复状态暂时不可用",
				true,
			)
		}
		result.AudioRecovery = audioRecoverySummaryFromRecord(recovery)
		return nil
	})
	return result, err
}

func (s *AudioRecoveryStore) ListClaimable(
	ctx context.Context,
	limit int,
) ([]uint, error) {
	if limit <= 0 || limit > 1000 {
		return nil, newAudioRecoveryError(
			AudioRecoveryErrorUnavailable,
			"音频恢复批次大小无效",
			false,
		)
	}
	now := s.now().UTC()
	if err := s.finalizeExhausted(ctx, now); err != nil {
		return nil, err
	}
	var ids []uint
	err := s.db.WithContext(ctx).Model(&models.EpisodeArtifactAudioRecovery{}).
		Where(
			`(status = ? AND attempt_count < max_attempts
				AND (retry_deadline_at IS NULL OR retry_deadline_at > ?)
				AND (next_attempt_at IS NULL OR next_attempt_at <= ?))
			 OR (status = ? AND attempt_count < max_attempts
				AND (retry_deadline_at IS NULL OR retry_deadline_at > ?)
				AND (claim_expires_at IS NULL OR claim_expires_at <= ?))`,
			models.EpisodeArtifactAudioRecoveryStatusQueued,
			now,
			now,
			models.EpisodeArtifactAudioRecoveryStatusDownloading,
			now,
			now,
		).
		Order("queued_at ASC").Order("id ASC").Limit(limit).Pluck("id", &ids).Error
	if err != nil {
		return nil, newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复状态暂时不可用",
			true,
		)
	}
	return ids, nil
}

func (s *AudioRecoveryStore) Claim(
	ctx context.Context,
	recoveryID uint,
) (AudioRecoveryClaim, bool, error) {
	if recoveryID == 0 {
		return AudioRecoveryClaim{}, false, newAudioRecoveryError(
			AudioRecoveryErrorUnavailable,
			"音频恢复任务无效",
			false,
		)
	}
	token, err := randomAudioClaimToken()
	if err != nil {
		return AudioRecoveryClaim{}, false, newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复任务暂时不可用",
			true,
		)
	}
	now := s.now().UTC()
	expiresAt := now.Add(s.claimTTL)
	update := s.db.WithContext(ctx).Model(&models.EpisodeArtifactAudioRecovery{}).
		Where(
			`id = ? AND (
				(status = ? AND attempt_count < max_attempts
					AND (retry_deadline_at IS NULL OR retry_deadline_at > ?)
					AND (next_attempt_at IS NULL OR next_attempt_at <= ?))
				OR (status = ? AND attempt_count < max_attempts
					AND (retry_deadline_at IS NULL OR retry_deadline_at > ?)
					AND (claim_expires_at IS NULL OR claim_expires_at <= ?))
			)`,
			recoveryID,
			models.EpisodeArtifactAudioRecoveryStatusQueued,
			now,
			now,
			models.EpisodeArtifactAudioRecoveryStatusDownloading,
			now,
			now,
		).
		Updates(map[string]any{
			"status":           models.EpisodeArtifactAudioRecoveryStatusDownloading,
			"claim_token":      token,
			"claim_expires_at": expiresAt,
			"downloading_at":   now,
			"attempt_count":    gorm.Expr("attempt_count + 1"),
			"updated_at":       now,
		})
	if update.Error != nil {
		return AudioRecoveryClaim{}, false, newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复任务暂时不可用",
			true,
		)
	}
	if update.RowsAffected == 0 {
		var existing models.EpisodeArtifactAudioRecovery
		err := s.db.WithContext(ctx).First(&existing, recoveryID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return AudioRecoveryClaim{}, false, newAudioRecoveryError(
				AudioRecoveryErrorUnavailable,
				"音频恢复任务不存在",
				false,
			)
		}
		if err != nil {
			return AudioRecoveryClaim{}, false, newAudioRecoveryError(
				AudioRecoveryErrorStorageFailed,
				"音频恢复任务暂时不可用",
				true,
			)
		}
		return AudioRecoveryClaim{}, false, nil
	}
	var claimed models.EpisodeArtifactAudioRecovery
	if err := s.db.WithContext(ctx).
		Where("id = ? AND claim_token = ?", recoveryID, token).
		First(&claimed).Error; err != nil {
		return AudioRecoveryClaim{}, false, newAudioRecoveryError(
			AudioRecoveryErrorClaimLost,
			"音频恢复任务领取已失效",
			true,
		)
	}
	return AudioRecoveryClaim{
		ID:            claimed.ID,
		ArtifactSetID: claimed.ArtifactSetID,
		EpisodeID:     claimed.EpisodeID,
		Token:         token,
	}, true, nil
}

func (s *AudioRecoveryStore) finalizeExhausted(
	ctx context.Context,
	now time.Time,
) error {
	update := s.db.WithContext(ctx).
		Model(&models.EpisodeArtifactAudioRecovery{}).
		Where(
			`(status = ? AND (attempt_count >= max_attempts OR max_attempts < 1 OR retry_deadline_at <= ?))
			 OR (status = ? AND (claim_expires_at IS NULL OR claim_expires_at <= ?)
				AND (attempt_count >= max_attempts OR max_attempts < 1 OR retry_deadline_at <= ?))`,
			models.EpisodeArtifactAudioRecoveryStatusQueued,
			now,
			models.EpisodeArtifactAudioRecoveryStatusDownloading,
			now,
			now,
		).
		Updates(map[string]any{
			"status":           models.EpisodeArtifactAudioRecoveryStatusFailed,
			"claim_token":      "",
			"claim_expires_at": nil,
			"next_attempt_at":  nil,
			"error_code":       AudioRecoveryErrorDownloadFailed,
			"error_message":    "音频恢复未在有限时间内完成，请检查后重试",
			"error_retryable":  false,
			"failed_at":        now,
			"updated_at":       now,
		})
	if update.Error != nil {
		return newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复状态暂时不可用",
			true,
		)
	}
	return nil
}

func (s *AudioRecoveryStore) Recover(
	ctx context.Context,
	claim AudioRecoveryClaim,
) error {
	recovery, err := s.loadClaim(ctx, claim)
	if err != nil {
		return err
	}
	var artifact models.EpisodeArtifactSet
	if err := s.db.WithContext(ctx).First(&artifact, recovery.ArtifactSetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.failClaim(ctx, claim, newAudioRecoveryError(
				AudioRecoveryErrorArtifactInvalid,
				"目标转写产物不存在，无法恢复音频",
				false,
			))
		}
		return s.failClaim(ctx, claim, newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复状态暂时不可用",
			true,
		))
	}
	source, err := s.resolveSource(ctx, artifact)
	if err != nil {
		return s.failClaim(ctx, claim, err)
	}
	if source.artifact.EpisodeID != recovery.EpisodeID ||
		source.asset.ID != recovery.AudioAssetID ||
		source.artifact.AudioSHA256 != recovery.AudioSHA256 {
		return s.failClaim(ctx, claim, newAudioRecoveryError(
			AudioRecoveryErrorUnavailable,
			"恢复记录与当前产物不一致，无法安全恢复",
			false,
		))
	}
	if err := ctx.Err(); err != nil {
		return s.releaseClaim(ctx, claim, err)
	}
	if err := s.audio.validateRecoveryTarget(source.target, true); err != nil {
		return s.failClaim(ctx, claim, err)
	}
	extension := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(source.asset.Extension)), ".")
	temporaryDirectory, err := os.MkdirTemp(
		filepath.Dir(source.target),
		fmt.Sprintf(".audio-recovery-%d-*", recovery.ID),
	)
	if err != nil {
		return s.failClaim(ctx, claim, newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"恢复临时文件无法创建，未覆盖现有音频",
			true,
		))
	}
	defer os.RemoveAll(temporaryDirectory)
	if err := os.Chmod(temporaryDirectory, 0o700); err != nil {
		return s.failClaim(ctx, claim, newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"恢复临时目录无法保护，未覆盖现有音频",
			true,
		))
	}
	fileName := "recovered." + extension
	downloadContext, cancelDownload := context.WithTimeout(ctx, s.downloadTimeout)
	defer cancelDownload()
	if err := s.downloader.Download(
		downloadContext,
		source.fileToken,
		temporaryDirectory,
		fileName,
	); err != nil {
		if ctx.Err() != nil {
			return s.releaseClaim(ctx, claim, ctx.Err())
		}
		if errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(downloadContext.Err(), context.DeadlineExceeded) {
			return s.failClaim(ctx, claim, newAudioRecoveryError(
				AudioRecoveryErrorDownloadTimeout,
				"飞书 Drive 音频下载超时，稍后可重试",
				true,
			))
		}
		return s.failClaim(ctx, claim, err)
	}
	if err := ctx.Err(); err != nil {
		return s.releaseClaim(ctx, claim, err)
	}
	if err := downloadContext.Err(); err != nil {
		return s.failClaim(ctx, claim, newAudioRecoveryError(
			AudioRecoveryErrorDownloadTimeout,
			"飞书 Drive 音频下载超时，稍后可重试",
			true,
		))
	}
	temporaryPath := filepath.Join(temporaryDirectory, fileName)
	if err := validateDownloadedRecoveryFile(
		temporaryPath,
		source.asset.SizeBytes,
		source.artifact.AudioSHA256,
	); err != nil {
		return s.failClaim(ctx, claim, err)
	}
	if _, err := s.loadClaim(ctx, claim); err != nil {
		return err
	}
	published, err := s.publishRecoveryFile(temporaryPath, source.target)
	if err != nil {
		return s.failClaim(ctx, claim, err)
	}
	rollback := true
	defer func() {
		if rollback {
			_ = published.rollback()
		}
	}()
	s.audio.verified.Delete(source.target)
	if _, err := s.audio.ResolveReadyAudioByDigest(
		ctx,
		source.artifact.EpisodeID,
		source.artifact.AudioSHA256,
	); err != nil {
		return s.failClaim(ctx, claim, newAudioRecoveryError(
			AudioRecoveryErrorDigestMismatch,
			"恢复后的音频未通过受管摘要校验，未完成恢复",
			false,
		))
	}
	now := s.now().UTC()
	update := s.db.WithContext(ctx).Model(&models.EpisodeArtifactAudioRecovery{}).
		Where(
			`id = ? AND status = ? AND claim_token = ?
				AND claim_expires_at > ? AND retry_deadline_at > ?`,
			claim.ID,
			models.EpisodeArtifactAudioRecoveryStatusDownloading,
			claim.Token,
			now,
			now,
		).
		Updates(map[string]any{
			"status":           models.EpisodeArtifactAudioRecoveryStatusCompleted,
			"claim_token":      "",
			"claim_expires_at": nil,
			"completed_at":     now,
			"next_attempt_at":  nil,
			"error_code":       "",
			"error_message":    "",
			"error_retryable":  false,
			"updated_at":       now,
		})
	if update.Error != nil {
		return s.failClaim(ctx, claim, newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复状态无法完成，已保留原子恢复保护",
			true,
		))
	}
	if update.RowsAffected != 1 {
		return s.failClaim(ctx, claim, newAudioRecoveryError(
			AudioRecoveryErrorClaimLost,
			"音频恢复任务领取已失效，未确认恢复结果",
			true,
		))
	}
	rollback = false
	_ = published.cleanup()
	return nil
}

func (s *AudioRecoveryStore) resolveSource(
	ctx context.Context,
	artifact models.EpisodeArtifactSet,
) (audioRecoverySource, error) {
	if artifact.ID == 0 || artifact.RunID == 0 || artifact.EpisodeID == 0 ||
		artifact.PipelineVersion != NativeMinutesPipelineVersion ||
		!sha256Pattern.MatchString(artifact.AudioSHA256) {
		return audioRecoverySource{}, newAudioRecoveryError(
			AudioRecoveryErrorArtifactInvalid,
			"当前转写产物不具备安全的音频恢复条件",
			false,
		)
	}
	var run models.EpisodeProcessingRun
	if err := s.db.WithContext(ctx).First(&run, artifact.RunID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return audioRecoverySource{}, newAudioRecoveryError(
				AudioRecoveryErrorArtifactInvalid,
				"当前转写产物不具备安全的音频恢复条件",
				false,
			)
		}
		return audioRecoverySource{}, newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复状态暂时不可用",
			true,
		)
	}
	if run.EpisodeID != artifact.EpisodeID ||
		run.Status != models.ProcessingRunStatusCompleted ||
		run.PipelineVersion != NativeMinutesPipelineVersion ||
		run.AudioDigest != artifact.AudioSHA256 {
		return audioRecoverySource{}, newAudioRecoveryError(
			AudioRecoveryErrorArtifactInvalid,
			"当前转写产物不具备安全的音频恢复条件",
			false,
		)
	}
	var checkpoint models.ProcessingCheckpoint
	err := s.db.WithContext(ctx).
		Where("run_id = ? AND step = ?", run.ID, StepTranscription).
		First(&checkpoint).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return audioRecoverySource{}, newAudioRecoveryError(
			AudioRecoveryErrorCheckpointMissing,
			"缺少可恢复的飞书转写检查点，逐字稿仍可阅读",
			false,
		)
	}
	if err != nil {
		return audioRecoverySource{}, newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复状态暂时不可用",
			true,
		)
	}
	if !checkpointIsValid(checkpoint) || checkpoint.Status != ExternalProgressCompleted {
		return audioRecoverySource{}, newAudioRecoveryError(
			AudioRecoveryErrorCheckpointInvalid,
			"飞书转写检查点未通过完整性校验，逐字稿仍可阅读",
			false,
		)
	}
	if checkpoint.Adapter != feishuMinutesAdapterName ||
		checkpoint.AdapterVersion != feishuMinutesAdapterVersion {
		return audioRecoverySource{}, newAudioRecoveryError(
			AudioRecoveryErrorAdapterMismatch,
			"当前转写检查点版本不支持安全恢复",
			false,
		)
	}
	state, err := decodeFeishuCheckpoint([]byte(checkpoint.StateJSON))
	if err != nil || state.Phase != feishuPhaseTranscriptStored ||
		state.AudioDigest != artifact.AudioSHA256 {
		return audioRecoverySource{}, newAudioRecoveryError(
			AudioRecoveryErrorCheckpointInvalid,
			"飞书转写检查点未通过完整性校验，逐字稿仍可阅读",
			false,
		)
	}
	if !larkTokenPattern.MatchString(state.FileToken) {
		return audioRecoverySource{}, newAudioRecoveryError(
			AudioRecoveryErrorFileTokenMissing,
			"缺少受保护的飞书 Drive 文件身份，无法恢复音频",
			false,
		)
	}
	var asset models.EpisodeAudioAsset
	err = s.db.WithContext(ctx).
		Where(
			"episode_id = ? AND sha256 = ? AND status = ?",
			artifact.EpisodeID,
			artifact.AudioSHA256,
			models.EpisodeAudioAssetStatusReady,
		).
		Order("id DESC").First(&asset).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return audioRecoverySource{}, newAudioRecoveryError(
			AudioRecoveryErrorManagedAudio,
			"缺少匹配的受管音频元数据，无法安全恢复",
			false,
		)
	}
	if err != nil {
		return audioRecoverySource{}, newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复状态暂时不可用",
			true,
		)
	}
	if err := validateRecoveryAudioAsset(asset, artifact.AudioSHA256); err != nil {
		return audioRecoverySource{}, err
	}
	target, err := s.audio.managedPath(filepath.FromSlash(asset.RelativePath))
	if err != nil {
		return audioRecoverySource{}, newAudioRecoveryError(
			AudioRecoveryErrorPathInvalid,
			"受管音频路径不安全，未执行恢复",
			false,
		)
	}
	if err := s.audio.validateRecoveryTarget(target, false); err != nil {
		return audioRecoverySource{}, err
	}
	return audioRecoverySource{
		artifact:  artifact,
		run:       run,
		asset:     asset,
		target:    target,
		fileToken: state.FileToken,
	}, nil
}

func validateRecoveryAudioAsset(
	asset models.EpisodeAudioAsset,
	expectedDigest string,
) error {
	extension := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(asset.Extension)), ".")
	mediaType := strings.ToLower(strings.TrimSpace(asset.MediaType))
	allowed, extensionAllowed := audioContentTypesByExtension[extension]
	_, mediaAllowed := allowed[mediaType]
	if !extensionAllowed || !mediaAllowed || !isBrowserPlayableMediaType(mediaType) {
		return newAudioRecoveryError(
			AudioRecoveryErrorFormat,
			"受管音频格式不受支持，无法恢复",
			false,
		)
	}
	if asset.Status != models.EpisodeAudioAssetStatusReady ||
		asset.SHA256 != expectedDigest ||
		!sha256Pattern.MatchString(asset.SHA256) ||
		asset.SizeBytes <= 0 || asset.SizeBytes > MaxManagedAudioBytes ||
		asset.DurationSeconds <= 0 ||
		asset.DurationSeconds > int(MaxManagedAudioDuration/time.Second) ||
		strings.TrimSpace(asset.RelativePath) == "" {
		return newAudioRecoveryError(
			AudioRecoveryErrorManagedAudio,
			"受管音频元数据不完整或格式不受支持，无法恢复",
			false,
		)
	}
	if _, err := hex.DecodeString(asset.SHA256); err != nil {
		return newAudioRecoveryError(
			AudioRecoveryErrorManagedAudio,
			"受管音频摘要无效，无法恢复",
			false,
		)
	}
	return nil
}

func validateDownloadedRecoveryFile(
	path string,
	expectedSize int64,
	expectedDigest string,
) error {
	info, err := os.Lstat(path)
	if err != nil {
		return newAudioRecoveryError(
			AudioRecoveryErrorDownloadFailed,
			"飞书 Drive 未返回完整音频文件",
			true,
		)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return newAudioRecoveryError(
			AudioRecoveryErrorSymlink,
			"下载文件是软链接，已拒绝恢复",
			false,
		)
	}
	if !info.Mode().IsRegular() {
		return newAudioRecoveryError(
			AudioRecoveryErrorPathInvalid,
			"下载结果不是普通文件，已拒绝恢复",
			false,
		)
	}
	if info.Size() <= 0 {
		return newAudioRecoveryError(
			AudioRecoveryErrorEmpty,
			"下载的音频文件为空，未覆盖现有音频",
			false,
		)
	}
	if info.Size() > MaxManagedAudioBytes {
		return newAudioRecoveryError(
			AudioRecoveryErrorTooLarge,
			"下载的音频超过受管大小上限，未覆盖现有音频",
			false,
		)
	}
	if expectedSize > 0 && info.Size() != expectedSize {
		return newAudioRecoveryError(
			AudioRecoveryErrorSizeMismatch,
			"下载的音频大小与产物记录不一致，未覆盖现有音频",
			false,
		)
	}
	if info.Mode().Perm() != 0o600 {
		return newAudioRecoveryError(
			AudioRecoveryErrorPermission,
			"下载文件权限不安全，未覆盖现有音频",
			false,
		)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || filepath.Clean(canonical) != filepath.Clean(path) {
		return newAudioRecoveryError(
			AudioRecoveryErrorSymlink,
			"下载文件路径不安全，已拒绝恢复",
			false,
		)
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return newAudioRecoveryError(
			AudioRecoveryErrorDownloadFailed,
			"下载的音频文件无法读取，未覆盖现有音频",
			true,
		)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || openedInfo.Size() != info.Size() ||
		openedInfo.Mode().Perm() != 0o600 || !os.SameFile(openedInfo, info) {
		return newAudioRecoveryError(
			AudioRecoveryErrorDownloadFailed,
			"下载文件在校验前发生变化，未覆盖现有音频",
			true,
		)
	}
	hasher := sha256.New()
	read, err := io.Copy(hasher, io.LimitReader(file, MaxManagedAudioBytes+1))
	if err != nil {
		return newAudioRecoveryError(
			AudioRecoveryErrorDownloadFailed,
			"下载的音频文件无法校验，未覆盖现有音频",
			true,
		)
	}
	if read > MaxManagedAudioBytes {
		return newAudioRecoveryError(
			AudioRecoveryErrorTooLarge,
			"下载的音频超过受管大小上限，未覆盖现有音频",
			false,
		)
	}
	if hex.EncodeToString(hasher.Sum(nil)) != expectedDigest {
		return newAudioRecoveryError(
			AudioRecoveryErrorDigestMismatch,
			"下载的音频与目标产物摘要不一致，未覆盖现有音频",
			false,
		)
	}
	finalInfo, pathErr := file.Stat()
	pathInfo, lstatErr := os.Lstat(path)
	if pathErr != nil || lstatErr != nil || !finalInfo.Mode().IsRegular() ||
		!pathInfo.Mode().IsRegular() || pathInfo.Mode()&os.ModeSymlink != 0 ||
		finalInfo.Mode().Perm() != 0o600 || pathInfo.Mode().Perm() != 0o600 ||
		finalInfo.Size() != openedInfo.Size() ||
		!os.SameFile(finalInfo, pathInfo) {
		return newAudioRecoveryError(
			AudioRecoveryErrorDownloadFailed,
			"下载文件在校验后发生变化，未覆盖现有音频",
			true,
		)
	}
	return nil
}

type recoveryPublication struct {
	target string
	backup string
}

func (p recoveryPublication) rollback() error {
	_ = os.Remove(p.target)
	if p.backup == "" {
		return nil
	}
	return os.Rename(p.backup, p.target)
}

func (p recoveryPublication) cleanup() error {
	if p.backup == "" {
		return nil
	}
	return os.Remove(p.backup)
}

func (s *AudioRecoveryStore) publishRecoveryFile(
	temporaryPath string,
	target string,
) (recoveryPublication, error) {
	if err := s.audio.validateRecoveryTarget(target, true); err != nil {
		return recoveryPublication{}, err
	}
	publication := recoveryPublication{target: target}
	targetInfo, err := os.Lstat(target)
	switch {
	case err == nil:
		if targetInfo.Mode()&os.ModeSymlink != 0 {
			return recoveryPublication{}, newAudioRecoveryError(
				AudioRecoveryErrorSymlink,
				"受管音频目标是软链接，已拒绝覆盖",
				false,
			)
		}
		if !targetInfo.Mode().IsRegular() || targetInfo.Mode().Perm() != 0o600 {
			return recoveryPublication{}, newAudioRecoveryError(
				AudioRecoveryErrorPermission,
				"受管音频目标权限或类型不安全，已拒绝覆盖",
				false,
			)
		}
		backupFile, createErr := os.CreateTemp(filepath.Dir(target), ".audio-recovery-backup-*")
		if createErr != nil {
			return recoveryPublication{}, newAudioRecoveryError(
				AudioRecoveryErrorStorageFailed,
				"现有受管音频无法安全保留，未执行恢复",
				true,
			)
		}
		backupPath := backupFile.Name()
		if closeErr := backupFile.Close(); closeErr != nil {
			_ = os.Remove(backupPath)
			return recoveryPublication{}, newAudioRecoveryError(
				AudioRecoveryErrorStorageFailed,
				"现有受管音频无法安全保留，未执行恢复",
				true,
			)
		}
		if removeErr := os.Remove(backupPath); removeErr != nil {
			return recoveryPublication{}, newAudioRecoveryError(
				AudioRecoveryErrorStorageFailed,
				"现有受管音频无法安全保留，未执行恢复",
				true,
			)
		}
		if linkErr := os.Link(target, backupPath); linkErr != nil {
			return recoveryPublication{}, newAudioRecoveryError(
				AudioRecoveryErrorStorageFailed,
				"现有受管音频无法安全保留，未执行恢复",
				true,
			)
		}
		publication.backup = backupPath
	case errors.Is(err, os.ErrNotExist):
		// A missing local file is the expected repair case.
	default:
		return recoveryPublication{}, newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"受管音频目标无法安全读取，未执行恢复",
			true,
		)
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		_ = publication.rollback()
		return recoveryPublication{}, newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"受管音频无法原子恢复，未覆盖现有音频",
			true,
		)
	}
	if err := os.Chmod(target, 0o600); err != nil {
		_ = publication.rollback()
		return recoveryPublication{}, newAudioRecoveryError(
			AudioRecoveryErrorPermission,
			"恢复后的音频权限不安全，未完成恢复",
			false,
		)
	}
	return publication, nil
}

func (s *DiskAudioStore) validateRecoveryTarget(path string, createParent bool) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return newAudioRecoveryError(
			AudioRecoveryErrorPathInvalid,
			"受管音频路径不安全，未执行恢复",
			false,
		)
	}
	relative, err := filepath.Rel(s.root, path)
	if err != nil || pathEscapesRoot(relative) {
		return newAudioRecoveryError(
			AudioRecoveryErrorPathInvalid,
			"受管音频路径不安全，未执行恢复",
			false,
		)
	}
	parent := filepath.Dir(path)
	if err := validateManagedRecoveryParent(s.root, parent); err != nil {
		return err
	}
	if createParent {
		if err := ensureProtectedDirectory(parent); err != nil {
			return newAudioRecoveryError(
				AudioRecoveryErrorStorageFailed,
				"受管音频目录无法保护，未执行恢复",
				true,
			)
		}
		if err := validateManagedRecoveryParent(s.root, parent); err != nil {
			return err
		}
	}
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err != nil:
		return newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"受管音频目标无法安全读取，未执行恢复",
			true,
		)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return newAudioRecoveryError(
			AudioRecoveryErrorSymlink,
			"受管音频目标是软链接，已拒绝恢复",
			false,
		)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return newAudioRecoveryError(
			AudioRecoveryErrorPermission,
			"受管音频目标权限或类型不安全，已拒绝恢复",
			false,
		)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || filepath.Clean(canonical) != filepath.Clean(path) {
		return newAudioRecoveryError(
			AudioRecoveryErrorSymlink,
			"受管音频目标路径不安全，已拒绝恢复",
			false,
		)
	}
	return nil
}

func validateManagedRecoveryParent(root, parent string) error {
	root = filepath.Clean(root)
	parent = filepath.Clean(parent)
	relative, err := filepath.Rel(root, parent)
	if err != nil || pathEscapesRoot(relative) {
		return newAudioRecoveryError(
			AudioRecoveryErrorPathInvalid,
			"受管音频目录路径不安全，已拒绝恢复",
			false,
		)
	}

	current := parent
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
				return newAudioRecoveryError(
					AudioRecoveryErrorPermission,
					"受管音频目录权限或类型不安全，已拒绝恢复",
					false,
				)
			}
			canonical, evalErr := filepath.EvalSymlinks(current)
			if evalErr != nil || filepath.Clean(canonical) != current {
				return newAudioRecoveryError(
					AudioRecoveryErrorSymlink,
					"受管音频目录路径不安全，已拒绝恢复",
					false,
				)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return newAudioRecoveryError(
				AudioRecoveryErrorStorageFailed,
				"受管音频目录无法安全读取，未执行恢复",
				true,
			)
		}
		if current == root {
			if err != nil {
				return newAudioRecoveryError(
					AudioRecoveryErrorPermission,
					"受管音频根目录权限或类型不安全，已拒绝恢复",
					false,
				)
			}
			return nil
		}
		next := filepath.Dir(current)
		if next == current {
			return newAudioRecoveryError(
				AudioRecoveryErrorPathInvalid,
				"受管音频目录路径不安全，已拒绝恢复",
				false,
			)
		}
		current = next
	}
}

func isProtectedDirectory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 && info.Mode().Perm() == 0o700
}

func (s *AudioRecoveryStore) loadRecovery(
	ctx context.Context,
	artifactSetID uint,
) (models.EpisodeArtifactAudioRecovery, bool, error) {
	var recovery models.EpisodeArtifactAudioRecovery
	err := s.db.WithContext(ctx).Where("artifact_set_id = ?", artifactSetID).First(&recovery).Error
	switch {
	case err == nil:
		return recovery, true, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return models.EpisodeArtifactAudioRecovery{}, false, nil
	default:
		return models.EpisodeArtifactAudioRecovery{}, false, newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复状态暂时不可用",
			true,
		)
	}
}

func (s *AudioRecoveryStore) loadClaim(
	ctx context.Context,
	claim AudioRecoveryClaim,
) (models.EpisodeArtifactAudioRecovery, error) {
	if claim.ID == 0 || claim.ArtifactSetID == 0 || claim.EpisodeID == 0 ||
		len(claim.Token) != sha256.Size*2 {
		return models.EpisodeArtifactAudioRecovery{}, newAudioRecoveryError(
			AudioRecoveryErrorClaimLost,
			"音频恢复任务领取已失效",
			true,
		)
	}
	var recovery models.EpisodeArtifactAudioRecovery
	now := s.now().UTC()
	err := s.db.WithContext(ctx).
		Where(
			`id = ? AND artifact_set_id = ? AND episode_id = ? AND status = ?
				AND claim_token = ? AND claim_expires_at > ? AND retry_deadline_at > ?`,
			claim.ID,
			claim.ArtifactSetID,
			claim.EpisodeID,
			models.EpisodeArtifactAudioRecoveryStatusDownloading,
			claim.Token,
			now,
			now,
		).
		First(&recovery).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.EpisodeArtifactAudioRecovery{}, newAudioRecoveryError(
			AudioRecoveryErrorClaimLost,
			"音频恢复任务领取已失效",
			true,
		)
	}
	if err != nil {
		return models.EpisodeArtifactAudioRecovery{}, newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复状态暂时不可用",
			true,
		)
	}
	return recovery, nil
}

func (s *AudioRecoveryStore) failClaim(
	ctx context.Context,
	claim AudioRecoveryClaim,
	cause error,
) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return s.releaseClaim(ctx, claim, ctxErr)
	}
	classified := classifyAudioRecoveryFailure(cause)
	recovery, err := s.loadClaim(ctx, claim)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	maxAttempts := recovery.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = s.maxAttempts
	}
	canAutoRetry := classified.Retryable &&
		recovery.AttemptCount < maxAttempts &&
		!recovery.RetryDeadlineAt.IsZero() && now.Before(recovery.RetryDeadlineAt)
	values := map[string]any{
		"claim_token":      "",
		"claim_expires_at": nil,
		"error_code":       classified.Code,
		"error_message":    classified.SafeMessage,
		"error_retryable":  classified.Retryable,
		"updated_at":       now,
	}
	if canAutoRetry {
		next := now.Add(audioRecoveryBackoff(s.baseDelay, recovery.AttemptCount))
		if next.After(recovery.RetryDeadlineAt) {
			next = recovery.RetryDeadlineAt
		}
		values["status"] = models.EpisodeArtifactAudioRecoveryStatusQueued
		values["next_attempt_at"] = next
		values["failed_at"] = nil
	} else {
		values["status"] = models.EpisodeArtifactAudioRecoveryStatusFailed
		values["next_attempt_at"] = nil
		values["failed_at"] = now
		values["error_retryable"] = false
	}
	update := s.db.WithContext(ctx).Model(&models.EpisodeArtifactAudioRecovery{}).
		Where(
			"id = ? AND status = ? AND claim_token = ?",
			claim.ID,
			models.EpisodeArtifactAudioRecoveryStatusDownloading,
			claim.Token,
		).
		Updates(values)
	if update.Error != nil {
		return newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复失败状态无法保存",
			true,
		)
	}
	if update.RowsAffected != 1 {
		return newAudioRecoveryError(
			AudioRecoveryErrorClaimLost,
			"音频恢复任务领取已失效",
			true,
		)
	}
	return classified
}

func (s *AudioRecoveryStore) releaseClaim(
	ctx context.Context,
	claim AudioRecoveryClaim,
	contextErr error,
) error {
	durableCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	now := s.now().UTC()
	update := s.db.WithContext(durableCtx).Model(&models.EpisodeArtifactAudioRecovery{}).
		Where(
			"id = ? AND status = ? AND claim_token = ?",
			claim.ID,
			models.EpisodeArtifactAudioRecoveryStatusDownloading,
			claim.Token,
		).
		Updates(map[string]any{
			"status":           models.EpisodeArtifactAudioRecoveryStatusQueued,
			"claim_token":      "",
			"claim_expires_at": nil,
			"next_attempt_at":  now,
			"updated_at":       now,
		})
	if update.Error != nil {
		return errors.Join(
			contextErr,
			newAudioRecoveryError(
				AudioRecoveryErrorStorageFailed,
				"音频恢复任务领取无法释放",
				true,
			),
		)
	}
	return contextErr
}

func classifyAudioRecoveryFailure(err error) *AudioRecoveryError {
	if err == nil {
		return newAudioRecoveryError(
			AudioRecoveryErrorDownloadFailed,
			"音频恢复失败，稍后可重试",
			true,
		)
	}
	var recoveryErr *AudioRecoveryError
	if errors.As(err, &recoveryErr) {
		return recoveryErr
	}
	return newAudioRecoveryError(
		AudioRecoveryErrorDownloadFailed,
		"音频恢复失败，稍后可重试",
		true,
	)
}

func audioRecoveryBackoff(base time.Duration, attempt int) time.Duration {
	if base <= 0 || attempt <= 0 {
		return 0
	}
	delay := base
	for i := 1; i < attempt && delay < time.Hour; i++ {
		if delay > time.Hour/2 {
			return time.Hour
		}
		delay *= 2
	}
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}

func audioRecoverySummaryFromRecord(
	recovery models.EpisodeArtifactAudioRecovery,
) AudioRecoverySummary {
	status := recovery.Status
	if status == models.EpisodeArtifactAudioRecoveryStatusCompleted {
		// A completed record with a missing local file is ready to be explicitly
		// queued again; exposing it as active would make the UI look stuck.
		status = ""
	}
	updatedAt := recovery.UpdatedAt
	summary := AudioRecoverySummary{
		Recoverable:   true,
		Status:        status,
		ErrorCode:     recovery.ErrorCode,
		ErrorMessage:  recovery.ErrorMessage,
		CanRetry:      status == models.EpisodeArtifactAudioRecoveryStatusFailed && audioRecoveryCanRetry(recovery.ErrorCode),
		NextAttemptAt: recovery.NextAttemptAt,
		UpdatedAt:     &updatedAt,
	}
	return summary
}

func audioRecoveryCanRetry(code string) bool {
	switch code {
	case AudioRecoveryErrorArtifactInvalid,
		AudioRecoveryErrorCheckpointMissing,
		AudioRecoveryErrorCheckpointInvalid,
		AudioRecoveryErrorAdapterMismatch,
		AudioRecoveryErrorFileTokenMissing,
		AudioRecoveryErrorManagedAudio,
		AudioRecoveryErrorPathInvalid,
		AudioRecoveryErrorSymlink,
		AudioRecoveryErrorPermission,
		AudioRecoveryErrorRemoteNotFound,
		AudioRecoveryErrorFormat,
		AudioRecoveryErrorTooLarge,
		AudioRecoveryErrorEmpty,
		AudioRecoveryErrorSizeMismatch,
		AudioRecoveryErrorDigestMismatch:
		return false
	default:
		return true
	}
}

func resetAudioRecoveryForManualRetry(
	tx *gorm.DB,
	recovery *models.EpisodeArtifactAudioRecovery,
	now time.Time,
	maxAttempts int,
	maxElapsed time.Duration,
) error {
	if recovery == nil {
		return newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复状态暂时不可用",
			true,
		)
	}
	if maxAttempts < 1 {
		maxAttempts = defaultAudioRecoveryMaxAttempts
	}
	if maxElapsed <= 0 {
		maxElapsed = defaultAudioRecoveryMaxElapsed
	}
	update := tx.Model(&models.EpisodeArtifactAudioRecovery{}).
		Where("id = ?", recovery.ID).
		Updates(map[string]any{
			"status":            models.EpisodeArtifactAudioRecoveryStatusQueued,
			"attempt_count":     0,
			"max_attempts":      maxAttempts,
			"retry_deadline_at": now.Add(maxElapsed),
			"next_attempt_at":   now,
			"claim_token":       "",
			"claim_expires_at":  nil,
			"error_code":        "",
			"error_message":     "",
			"error_retryable":   false,
			"downloading_at":    nil,
			"completed_at":      nil,
			"failed_at":         nil,
			"queued_at":         now,
			"updated_at":        now,
		})
	if update.Error != nil || update.RowsAffected != 1 {
		return newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复状态暂时不可用",
			true,
		)
	}
	return nil
}

func requeueExpiredAudioRecovery(
	tx *gorm.DB,
	recovery *models.EpisodeArtifactAudioRecovery,
	now time.Time,
) error {
	if recovery == nil {
		return newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复状态暂时不可用",
			true,
		)
	}
	update := tx.Model(&models.EpisodeArtifactAudioRecovery{}).
		Where(
			"id = ? AND status = ? AND (claim_expires_at IS NULL OR claim_expires_at <= ?)",
			recovery.ID,
			models.EpisodeArtifactAudioRecoveryStatusDownloading,
			now,
		).
		Updates(map[string]any{
			"status":           models.EpisodeArtifactAudioRecoveryStatusQueued,
			"claim_token":      "",
			"claim_expires_at": nil,
			"next_attempt_at":  now,
			"updated_at":       now,
		})
	if update.Error != nil || update.RowsAffected != 1 {
		return newAudioRecoveryError(
			AudioRecoveryErrorStorageFailed,
			"音频恢复状态暂时不可用",
			true,
		)
	}
	return nil
}

func timePtr(value time.Time) *time.Time {
	return &value
}
