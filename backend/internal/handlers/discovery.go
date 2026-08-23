package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"magicpodcast/internal/middleware"
	"magicpodcast/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type DiscoveryHandler struct {
	service               *services.DiscoveryService
	triageService         *services.TriageService
	consumptionService    *services.ConsumptionService
	homepageReportService *services.HomepageReportService
}

func NewDiscoveryHandler(
	service *services.DiscoveryService,
	triageService *services.TriageService,
	homepageReportService ...*services.HomepageReportService,
) *DiscoveryHandler {
	var reports *services.HomepageReportService
	if len(homepageReportService) > 0 {
		reports = homepageReportService[0]
	}
	return &DiscoveryHandler{
		service:               service,
		triageService:         triageService,
		consumptionService:    triageService.Consumption(),
		homepageReportService: reports,
	}
}

func (h *DiscoveryHandler) ListCandidates(c *gin.Context) {
	limit := 0
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit < 1 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVALID_LIMIT",
					"message": "limit must be a positive integer",
				},
			})
			return
		}
		limit = parsedLimit
	}

	view := c.Query("view")
	var candidates []services.DiscoveryCandidate
	var err error
	switch view {
	case "":
		candidates, err = h.service.ListRecentCandidates(limit)
	case "summary":
		candidates, err = h.service.ListRecentCandidateSummaries(limit)
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_VIEW",
				"message": "view must be summary when provided",
			},
		})
		return
	}
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch discovery candidates")
		return
	}

	episodeIDs := make([]uint, len(candidates))
	for index := range candidates {
		episodeIDs[index] = candidates[index].EpisodeID
	}
	decisions, err := h.triageService.DecisionsForEpisodes(episodeIDs)
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch triage decisions")
		return
	}
	for index := range candidates {
		if decision, exists := decisions[candidates[index].EpisodeID]; exists {
			services.AttachConsumptionStateToCandidate(&candidates[index], decision)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    candidates,
	})
}

func (h *DiscoveryHandler) GetCandidate(c *gin.Context) {
	episodeID, err := strconv.ParseUint(c.Param("episodeID"), 10, 64)
	if err != nil || episodeID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_EPISODE_ID",
				"message": "invalid episode id",
			},
		})
		return
	}

	candidate, err := h.service.GetCandidate(uint(episodeID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "CANDIDATE_NOT_FOUND",
					"message": "discovery candidate not found",
				},
			})
			return
		}
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch discovery candidate")
		return
	}

	decisions, err := h.triageService.DecisionsForEpisodes([]uint{candidate.EpisodeID})
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch triage decision")
		return
	}
	if decision, exists := decisions[candidate.EpisodeID]; exists {
		services.AttachConsumptionStateToCandidate(candidate, decision)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    candidate,
	})
}

// ListHomepageReports returns today's publishable workflow reports plus optional history.
// Empty today is a success with an empty list — not an error — so the homepage can hide the region (#90/#92).
// History rows are metadata-only; full body is loaded via GetHomepageReport (#95).
func (h *DiscoveryHandler) ListHomepageReports(c *gin.Context) {
	if h.homepageReportService == nil {
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "Homepage report service unavailable")
		return
	}

	historyLimit := 0
	if raw := c.Query("history_limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVALID_HISTORY_LIMIT",
					"message": "history_limit must be a non-negative integer",
				},
			})
			return
		}
		historyLimit = parsed
	}

	payload, err := h.homepageReportService.ListTodayAndHistory(historyLimit)
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch homepage reports")
		return
	}

	episodeIDs := services.CollectHomepageEpisodeIDs(payload.Today, payload.History)
	decisions, err := h.triageService.DecisionsForEpisodes(episodeIDs)
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch triage decisions")
		return
	}
	services.AttachHomepageReportDecisions(payload.Today, decisions)
	services.AttachHomepageReportDecisions(payload.History, decisions)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    payload,
	})
}

// GetHomepageReport returns one publishable report with full Markdown for on-demand history reading (#95).
func (h *DiscoveryHandler) GetHomepageReport(c *gin.Context) {
	if h.homepageReportService == nil {
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "Homepage report service unavailable")
		return
	}
	reportID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || reportID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_REPORT_ID", "message": "invalid report id"},
		})
		return
	}

	report, err := h.homepageReportService.GetPublishedReport(uint(reportID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || report == nil {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   gin.H{"code": "REPORT_NOT_FOUND", "message": "homepage report not found"},
			})
			return
		}
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch homepage report")
		return
	}
	if report == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   gin.H{"code": "REPORT_NOT_FOUND", "message": "homepage report not found"},
		})
		return
	}

	episodeIDs := services.CollectHomepageEpisodeIDs([]services.HomepageReport{*report})
	decisions, err := h.triageService.DecisionsForEpisodes(episodeIDs)
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch triage decisions")
		return
	}
	slice := []services.HomepageReport{*report}
	services.AttachHomepageReportDecisions(slice, decisions)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    slice[0],
	})
}

type putQueueRequest struct {
	QueueState            string `json:"queue_state"`
	AcknowledgeFocusLimit bool   `json:"acknowledge_focus_limit"`
}

type putPlacementRequest struct {
	QueueState            string           `json:"queue_state"`
	BeforeEpisodeID       *uint            `json:"before_episode_id"`
	ExpectedRevisions     map[string]int64 `json:"expected_revisions"`
	AcknowledgeFocusLimit bool             `json:"acknowledge_focus_limit"`
}

type putDismissedRequest struct {
	Dismissed bool `json:"dismissed"`
}

type undoCompletionRequest struct {
	Token string `json:"token" binding:"required"`
}

func (h *DiscoveryHandler) ListQueue(c *gin.Context) {
	queueState := c.Param("queue")
	snapshot, err := h.consumptionService.ListQueue(queueState)
	if errors.Is(err, services.ErrInvalidQueueState) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_QUEUE_STATE", "message": "invalid queue state"},
		})
		return
	}
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch consumption queue")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    snapshot,
	})
}

func (h *DiscoveryHandler) ListCompletionHistory(c *gin.Context) {
	limit := services.CompletionHistoryDefaultLimit
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit < 1 || parsedLimit > services.CompletionHistoryMaxLimit {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVALID_LIMIT",
					"message": "limit must be between 1 and 50",
				},
			})
			return
		}
		limit = parsedLimit
	}
	snapshot, err := h.consumptionService.ListCompletionHistory(
		services.CompletionHistoryOptions{
			Query:  c.Query("q"),
			Cursor: c.Query("cursor"),
			Limit:  limit,
		},
	)
	if errors.Is(err, services.ErrInvalidCompletionHistoryCursor) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_CURSOR",
				"message": "completion history cursor is invalid",
			},
		})
		return
	}
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch completion history")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    snapshot,
	})
}

func (h *DiscoveryHandler) GetQueueSummary(c *gin.Context) {
	summary, err := h.consumptionService.QueueSummary()
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch queue summary")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

func (h *DiscoveryHandler) GetConsumptionItem(c *gin.Context) {
	episodeID, ok := parseConsumptionEpisodeID(c)
	if !ok {
		return
	}
	item, err := h.consumptionService.GetItem(episodeID)
	if errors.Is(err, services.ErrConsumptionEpisodeNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   gin.H{"code": "EPISODE_NOT_FOUND", "message": "episode consumption state not found"},
		})
		return
	}
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch consumption state")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": item})
}

func (h *DiscoveryHandler) PutQueue(c *gin.Context) {
	episodeID, ok := parseConsumptionEpisodeID(c)
	if !ok {
		return
	}
	var request putQueueRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_REQUEST", "message": "invalid queue request"},
		})
		return
	}
	result, err := h.consumptionService.SetQueueWithResult(
		episodeID,
		request.QueueState,
		services.QueueWriteOptions{AcknowledgeFocusLimit: request.AcknowledgeFocusLimit},
	)
	var focusErr *services.FocusLimitConfirmationError
	switch {
	case errors.Is(err, services.ErrInvalidQueueState):
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_QUEUE_STATE", "message": "invalid queue state"},
		})
		return
	case errors.Is(err, services.ErrConsumptionEpisodeNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   gin.H{"code": "EPISODE_NOT_FOUND", "message": "episode not found"},
		})
		return
	case errors.As(err, &focusErr):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":          "FOCUS_LIMIT_CONFIRMATION_REQUIRED",
				"message":       "Focus soft limit confirmation required",
				"current_count": focusErr.CurrentCount,
				"focus_limit":   services.FocusSoftLimit,
			},
		})
		return
	case err != nil:
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to update queue state")
		return
	}
	h.writeConsumptionItemWithUndo(c, episodeID, result.CompletionUndo)
}

func (h *DiscoveryHandler) PutPlacement(c *gin.Context) {
	episodeID, ok := parseConsumptionEpisodeID(c)
	if !ok {
		return
	}
	var request putPlacementRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_REQUEST", "message": "invalid queue placement request"},
		})
		return
	}
	result, err := h.consumptionService.PlaceQueue(
		episodeID,
		request.QueueState,
		services.QueuePlacementOptions{
			BeforeEpisodeID:       request.BeforeEpisodeID,
			ExpectedRevisions:     request.ExpectedRevisions,
			AcknowledgeFocusLimit: request.AcknowledgeFocusLimit,
		},
	)
	var focusErr *services.FocusLimitConfirmationError
	switch {
	case errors.Is(err, services.ErrInvalidQueueState):
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_QUEUE_STATE", "message": "invalid queue state"},
		})
		return
	case errors.Is(err, services.ErrInvalidQueuePlacement):
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_QUEUE_PLACEMENT", "message": "invalid queue placement"},
		})
		return
	case errors.Is(err, services.ErrConsumptionEpisodeNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   gin.H{"code": "EPISODE_NOT_FOUND", "message": "episode not found"},
		})
		return
	case errors.Is(err, services.ErrQueueOrderConflict):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "QUEUE_ORDER_CONFLICT",
				"message": "Consumption queue order changed; reload and try again",
			},
		})
		return
	case errors.As(err, &focusErr):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":          "FOCUS_LIMIT_CONFIRMATION_REQUIRED",
				"message":       "Focus soft limit confirmation required",
				"current_count": focusErr.CurrentCount,
				"focus_limit":   services.FocusSoftLimit,
			},
		})
		return
	case err != nil:
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to place consumption episode")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *DiscoveryHandler) DeleteQueue(c *gin.Context) {
	episodeID, ok := parseConsumptionEpisodeID(c)
	if !ok {
		return
	}
	if _, err := h.consumptionService.ClearQueue(episodeID); err != nil {
		h.writeConsumptionError(c, err, "Failed to clear queue state")
		return
	}
	h.writeConsumptionItem(c, episodeID)
}

func (h *DiscoveryHandler) PutDismissed(c *gin.Context) {
	episodeID, ok := parseConsumptionEpisodeID(c)
	if !ok {
		return
	}
	var request putDismissedRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_REQUEST", "message": "invalid dismissed request"},
		})
		return
	}
	if _, err := h.consumptionService.SetDismissed(episodeID, request.Dismissed); err != nil {
		h.writeConsumptionError(c, err, "Failed to update dismissed state")
		return
	}
	h.writeConsumptionItem(c, episodeID)
}

func (h *DiscoveryHandler) MarkRead(c *gin.Context) {
	episodeID, ok := parseConsumptionEpisodeID(c)
	if !ok {
		return
	}
	if _, err := h.consumptionService.MarkRead(episodeID); err != nil {
		h.writeConsumptionError(c, err, "Failed to mark episode as read")
		return
	}
	h.writeConsumptionItem(c, episodeID)
}

func (h *DiscoveryHandler) MarkInProgress(c *gin.Context) {
	episodeID, ok := parseConsumptionEpisodeID(c)
	if !ok {
		return
	}
	if _, err := h.consumptionService.MarkInProgress(episodeID); err != nil {
		h.writeConsumptionError(c, err, "Failed to mark episode in progress")
		return
	}
	h.writeConsumptionItem(c, episodeID)
}

func (h *DiscoveryHandler) UndoCompletion(c *gin.Context) {
	episodeID, ok := parseConsumptionEpisodeID(c)
	if !ok {
		return
	}
	var request undoCompletionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_REQUEST", "message": "invalid completion undo request"},
		})
		return
	}
	result, err := h.consumptionService.UndoCompletion(episodeID, request.Token)
	switch {
	case errors.Is(err, services.ErrInvalidCompletionUndo):
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_COMPLETION_UNDO", "message": "completion undo token is invalid"},
		})
		return
	case errors.Is(err, services.ErrCompletionUndoExpired):
		c.JSON(http.StatusGone, gin.H{
			"success": false,
			"error":   gin.H{"code": "COMPLETION_UNDO_EXPIRED", "message": "completion undo has expired"},
		})
		return
	case errors.Is(err, services.ErrCompletionUndoConflict),
		errors.Is(err, services.ErrQueueOrderConflict):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "COMPLETION_UNDO_CONFLICT",
				"message": "Consumption state changed; reload and reprocess the episode",
			},
		})
		return
	case errors.Is(err, services.ErrConsumptionEpisodeNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   gin.H{"code": "EPISODE_NOT_FOUND", "message": "episode not found"},
		})
		return
	case err != nil:
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to undo completion")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func parseConsumptionEpisodeID(c *gin.Context) (uint, bool) {
	episodeID, err := strconv.ParseUint(c.Param("episodeID"), 10, 64)
	if err != nil || episodeID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_EPISODE_ID", "message": "invalid episode id"},
		})
		return 0, false
	}
	return uint(episodeID), true
}

func (h *DiscoveryHandler) writeConsumptionItem(c *gin.Context, episodeID uint) {
	h.writeConsumptionItemWithUndo(c, episodeID, nil)
}

func (h *DiscoveryHandler) writeConsumptionItemWithUndo(
	c *gin.Context,
	episodeID uint,
	undo *services.CompletionUndo,
) {
	item, err := h.consumptionService.GetItem(episodeID)
	if err != nil {
		h.writeConsumptionError(c, err, "Failed to read updated consumption state")
		return
	}
	item.CompletionUndo = undo
	c.JSON(http.StatusOK, gin.H{"success": true, "data": item})
}

func (h *DiscoveryHandler) writeConsumptionError(c *gin.Context, err error, message string) {
	if errors.Is(err, services.ErrConsumptionEpisodeNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   gin.H{"code": "EPISODE_NOT_FOUND", "message": "episode not found"},
		})
		return
	}
	middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", message)
}
