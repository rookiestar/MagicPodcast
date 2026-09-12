package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"magicpodcast/internal/contentsearch"
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
	// Consume the bounded request body before long-running work. Otherwise
	// net/http cannot detect a disconnected HTTP/1 client while the unread POST
	// body prevents its background connection read, so Context never cancels.
	if _, err := io.Copy(io.Discard, http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)); err != nil {
		writeInvalidPersonRequest(c)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 150*time.Second)
	defer cancel()
	result, err := preparer.PrepareCurrent(ctx, id)
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
			"message": "请检查姓名、角色和匹配范围；同一片段不能同时归属多人。",
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
	case errors.Is(err, personidentity.ErrSourcesChanged), errors.Is(err, contentsearch.ErrStaleDocument):
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{"code": "PERSON_SOURCE_CHANGED", "message": "人物资料或逐字稿已更新，请重新读取后核对。"}})
	case errors.Is(err, personidentity.ErrIdentityUnavailable):
		writePersonUnavailable(c)
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

func (h *PersonHandler) CorrectAppearance(c *gin.Context) {
	episodeID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	personID, ok := ParseUintParam(c, "personId")
	if !ok {
		return
	}
	corrector, ok := h.module.(interface {
		CorrectAppearance(context.Context, uint, personidentity.AppearanceCorrection) (personidentity.EpisodePeople, error)
	})
	if !ok {
		writePersonUnavailable(c)
		return
	}
	var body struct {
		Role     *string `json:"role"`
		Excluded *bool   `json:"excluded"`
	}
	if !decodeStrictJSON(c, &body) {
		return
	}
	result, err := corrector.CorrectAppearance(c.Request.Context(), episodeID, personidentity.AppearanceCorrection{PersonID: personID, Role: body.Role, Excluded: body.Excluded})
	if err != nil {
		writePersonError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *PersonHandler) Review(c *gin.Context)      { h.review(c, false) }
func (h *PersonHandler) ApplyReview(c *gin.Context) { h.review(c, true) }
func (h *PersonHandler) review(c *gin.Context, apply bool) {
	id, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	reviewer, ok := h.module.(interface {
		Review(context.Context, uint, personidentity.ReviewRequest, bool) (personidentity.EpisodePeople, error)
	})
	if !ok {
		writePersonUnavailable(c)
		return
	}
	var body personidentity.ReviewRequest
	if !decodeStrictJSON(c, &body) {
		return
	}
	result, err := reviewer.Review(c.Request.Context(), id, body, apply)
	if err != nil {
		writePersonError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
func (h *PersonHandler) Manual(c *gin.Context) {
	id, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	reviewer, ok := h.module.(interface {
		ApplyManual(context.Context, uint, personidentity.ManualMatch) (personidentity.EpisodePeople, error)
	})
	if !ok {
		writePersonUnavailable(c)
		return
	}
	var body personidentity.ManualMatch
	if !decodeStrictJSON(c, &body) {
		return
	}
	result, err := reviewer.ApplyManual(c.Request.Context(), id, body)
	if err != nil {
		writePersonError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
func (h *PersonHandler) ReviewHistory(c *gin.Context) {
	id, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	reviewer, ok := h.module.(interface {
		ReviewHistory(context.Context, uint) ([]personidentity.ReviewDraft, error)
	})
	if !ok {
		writePersonUnavailable(c)
		return
	}
	result, err := reviewer.ReviewHistory(c.Request.Context(), id)
	if err != nil {
		writePersonError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
