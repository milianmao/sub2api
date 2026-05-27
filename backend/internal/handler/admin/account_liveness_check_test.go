package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type livenessAdminService struct {
	*stubAdminService
	accountsByID map[int64]*service.Account
	listed       []service.Account
	clearIDs     []int64
	setErrors    map[int64]string
}

func newLivenessAdminService() *livenessAdminService {
	return &livenessAdminService{
		stubAdminService: newStubAdminService(),
		accountsByID:     map[int64]*service.Account{},
		setErrors:        map[int64]string{},
	}
}

func (s *livenessAdminService) GetAccountsByIDs(_ context.Context, ids []int64) ([]*service.Account, error) {
	result := make([]*service.Account, 0, len(ids))
	for _, id := range ids {
		if acc := s.accountsByID[id]; acc != nil {
			copy := *acc
			result = append(result, &copy)
		}
	}
	return result, nil
}

func (s *livenessAdminService) ListAccounts(_ context.Context, page, pageSize int, platform, accountType, status, search string, groupID int64, privacyMode string, sortBy, sortOrder string) ([]service.Account, int64, error) {
	return s.listed, int64(len(s.listed)), nil
}

func (s *livenessAdminService) ClearAccountError(_ context.Context, id int64) (*service.Account, error) {
	s.clearIDs = append(s.clearIDs, id)
	acc := s.accountsByID[id]
	if acc == nil {
		return nil, errors.New("account not found")
	}
	copy := *acc
	copy.Status = service.StatusActive
	copy.ErrorMessage = ""
	s.accountsByID[id] = &copy
	return &copy, nil
}

func (s *livenessAdminService) SetAccountError(_ context.Context, id int64, errorMsg string) error {
	s.setErrors[id] = errorMsg
	acc := s.accountsByID[id]
	if acc == nil {
		return errors.New("account not found")
	}
	copy := *acc
	copy.Status = service.StatusError
	copy.ErrorMessage = errorMsg
	copy.Schedulable = false
	s.accountsByID[id] = &copy
	return nil
}

type livenessTestRunner struct {
	results map[int64]*service.ScheduledTestResult
}

func (r livenessTestRunner) RunTestBackground(_ context.Context, accountID int64, _ string) (*service.ScheduledTestResult, error) {
	if result := r.results[accountID]; result != nil {
		return result, nil
	}
	return &service.ScheduledTestResult{
		Status:       "failed",
		ErrorMessage: "missing test result",
		LatencyMs:    0,
		StartedAt:    time.Now(),
		FinishedAt:   time.Now(),
	}, nil
}

func setupLivenessRouter(adminSvc service.AdminService, runner accountLivenessTestRunner) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.accountLivenessTestRunner = runner
	router.POST("/api/v1/admin/accounts/liveness-check", handler.LivenessCheck)
	return router
}

func TestAccountHandlerLivenessCheckSelectedUpdatesStatusesAndAggregates(t *testing.T) {
	adminSvc := newLivenessAdminService()
	adminSvc.accountsByID[1] = &service.Account{ID: 1, Name: "claude-ok", Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth, Status: service.StatusError, Schedulable: true}
	adminSvc.accountsByID[2] = &service.Account{ID: 2, Name: "openai-bad", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true}

	router := setupLivenessRouter(adminSvc, livenessTestRunner{results: map[int64]*service.ScheduledTestResult{
		1: {Status: "success", LatencyMs: 120, StartedAt: time.Now(), FinishedAt: time.Now()},
		2: {Status: "failed", ErrorMessage: "401 unauthorized token abc123", LatencyMs: 55, StartedAt: time.Now(), FinishedAt: time.Now()},
	}})

	body := bytes.NewBufferString(`{"scope":"selected","account_ids":[1,2,2],"concurrency":3}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/liveness-check", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data AccountLivenessCheckResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 2, resp.Data.Total)
	require.Equal(t, 2, resp.Data.Completed)
	require.Equal(t, 1, resp.Data.Success)
	require.Equal(t, 1, resp.Data.Failed)
	require.Equal(t, 0, resp.Data.Skipped)
	require.Equal(t, int64(120), resp.Data.AverageLatencyMs)
	require.Equal(t, 1, resp.Data.ByPlatform[service.PlatformAnthropic].Success)
	require.Equal(t, 1, resp.Data.ByPlatform[service.PlatformOpenAI].Failed)
	require.Equal(t, 1, resp.Data.FailureReasons["auth"])
	require.Equal(t, []int64{1}, adminSvc.clearIDs)
	require.Contains(t, adminSvc.setErrors[2], "401 unauthorized")
	require.NotContains(t, adminSvc.setErrors[2], "abc123")
	require.Len(t, resp.Data.Items, 2)
	require.Equal(t, "active", resp.Data.Items[0].StatusAfter)
	require.Equal(t, "error", resp.Data.Items[1].StatusAfter)
}

func TestAccountHandlerLivenessCheckFilteredUsesAccountFilters(t *testing.T) {
	adminSvc := newLivenessAdminService()
	adminSvc.listed = []service.Account{
		{ID: 3, Name: "gemini-ok", Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true},
	}
	adminSvc.accountsByID[3] = &adminSvc.listed[0]

	router := setupLivenessRouter(adminSvc, livenessTestRunner{results: map[int64]*service.ScheduledTestResult{
		3: {Status: "success", LatencyMs: 90, StartedAt: time.Now(), FinishedAt: time.Now()},
	}})

	body := bytes.NewBufferString(`{"scope":"filtered","filters":{"platform":"gemini","status":"active","group":"0"}}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/liveness-check", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data AccountLivenessCheckResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.Data.Total)
	require.Equal(t, 1, resp.Data.Success)
	require.Equal(t, "gemini-ok", resp.Data.Items[0].AccountName)
}

func TestAccountHandlerLivenessCheckRejectsEmptySelectedScope(t *testing.T) {
	router := setupLivenessRouter(newLivenessAdminService(), livenessTestRunner{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/liveness-check", bytes.NewBufferString(`{"scope":"selected","account_ids":[]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "account_ids is required")
}
