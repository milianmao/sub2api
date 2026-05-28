package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const defaultCardMailboxPageSize = 20

type CardMailboxHandler struct {
	service *service.CardMailboxService
}

func NewCardMailboxHandler(service *service.CardMailboxService) *CardMailboxHandler {
	return &CardMailboxHandler{service: service}
}

type cardMailboxImportRequest struct {
	Content string `json:"content" binding:"required"`
}

type cardMailboxExportRequest struct {
	IDs []int64 `json:"ids" binding:"required"`
}

type cardMailboxResponse struct {
	ID            int64      `json:"id"`
	Email         string     `json:"email"`
	LastCode      string     `json:"last_code"`
	LastStatus    string     `json:"last_status"`
	LastError     string     `json:"last_error,omitempty"`
	LastFetchedAt *time.Time `json:"last_fetched_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type cardMailboxImportResultResponse struct {
	Imported int                              `json:"imported"`
	Failed   int                              `json:"failed"`
	Errors   []service.CardMailboxImportError `json:"errors"`
}

type cardMailboxFetchResponse struct {
	Email      string    `json:"email"`
	Code       string    `json:"code"`
	Status     string    `json:"status"`
	FetchedAt  time.Time `json:"fetched_at"`
	Source     string    `json:"source"`
	Subject    string    `json:"subject"`
	From       string    `json:"from"`
	ReceivedAt time.Time `json:"received_at"`
	Snippet    string    `json:"snippet"`
}

func (h *CardMailboxHandler) List(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "card mailbox service unavailable")
		return
	}
	page, pageSize := response.ParsePagination(c)
	if pageSize <= 0 {
		pageSize = defaultCardMailboxPageSize
	}
	items, total, err := h.service.List(c.Request.Context(), service.CardMailboxListFilter{
		Email:  strings.TrimSpace(c.Query("search")),
		Status: strings.TrimSpace(c.Query("status")),
		Limit:  pageSize,
		Offset: (page - 1) * pageSize,
	})
	if writeCardMailboxError(c, err) {
		return
	}
	responses := make([]cardMailboxResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, buildCardMailboxResponse(item))
	}
	response.Paginated(c, responses, int64(total), page, pageSize)
}

func (h *CardMailboxHandler) Import(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "card mailbox service unavailable")
		return
	}
	var req cardMailboxImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_REQUEST", "invalid request body"))
		return
	}
	result, err := h.service.ImportJSONL(c.Request.Context(), bytes.NewBufferString(req.Content))
	if writeCardMailboxError(c, err) {
		return
	}
	response.Success(c, buildCardMailboxImportResultResponse(result))
}

func (h *CardMailboxHandler) FetchCode(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "card mailbox service unavailable")
		return
	}
	id, ok := parseCardMailboxIDParam(c)
	if !ok {
		return
	}
	result, err := h.service.FetchCode(c.Request.Context(), id)
	if writeCardMailboxError(c, err) {
		return
	}
	response.Success(c, buildCardMailboxFetchResponse(result))
}

func (h *CardMailboxHandler) Export(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "card mailbox service unavailable")
		return
	}
	var req cardMailboxExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_REQUEST", "invalid request body"))
		return
	}
	items, err := h.service.ExportByIDs(c.Request.Context(), req.IDs)
	if writeCardMailboxError(c, err) {
		return
	}
	payload, err := json.MarshalIndent(gin.H{"items": items}, "", "  ")
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("CARD_MAILBOX_EXPORT_FAILED", "failed to encode export payload"))
		return
	}
	c.Header("Content-Disposition", `attachment; filename="card-mailboxes-export.json"`)
	c.Data(http.StatusOK, "application/json; charset=utf-8", payload)
}

func (h *CardMailboxHandler) Delete(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "card mailbox service unavailable")
		return
	}
	id, ok := parseCardMailboxIDParam(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); writeCardMailboxError(c, err) {
		return
	}
	response.Success(c, gin.H{"success": true, "count": 1})
}

func parseCardMailboxIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_ID", "invalid card mailbox id"))
		return 0, false
	}
	return id, true
}

func writeCardMailboxError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	response.ErrorFrom(c, infraerrors.FromError(err).WithCause(nil).WithMetadata(nil).WithMetadata(map[string]string{"detail": service.RedactCardMailboxSensitive(err.Error())}))
	return true
}

func buildCardMailboxResponse(mailbox *service.CardMailbox) cardMailboxResponse {
	masked := service.MaskCardMailbox(mailbox)
	if masked == nil {
		return cardMailboxResponse{}
	}
	return cardMailboxResponse{
		ID:            masked.ID,
		Email:         masked.Email,
		LastCode:      masked.LastCode,
		LastStatus:    masked.LastStatus,
		LastError:     service.RedactCardMailboxSensitive(masked.LastError),
		LastFetchedAt: masked.LastFetchedAt,
		CreatedAt:     masked.CreatedAt,
		UpdatedAt:     masked.UpdatedAt,
	}
}

func buildCardMailboxImportResultResponse(result *service.CardMailboxImportResult) cardMailboxImportResultResponse {
	if result == nil {
		return cardMailboxImportResultResponse{}
	}
	return cardMailboxImportResultResponse{Imported: result.Imported, Failed: result.Failed, Errors: result.Errors}
}

func buildCardMailboxFetchResponse(result *service.CardMailboxFetchResult) cardMailboxFetchResponse {
	if result == nil {
		return cardMailboxFetchResponse{}
	}
	return cardMailboxFetchResponse{
		Email:      result.Email,
		Code:       result.Code,
		Status:     result.Status,
		FetchedAt:  result.FetchedAt,
		Source:     result.Source,
		Subject:    service.RedactCardMailboxSensitive(result.Subject),
		From:       result.From,
		ReceivedAt: result.ReceivedAt,
		Snippet:    service.RedactCardMailboxSensitive(result.Snippet),
	}
}
