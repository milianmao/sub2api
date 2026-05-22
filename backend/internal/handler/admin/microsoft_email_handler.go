package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const defaultMicrosoftEmailPageSize = 20

type MicrosoftEmailHandler struct {
	service *service.MicrosoftEmailService
}

func NewMicrosoftEmailHandler(service *service.MicrosoftEmailService) *MicrosoftEmailHandler {
	return &MicrosoftEmailHandler{service: service}
}

type microsoftEmailImportRequest struct {
	Content string `json:"content" binding:"required"`
}

type microsoftEmailIDListRequest struct {
	IDs []int64 `json:"ids" binding:"required"`
}

type microsoftEmailAccountResponse struct {
	ID           int64      `json:"id"`
	Email        string     `json:"email"`
	Password     string     `json:"password"`
	ClientID     string     `json:"client_id"`
	RefreshToken string     `json:"refresh_token"`
	Status       string     `json:"status"`
	LastCheckAt  *time.Time `json:"last_check_at,omitempty"`
	LastFetchAt  *time.Time `json:"last_fetch_at,omitempty"`
	LastError    *string    `json:"last_error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type microsoftEmailImportResultResponse struct {
	Total   int                                 `json:"total"`
	Created int                                 `json:"created"`
	Updated int                                 `json:"updated"`
	Failed  int                                 `json:"failed"`
	Items   []microsoftEmailImportItem          `json:"items"`
	Errors  []service.MicrosoftEmailImportError `json:"errors"`
}

type microsoftEmailImportItem struct {
	Line    int                            `json:"line"`
	Email   string                         `json:"email"`
	Action  string                         `json:"action"`
	Account *microsoftEmailAccountResponse `json:"account,omitempty"`
}

type microsoftEmailCheckResultResponse struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Status    string    `json:"status"`
	CheckedAt time.Time `json:"checked_at"`
	LastError *string   `json:"last_error,omitempty"`
}

type microsoftEmailFetchCodeResponse struct {
	Email      string    `json:"email"`
	Code       string    `json:"code"`
	Source     string    `json:"source"`
	Subject    string    `json:"subject"`
	From       string    `json:"from"`
	ReceivedAt time.Time `json:"received_at"`
	Snippet    string    `json:"snippet"`
	Error      string    `json:"error"`
}

func (h *MicrosoftEmailHandler) List(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "Microsoft email service unavailable")
		return
	}
	page, pageSize := response.ParsePagination(c)
	if pageSize <= 0 {
		pageSize = defaultMicrosoftEmailPageSize
	}
	accounts, total, err := h.service.List(c.Request.Context(), service.MicrosoftEmailListFilter{
		Email:  strings.TrimSpace(c.Query("search")),
		Status: strings.TrimSpace(c.Query("status")),
		Limit:  pageSize,
		Offset: (page - 1) * pageSize,
	})
	if writeMicrosoftEmailError(c, err) {
		return
	}
	items := make([]microsoftEmailAccountResponse, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, buildMicrosoftEmailAccountResponse(account))
	}
	response.Paginated(c, items, int64(total), page, pageSize)
}

func (h *MicrosoftEmailHandler) Import(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "Microsoft email service unavailable")
		return
	}
	var req microsoftEmailImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_REQUEST", "invalid request body"))
		return
	}
	result, err := h.service.ImportTXT(c.Request.Context(), req.Content)
	if writeMicrosoftEmailError(c, err) {
		return
	}
	response.Success(c, buildMicrosoftEmailImportResultResponse(result))
}

func (h *MicrosoftEmailHandler) Check(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "Microsoft email service unavailable")
		return
	}
	id, ok := parseMicrosoftEmailIDParam(c)
	if !ok {
		return
	}
	result, err := h.service.Check(c.Request.Context(), id)
	if writeMicrosoftEmailError(c, err) {
		return
	}
	response.Success(c, buildMicrosoftEmailCheckResultResponse(result))
}

func (h *MicrosoftEmailHandler) BatchCheck(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "Microsoft email service unavailable")
		return
	}
	var req microsoftEmailIDListRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_REQUEST", "ids are required"))
		return
	}
	results := make([]microsoftEmailCheckResultResponse, 0, len(req.IDs))
	for _, id := range req.IDs {
		result, err := h.service.Check(c.Request.Context(), id)
		if err != nil {
			results = append(results, microsoftEmailCheckResultResponse{ID: id, Status: "error", LastError: stringPtr(microsoftEmailErrorMessage(err))})
			continue
		}
		results = append(results, buildMicrosoftEmailCheckResultResponse(result))
	}
	response.Success(c, gin.H{"items": results, "total": len(results)})
}

func (h *MicrosoftEmailHandler) FetchCode(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "Microsoft email service unavailable")
		return
	}
	id, ok := parseMicrosoftEmailIDParam(c)
	if !ok {
		return
	}
	result, err := h.service.FetchCode(c.Request.Context(), id)
	if writeMicrosoftEmailError(c, err) {
		return
	}
	response.Success(c, buildMicrosoftEmailFetchCodeResponse(result))
}

func (h *MicrosoftEmailHandler) Delete(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "Microsoft email service unavailable")
		return
	}
	id, ok := parseMicrosoftEmailIDParam(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); writeMicrosoftEmailError(c, err) {
		return
	}
	response.Success(c, gin.H{"success": true, "count": 1})
}

func (h *MicrosoftEmailHandler) BatchDelete(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "Microsoft email service unavailable")
		return
	}
	var req microsoftEmailIDListRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_REQUEST", "ids are required"))
		return
	}
	count, err := h.service.BatchDelete(c.Request.Context(), req.IDs)
	if writeMicrosoftEmailError(c, err) {
		return
	}
	response.Success(c, gin.H{"success": true, "count": count})
}

func parseMicrosoftEmailIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_ID", "invalid microsoft email id"))
		return 0, false
	}
	return id, true
}

func writeMicrosoftEmailError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, service.ErrMicrosoftEmailNotFound) {
		response.ErrorFrom(c, infraerrors.NotFound("MICROSOFT_EMAIL_NOT_FOUND", "microsoft email account not found"))
		return true
	}
	response.ErrorFrom(c, err)
	return true
}

func microsoftEmailErrorMessage(err error) string {
	if errors.Is(err, service.ErrMicrosoftEmailNotFound) {
		return "microsoft email account not found"
	}
	return err.Error()
}

func buildMicrosoftEmailAccountResponse(account *service.MicrosoftEmailAccount) microsoftEmailAccountResponse {
	masked := service.MaskMicrosoftEmailAccount(account)
	if masked == nil {
		return microsoftEmailAccountResponse{}
	}
	return microsoftEmailAccountResponse{
		ID:           masked.ID,
		Email:        masked.Email,
		Password:     masked.Password,
		ClientID:     masked.ClientID,
		RefreshToken: masked.RefreshToken,
		Status:       masked.Status,
		LastCheckAt:  masked.LastCheckAt,
		LastFetchAt:  masked.LastFetchAt,
		LastError:    masked.LastError,
		CreatedAt:    masked.CreatedAt,
		UpdatedAt:    masked.UpdatedAt,
	}
}

func buildMicrosoftEmailImportResultResponse(result *service.MicrosoftEmailImportResult) microsoftEmailImportResultResponse {
	if result == nil {
		return microsoftEmailImportResultResponse{}
	}
	items := make([]microsoftEmailImportItem, 0, len(result.Items))
	for _, item := range result.Items {
		var account *microsoftEmailAccountResponse
		if item.Account != nil {
			resp := buildMicrosoftEmailAccountResponse(item.Account)
			account = &resp
		}
		items = append(items, microsoftEmailImportItem{Line: item.Line, Email: item.Email, Action: item.Action, Account: account})
	}
	return microsoftEmailImportResultResponse{Total: result.Total, Created: result.Created, Updated: result.Updated, Failed: result.Failed, Items: items, Errors: result.Errors}
}

func buildMicrosoftEmailCheckResultResponse(result *service.MicrosoftEmailCheckResult) microsoftEmailCheckResultResponse {
	if result == nil {
		return microsoftEmailCheckResultResponse{}
	}
	return microsoftEmailCheckResultResponse{ID: result.ID, Email: result.Email, Status: result.Status, CheckedAt: result.CheckedAt, LastError: result.LastError}
}

func buildMicrosoftEmailFetchCodeResponse(result *service.MicrosoftEmailFetchCodeResult) microsoftEmailFetchCodeResponse {
	if result == nil {
		return microsoftEmailFetchCodeResponse{}
	}
	return microsoftEmailFetchCodeResponse{
		Email:      result.Email,
		Code:       result.Code,
		Source:     result.Source,
		Subject:    result.Subject,
		From:       result.From,
		ReceivedAt: result.ReceivedAt,
		Snippet:    result.Snippet,
		Error:      result.Error,
	}
}

func stringPtr(s string) *string {
	return &s
}
