package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"magicpodcast/internal/middleware"
	"magicpodcast/internal/models"
	"magicpodcast/internal/processing"

	"github.com/gin-gonic/gin"
)

type ProcessingHandler struct {
	service  *processing.Service
	canceler processing.RunCanceler
}

func NewProcessingHandler(
	service *processing.Service,
	canceler processing.RunCanceler,
) *ProcessingHandler {
	return &ProcessingHandler{service: service, canceler: canceler}
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
	case err != nil:
		middleware.InternalErrorResponseWithCode(c, "PROCESSING_START_FAILED", "Failed to start episode processing")
		return
	}

	status := http.StatusCreated
	if result.ReusedActive || result.ReusedSuccessful {
		status = http.StatusOK
	}
	c.JSON(status, gin.H{"success": true, "data": result})
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
