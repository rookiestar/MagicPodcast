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

	candidates, err := h.service.ListRecentCandidates(limit)
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
			candidates[index].DecisionState = decision.State
			candidates[index].DecisionUpdatedAt = &decision.DecidedAt
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    candidates,
	})
}

func (h *DiscoveryHandler) ListTodayShortlist(c *gin.Context) {
	shortlist, err := h.service.ListTodayShortlisted()
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch today's shortlist")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    shortlist,
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

type putDecisionRequest struct {
	State string `json:"state"`
}

func (h *DiscoveryHandler) PutDecision(c *gin.Context) {
	episodeID, err := strconv.ParseUint(c.Param("episodeID"), 10, 64)
	if err != nil || episodeID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_EPISODE_ID", "message": "invalid episode id"},
		})
		return
	}

	var request putDecisionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_REQUEST", "message": "invalid decision request"},
		})
		return
	}

	decision, err := h.triageService.SetDecision(uint(episodeID), request.State)
	switch {
	case errors.Is(err, services.ErrInvalidTriageState):
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_TRIAGE_STATE", "message": "invalid triage state"},
		})
		return
	case errors.Is(err, services.ErrTriageEpisodeNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   gin.H{"code": "EPISODE_NOT_FOUND", "message": "episode not found"},
		})
		return
	case err != nil:
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to persist triage decision")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"episode_id":          decision.EpisodeID,
			"state":               decision.State,
			"decision_updated_at": decision.DecidedAt,
		},
	})
}
