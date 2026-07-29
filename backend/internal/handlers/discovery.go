package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"magicpodcast/internal/middleware"
	"magicpodcast/internal/services"

	"github.com/gin-gonic/gin"
)

type DiscoveryHandler struct {
	service       *services.DiscoveryService
	triageService *services.TriageService
}

func NewDiscoveryHandler(
	service *services.DiscoveryService,
	triageService *services.TriageService,
) *DiscoveryHandler {
	return &DiscoveryHandler{
		service:       service,
		triageService: triageService,
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
