package processing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"magicpodcast/internal/models"
)

type WorkerConfig struct {
	ScanInterval         time.Duration
	ExternalPollInterval time.Duration
	BatchSize            int
}

func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		ScanInterval:         2 * time.Second,
		ExternalPollInterval: 30 * time.Second,
		BatchSize:            8,
	}
}

type Worker struct {
	service *Service
	engine  *Engine
	audio   AudioPreparer
	config  WorkerConfig
}

func NewWorker(
	service *Service,
	engine *Engine,
	audio AudioPreparer,
	config WorkerConfig,
) (*Worker, error) {
	if service == nil || engine == nil {
		return nil, fmt.Errorf("processing worker requires service and engine")
	}
	if config.ScanInterval <= 0 ||
		config.ExternalPollInterval <= 0 ||
		config.BatchSize < 1 {
		return nil, fmt.Errorf("processing worker configuration is invalid")
	}
	return &Worker{
		service: service,
		engine:  engine,
		audio:   audio,
		config:  config,
	}, nil
}

// Run owns restart recovery and durable queue polling. API construction stays
// read-only; only an explicitly enabled worker calls this method.
func (w *Worker) Run(ctx context.Context) error {
	if _, err := w.service.RecoverNonTerminalRuns(ctx, w.service.now().UTC()); err != nil {
		return err
	}
	if err := w.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	ticker := time.NewTicker(w.config.ScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
		}
	}
}

func (w *Worker) RunOnce(ctx context.Context) error {
	if w.audio != nil {
		if err := w.reconcileAudioPreparationRuns(ctx); err != nil {
			return err
		}
		assetIDs, err := w.audio.ListClaimable(ctx, w.config.BatchSize)
		if err != nil {
			return err
		}
		for _, assetID := range assetIDs {
			claim, claimed, err := w.audio.Claim(ctx, assetID)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return err
				}
				continue
			}
			if !claimed {
				continue
			}
			ready, err := w.audio.Prepare(ctx, claim)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return err
				}
				code, message, retryable := audioPreparationFailure(err)
				if failErr := w.service.failAudioPreparation(
					ctx,
					claim.EpisodeID,
					code,
					message,
					retryable,
				); failErr != nil {
					return failErr
				}
				continue
			}
			if _, _, err := w.service.completeAudioPreparation(
				ctx,
				claim.EpisodeID,
				ready,
			); err != nil {
				return fmt.Errorf(
					"complete audio preparation for episode %d: %w",
					claim.EpisodeID,
					err,
				)
			}
		}
	}

	runIDs, err := w.service.listRunnableRunIDs(
		ctx,
		w.service.now().UTC(),
		w.config.ExternalPollInterval,
		w.config.BatchSize,
	)
	if err != nil {
		return err
	}
	for _, runID := range runIDs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := w.engine.Advance(ctx, runID); err != nil {
			switch {
			case errors.Is(err, ErrRunBusy), errors.Is(err, ErrRetryNotReady):
				continue
			case errors.Is(err, context.Canceled):
				return err
			default:
				return fmt.Errorf("advance processing run %d: %w", runID, err)
			}
		}
	}
	return nil
}

func (w *Worker) reconcileAudioPreparationRuns(ctx context.Context) error {
	runs, err := w.service.listAudioPreparationRuns(ctx)
	if err != nil {
		return err
	}
	for _, run := range runs {
		ready, resolveErr := w.audio.ResolveReadyAudio(ctx, run.EpisodeID)
		if resolveErr == nil {
			if _, _, err := w.service.completeAudioPreparation(
				ctx,
				run.EpisodeID,
				ready,
			); err != nil {
				return err
			}
			continue
		}
		if errors.Is(resolveErr, context.Canceled) {
			return resolveErr
		}
		asset, assetErr := w.service.GetLatestEpisodeAudioAsset(ctx, run.EpisodeID)
		switch {
		case assetErr == nil &&
			(asset.Status == models.EpisodeAudioAssetStatusQueued ||
				asset.Status == models.EpisodeAudioAssetStatusDownloading):
			continue
		case assetErr == nil && asset.Status == models.EpisodeAudioAssetStatusFailed:
			if err := w.service.failAudioPreparation(
				ctx,
				run.EpisodeID,
				asset.ErrorCode,
				asset.ErrorMessage,
				audioFailureRetryable(asset.ErrorCode),
			); err != nil {
				return err
			}
		default:
			code, message, retryable := audioPreparationFailure(resolveErr)
			if err := w.service.failAudioPreparation(
				ctx,
				run.EpisodeID,
				code,
				message,
				retryable,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func audioPreparationFailure(err error) (string, string, bool) {
	var audioErr *AudioStoreError
	if errors.As(err, &audioErr) {
		return audioErr.Code, audioErr.SafeMessage, audioErr.Retryable
	}
	return AudioErrorDownloadFailed, "episode audio preparation failed", true
}

func audioFailureRetryable(code string) bool {
	switch code {
	case AudioErrorClaimLost,
		AudioErrorSourceUnavailable,
		AudioErrorHTTPStatus,
		AudioErrorEmpty,
		AudioErrorDownloadTimeout,
		AudioErrorDownloadFailed,
		AudioErrorStorageFailed:
		return true
	default:
		return false
	}
}

func (w *Worker) Canceler() RunCanceler {
	return w.engine
}
