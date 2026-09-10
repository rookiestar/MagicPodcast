package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"magicpodcast/internal/middleware"
	"magicpodcast/internal/personidentity"

	"github.com/gin-gonic/gin"
)

type PersonHandler struct {
	module personidentity.Module
}

func NewPersonHandler(module personidentity.Module) *PersonHandler {
	return &PersonHandler{module: module}
}

type personNameCorrectionBody struct {
	DisplayName  string   `json:"display_name"`
	Aliases      []string `json:"aliases"`
	IdentityNote string   `json:"identity_note"`
}

type attributionCorrectionBody struct {
	SourceVersion    string `json:"source_version"`
	SourceKind       string `json:"source_kind"`
	FragmentOrder    int    `json:"fragment_order"`
	AssignedPersonID *uint  `json:"assigned_person_id"`
	Status           string `json:"status"`
}

func (h *PersonHandler) Prepare(c *gin.Context) {
	id, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	preparer, ok := h.module.(interface {
		PrepareCurrent(context.Context, uint) (personidentity.EpisodePeople, error)
	})
	if !ok {
		writePersonUnavailable(c)
		return
	}
	result, err := preparer.PrepareCurrent(c.Request.Context(), id)
	if err != nil {
		writePersonError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *PersonHandler) List(c *gin.Context) {
	episodeID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	if h.module == nil {
		writePersonUnavailable(c)
		return
	}
	result, err := h.module.ListEpisodePeople(c.Request.Context(), episodeID)
	if err != nil {
		writePersonError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *PersonHandler) CorrectName(c *gin.Context) {
	episodeID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	personID, ok := ParseUintParam(c, "personId")
	if !ok {
		return
	}
	if h.module == nil {
		writePersonUnavailable(c)
		return
	}
	var body personNameCorrectionBody
	if !decodeStrictJSON(c, &body) {
		return
	}
	result, err := h.module.CorrectName(c.Request.Context(), episodeID, personidentity.NameCorrection{
		PersonID:     personID,
		DisplayName:  body.DisplayName,
		Aliases:      body.Aliases,
		IdentityNote: body.IdentityNote,
	})
	if err != nil {
		writePersonError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *PersonHandler) CorrectAttribution(c *gin.Context) {
	episodeID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	if h.module == nil {
		writePersonUnavailable(c)
		return
	}
	var body attributionCorrectionBody
	if !decodeStrictJSON(c, &body) {
		return
	}
	result, err := h.module.CorrectAttribution(
		c.Request.Context(),
		episodeID,
		personidentity.AttributionCorrection{
			SourceKind:       body.SourceKind,
			SourceVersion:    body.SourceVersion,
			FragmentOrder:    body.FragmentOrder,
			AssignedPersonID: body.AssignedPersonID,
			Status:           body.Status,
		},
	)
	if err != nil {
		writePersonError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func decodeStrictJSON(c *gin.Context, dest any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		writeInvalidPersonRequest(c)
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeInvalidPersonRequest(c)
		return false
	}
	return true
}

func writeInvalidPersonRequest(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{
		"success": false,
		"error": gin.H{
			"code":    "INVALID_PERSON_REQUEST",
			"message": "request must contain a valid person or attribution correction",
		},
	})
}

func writePersonUnavailable(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"success": false,
		"error": gin.H{
			"code":    "PERSON_IDENTITY_UNAVAILABLE",
			"message": "person identity is not available on this runtime",
		},
	})
}

func writePersonError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, personidentity.ErrInvalidCorrection), errors.Is(err, personidentity.ErrTranscriptRequired):
		writeInvalidPersonRequest(c)
	case errors.Is(err, personidentity.ErrEpisodeNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "EPISODE_NOT_FOUND",
				"message": "episode not found",
			},
		})
	case errors.Is(err, personidentity.ErrPersonNotFound),
		errors.Is(err, personidentity.ErrNotEpisodeParticipant):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PERSON_NOT_FOUND",
				"message": "person is not an episode participant",
			},
		})
	default:
		middleware.InternalErrorResponseWithCode(
			c,
			"PERSON_REQUEST_FAILED",
			"person identity request failed",
		)
	}
}
