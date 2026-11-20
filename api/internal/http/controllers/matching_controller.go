package controllers

import (
	"context"
	"net/http"
	"paycore/api/internal/http/requests"
	"strconv"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/http/responses"
	"paycore/api/internal/models"
)

type Matching interface {
	Report(context.Context, string, string, int, int) (models.MatchingReport, error)
	Configure(context.Context, string, string, models.MatchingConfigRequest) error
	Review(context.Context, string, string, int, models.MatchingReviewRequest) error
}
type MatchingController struct {
	svc Matching
}

func NewMatchingController(svc Matching) *MatchingController {
	return &MatchingController{svc}
}

func (h *MatchingController) Ready(c *gin.Context) {
	if h.svc == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Next()
}
func matchingID(c *gin.Context) bool {
	if !validUUID(c.Param("id")) {
		fail(c, models.ErrStatementNotFound)
		return false
	}
	return true
}
func (h *MatchingController) Report(c *gin.Context) {
	if !matchingID(c) {
		return
	}
	limit, offset, ok := page(c, 50, 100)
	if !ok {
		return
	}
	report, err := h.svc.Report(c.Request.Context(), sessionUser(c).ID, c.Param("id"), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, report)
}
func (h *MatchingController) Configure(c *gin.Context) {
	if !matchingID(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req models.MatchingConfigRequest
	if err := requests.BindJSON(c, &req); err != nil {
		fail(c, models.ErrMatchInvalid)
		return
	}
	if err := h.svc.Configure(c.Request.Context(), sessionUser(c).ID, c.Param("id"), req); err != nil {
		fail(c, err)
		return
	}
	responses.NoContent(c)
}
func (h *MatchingController) Review(c *gin.Context) {
	if !matchingID(c) {
		return
	}
	number, err := strconv.Atoi(c.Param("row"))
	if err != nil || number < 1 {
		fail(c, models.ErrMatchInvalid)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req models.MatchingReviewRequest
	if err := requests.BindJSON(c, &req); err != nil {
		fail(c, models.ErrMatchInvalid)
		return
	}
	if err := h.svc.Review(c.Request.Context(), sessionUser(c).ID, c.Param("id"), number, req); err != nil {
		fail(c, err)
		return
	}
	responses.NoContent(c)
}

// Guard returns the shared authentication and permission middleware provider.
