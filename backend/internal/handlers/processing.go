package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"magicpodcast/internal/middleware"
	"magicpodcast/internal/models"
	"magicpodcast/internal/processing"

	"github.com/gin-gonic/gin"
)

type ProcessingHandler struct {
	service   *processing.Service
	canceler  processing.RunCanceler
	scheduler processing.ScheduleStatusProvider
}

func NewProcessingHandler(
	service *processing.Service,
	canceler processing.RunCanceler,
	schedulers ...processing.ScheduleStatusProvider,
) *ProcessingHandler {
	handler := &ProcessingHandler{service: service, canceler: canceler}
	if len(schedulers) > 0 {
		handler.scheduler = schedulers[0]
	}
	return handler
}

type startProcessingRequest struct {
	Force bool `json:"force"`
}

func (h *ProcessingHandler) Start(c *gin.Context) {
	episodeID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	var request startProcessingRequest
	if c.Request.ContentLength != 0 {
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVALID_PROCESSING_REQUEST",
					"message": "request body must contain only an optional boolean force field",
				},
			})
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVALID_PROCESSING_REQUEST",
					"message": "request body must contain one JSON object",
				},
			})
			return
		}
	}
	result, err := h.service.StartEpisodeProcessing(c.Request.Context(), processing.StartRequest{
		EpisodeID:     episodeID,
		TriggerSource: models.ProcessingTriggerManual,
		Force:         request.Force,
	})
	switch {
	case errors.Is(err, processing.ErrInvalidStart):
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_PROCESSING_REQUEST",
				"message": err.Error(),
			},
		})
		return
	case errors.Is(err, processing.ErrProcessingInputUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PROCESSING_INPUT_UNAVAILABLE",
				"message": "downloaded audio or processing pipeline is not available",
			},
		})
		return
	case errors.Is(err, processing.ErrEpisodeNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "EPISODE_NOT_FOUND",
				"message": "episode not found",
			},
		})
		return
	case errors.Is(err, processing.ErrEpisodeNotFocused):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "EPISODE_NOT_IN_FOCUS",
				"message": "episode must be in Focus before processing starts",
			},
		})
		return
	case err != nil && isAudioStoreError(err):
		writeAudioStoreError(c, err)
		return
	case err != nil:
		middleware.InternalErrorResponseWithCode(c, "PROCESSING_START_FAILED", "Failed to start episode processing")
		return
	}

	status := http.StatusCreated
	if result.PreparingAudio {
		status = http.StatusAccepted
	} else if result.ReusedActive || result.ReusedSuccessful {
		status = http.StatusOK
	}
	c.JSON(status, gin.H{"success": true, "data": result})
}

func (h *ProcessingHandler) GetLatestAudio(c *gin.Context) {
	episodeID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	asset, err := h.service.GetLatestEpisodeAudioAsset(
		c.Request.Context(),
		episodeID,
	)
	switch {
	case errors.Is(err, processing.ErrEpisodeNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "EPISODE_NOT_FOUND",
				"message": "episode not found",
			},
		})
		return
	case errors.Is(err, processing.ErrProcessingInputUnavailable):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "AUDIO_ASSET_NOT_FOUND",
				"message": "managed audio asset not found",
			},
		})
		return
	case err != nil:
		middleware.InternalErrorResponseWithCode(c, "AUDIO_ASSET_READ_FAILED", "Failed to read managed audio state")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": asset})
}

func (h *ProcessingHandler) GetScheduleStatus(c *gin.Context) {
	status := processing.ScheduleStatus{Enabled: false}
	if h.scheduler != nil {
		var err error
		status, err = h.scheduler.Status(c.Request.Context())
		if err != nil {
			middleware.InternalErrorResponseWithCode(c, "PROCESSING_SCHEDULE_READ_FAILED", "Failed to read processing schedule")
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": status})
}

func (h *ProcessingHandler) Cancel(c *gin.Context) {
	runID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	var (
		run models.EpisodeProcessingRun
		err error
	)
	if h.canceler != nil {
		run, err = h.canceler.Cancel(c.Request.Context(), runID)
	} else {
		run, err = h.service.CancelProcessingRun(c.Request.Context(), runID)
	}
	switch {
	case errors.Is(err, processing.ErrRunNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PROCESSING_RUN_NOT_FOUND",
				"message": "processing run not found",
			},
		})
		return
	case err != nil:
		middleware.InternalErrorResponseWithCode(c, "PROCESSING_CANCEL_FAILED", "Failed to cancel processing run")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": run})
}

func (h *ProcessingHandler) Retry(c *gin.Context) {
	runID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	result, err := h.service.RetryProcessingRun(c.Request.Context(), runID)
	switch {
	case errors.Is(err, processing.ErrRunNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PROCESSING_RUN_NOT_FOUND",
				"message": "processing run not found",
			},
		})
		return
	case errors.Is(err, processing.ErrRetryUnsafe):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PROCESSING_RETRY_UNSAFE",
				"message": "processing run cannot be retried without risking duplicate external writes",
			},
		})
		return
	case errors.Is(err, processing.ErrEpisodeNotFocused):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "EPISODE_NOT_IN_FOCUS",
				"message": "episode must be in Focus before processing retries",
			},
		})
		return
	case errors.Is(err, processing.ErrProcessingInputUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PROCESSING_INPUT_UNAVAILABLE",
				"message": "downloaded audio or processing pipeline is not available",
			},
		})
		return
	case err != nil:
		middleware.InternalErrorResponseWithCode(c, "PROCESSING_RETRY_FAILED", "Failed to retry processing run")
		return
	}
	status := http.StatusCreated
	if result.ReusedActive {
		status = http.StatusOK
	}
	c.JSON(status, gin.H{"success": true, "data": result})
}

func (h *ProcessingHandler) Get(c *gin.Context) {
	runID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	detail, err := h.service.GetProcessingRun(c.Request.Context(), runID)
	switch {
	case errors.Is(err, processing.ErrRunNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PROCESSING_RUN_NOT_FOUND",
				"message": "processing run not found",
			},
		})
		return
	case err != nil:
		middleware.InternalErrorResponseWithCode(c, "PROCESSING_READ_FAILED", "Failed to read processing run")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": detail})
}

func (h *ProcessingHandler) ListEpisodeRuns(c *gin.Context) {
	episodeID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	runs, err := h.service.ListEpisodeProcessingRuns(c.Request.Context(), episodeID)
	switch {
	case errors.Is(err, processing.ErrEpisodeNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "EPISODE_NOT_FOUND",
				"message": "episode not found",
			},
		})
		return
	case err != nil:
		middleware.InternalErrorResponseWithCode(c, "PROCESSING_LIST_FAILED", "Failed to list processing runs")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": runs})
}

func (h *ProcessingHandler) GetArtifactContent(c *gin.Context) {
	artifactSetID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	content, err := h.service.GetArtifactContent(
		c.Request.Context(),
		artifactSetID,
		c.Param("kind"),
	)
	switch {
	case errors.Is(err, processing.ErrArtifactNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "ARTIFACT_NOT_FOUND",
				"message": "artifact set not found",
			},
		})
		return
	case errors.Is(err, processing.ErrInvalidArtifact):
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "ARTIFACT_INVALID",
				"message": "artifact content failed integrity validation",
			},
		})
		return
	case err != nil:
		middleware.InternalErrorResponseWithCode(c, "ARTIFACT_READ_FAILED", "Failed to read artifact content")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": content})
}

func isAudioStoreError(err error) bool {
	var audioErr *processing.AudioStoreError
	return errors.As(err, &audioErr)
}

func writeAudioStoreError(c *gin.Context, err error) {
	var audioErr *processing.AudioStoreError
	if !errors.As(err, &audioErr) {
		middleware.InternalErrorResponseWithCode(c, "AUDIO_PREPARATION_FAILED", "Failed to prepare managed audio")
		return
	}
	status := http.StatusUnprocessableEntity
	if audioErr.Retryable {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{
		"success": false,
		"error": gin.H{
			"code":    strings.ToUpper(audioErr.Code),
			"message": audioErr.SafeMessage,
		},
	})
}
