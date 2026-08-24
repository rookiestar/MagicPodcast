package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"magicpodcast/internal/episodecopilot"
	"magicpodcast/internal/middleware"

	"github.com/gin-gonic/gin"
)

const (
	maxEpisodeCopilotRequestBytes = 256 << 10
	episodeCopilotWriteTimeout    = 15 * time.Second
)

type EpisodeCopilotHandler struct {
	module episodecopilot.Module
}

func NewEpisodeCopilotHandler(
	module episodecopilot.Module,
) *EpisodeCopilotHandler {
	return &EpisodeCopilotHandler{module: module}
}

type episodeCopilotQuestionBody struct {
	Question           string                         `json:"question"`
	Selection          string                         `json:"selection"`
	SelectionSource    episodecopilot.SelectionSource `json:"selection_source"`
	IncludePrivateNote bool                           `json:"include_private_note"`
}

func (h *EpisodeCopilotHandler) Context(c *gin.Context) {
	episodeID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	if h.module == nil {
		writeEpisodeCopilotUnavailable(c)
		return
	}
	scope, err := h.module.ContextScope(c.Request.Context(), episodeID)
	if err != nil {
		writeEpisodeCopilotError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": scope})
}

func (h *EpisodeCopilotHandler) Ask(c *gin.Context) {
	episodeID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	if h.module == nil {
		writeEpisodeCopilotUnavailable(c)
		return
	}
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		maxEpisodeCopilotRequestBytes,
	)
	var body episodeCopilotQuestionBody
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeInvalidEpisodeCopilotRequest(c)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeInvalidEpisodeCopilotRequest(c)
		return
	}
	events, err := h.module.Ask(
		c.Request.Context(),
		episodecopilot.QuestionRequest{
			EpisodeID:          episodeID,
			Question:           body.Question,
			Selection:          body.Selection,
			SelectionSource:    body.SelectionSource,
			IncludePrivateNote: body.IncludePrivateNote,
		},
	)
	if err != nil {
		writeEpisodeCopilotError(c, err)
		return
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		middleware.InternalErrorResponseWithCode(
			c,
			"COPILOT_STREAM_UNAVAILABLE",
			"Streaming response is unavailable",
		)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	responseController := http.NewResponseController(c.Writer)
	refreshWriteDeadline := func() {
		_ = responseController.SetWriteDeadline(
			time.Now().Add(episodeCopilotWriteTimeout),
		)
	}
	refreshWriteDeadline()
	flusher.Flush()

	keepalive := time.NewTicker(10 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case event, open := <-events:
			if !open {
				return
			}
			payload, marshalErr := json.Marshal(event)
			if marshalErr != nil {
				return
			}
			refreshWriteDeadline()
			if _, writeErr := fmt.Fprintf(
				c.Writer,
				"data: %s\n\n",
				payload,
			); writeErr != nil {
				return
			}
			flusher.Flush()
		case <-keepalive.C:
			refreshWriteDeadline()
			if _, writeErr := fmt.Fprint(c.Writer, ": ping\n\n"); writeErr != nil {
				return
			}
			flusher.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}

func writeInvalidEpisodeCopilotRequest(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{
		"success": false,
		"error": gin.H{
			"code":    "INVALID_COPILOT_REQUEST",
			"message": "request must contain a valid question and optional current-episode selection",
		},
	})
}

func writeEpisodeCopilotUnavailable(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"success": false,
		"error": gin.H{
			"code":    "COPILOT_UNAVAILABLE",
			"message": "Episode Copilot is not available on this runtime",
		},
	})
}

func writeEpisodeCopilotError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, episodecopilot.ErrInvalidQuestion):
		writeInvalidEpisodeCopilotRequest(c)
	case errors.Is(err, episodecopilot.ErrEpisodeNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "EPISODE_NOT_FOUND",
				"message": "episode not found",
			},
		})
	case errors.Is(err, episodecopilot.ErrContextUnavailable):
		writeEpisodeCopilotUnavailable(c)
	default:
		middleware.InternalErrorResponseWithCode(
			c,
			"COPILOT_REQUEST_FAILED",
			"Episode Copilot request failed",
		)
	}
}
