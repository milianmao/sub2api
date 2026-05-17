package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupGroupAuthorizationAdminRouter(adminSvc *stubAdminService, role string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUserRole), role)
		c.Next()
	})

	userHandler := NewUserHandler(adminSvc, nil)
	groupHandler := NewGroupHandler(adminSvc, nil, nil)
	apiKeyHandler := NewAdminAPIKeyHandler(adminSvc)

	router.POST("/api/v1/admin/users", userHandler.Create)
	router.PUT("/api/v1/admin/users/:id", userHandler.Update)
	router.POST("/api/v1/admin/groups", groupHandler.Create)
	router.PUT("/api/v1/admin/groups/:id", groupHandler.Update)
	router.PUT("/api/v1/admin/api-keys/:id", apiKeyHandler.UpdateGroup)
	return router
}

func TestUserHandlerCreateSuperAdminCanSetLevelAndAllowedGroups(t *testing.T) {
	svc := newStubAdminService()
	router := setupGroupAuthorizationAdminRouter(svc, service.RoleSuperAdmin)
	body := map[string]any{
		"email":          "new@example.com",
		"password":       "pass123",
		"level":          3,
		"allowed_groups": []int64{10, 20},
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, svc.createdUsers, 1)
	require.Equal(t, 3, svc.createdUsers[0].Level)
	require.Equal(t, []int64{10, 20}, svc.createdUsers[0].AllowedGroups)
}

func TestUserHandlerCreateAdminCanSetLevel(t *testing.T) {
	svc := newStubAdminService()
	router := setupGroupAuthorizationAdminRouter(svc, service.RoleAdmin)
	body := `{"email":"new@example.com","password":"pass123","level":3}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, svc.createdUsers, 1)
	require.Equal(t, 3, svc.createdUsers[0].Level)
}

func TestUserHandlerCreateAdminCannotSetAllowedGroupsOrRole(t *testing.T) {
	svc := newStubAdminService()
	router := setupGroupAuthorizationAdminRouter(svc, service.RoleAdmin)
	body := `{"email":"new@example.com","password":"pass123","allowed_groups":[10],"role":"admin"}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Empty(t, svc.createdUsers)
}

func TestUserHandlerUpdateSuperAdminCanSetLevelAndAllowedGroups(t *testing.T) {
	svc := newStubAdminService()
	router := setupGroupAuthorizationAdminRouter(svc, service.RoleSuperAdmin)
	body := `{"level":4,"allowed_groups":[30]}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/7", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, svc.updatedUsers, 1)
	require.NotNil(t, svc.updatedUsers[0].Level)
	require.Equal(t, 4, *svc.updatedUsers[0].Level)
	require.NotNil(t, svc.updatedUsers[0].AllowedGroups)
	require.Equal(t, []int64{30}, *svc.updatedUsers[0].AllowedGroups)
}

func TestUserHandlerUpdateAdminCanSetLevel(t *testing.T) {
	svc := newStubAdminService()
	router := setupGroupAuthorizationAdminRouter(svc, service.RoleAdmin)
	body := `{"level":4}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/7", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, svc.updatedUsers, 1)
	require.NotNil(t, svc.updatedUsers[0].Level)
	require.Equal(t, 4, *svc.updatedUsers[0].Level)
}

func TestUserHandlerUpdateAdminCannotSetAllowedGroupsOrRole(t *testing.T) {
	svc := newStubAdminService()
	router := setupGroupAuthorizationAdminRouter(svc, service.RoleAdmin)
	body := `{"allowed_groups":[30],"role":"admin"}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/7", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Empty(t, svc.updatedUsers)
}

func TestGroupHandlerCreateSuperAdminCanSetAuthorizationFields(t *testing.T) {
	svc := newStubAdminService()
	router := setupGroupAuthorizationAdminRouter(svc, service.RoleSuperAdmin)
	body := `{"name":"restricted","platform":"anthropic","access_mode":"restricted","min_user_level":2,"visible_user_ids":[7,8]}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/groups", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, svc.createdGroups, 1)
	require.Equal(t, service.GroupAccessModeRestricted, svc.createdGroups[0].AccessMode)
	require.Equal(t, 2, svc.createdGroups[0].MinUserLevel)
	require.Equal(t, []int64{7, 8}, svc.createdGroups[0].VisibleUserIDs)
}

func TestGroupHandlerCreateAdminCannotSetAuthorizationFields(t *testing.T) {
	svc := newStubAdminService()
	router := setupGroupAuthorizationAdminRouter(svc, service.RoleAdmin)
	body := `{"name":"restricted","platform":"anthropic","access_mode":"restricted","min_user_level":2}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/groups", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Empty(t, svc.createdGroups)
}

func TestGroupHandlerUpdateSuperAdminCanSetAuthorizationFields(t *testing.T) {
	svc := newStubAdminService()
	router := setupGroupAuthorizationAdminRouter(svc, service.RoleSuperAdmin)
	body := `{"access_mode":"public","min_user_level":5,"visible_user_ids":[7]}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/groups/9", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, svc.updatedGroups, 1)
	require.NotNil(t, svc.updatedGroups[0].AccessMode)
	require.Equal(t, service.GroupAccessModePublic, *svc.updatedGroups[0].AccessMode)
	require.NotNil(t, svc.updatedGroups[0].MinUserLevel)
	require.Equal(t, 5, *svc.updatedGroups[0].MinUserLevel)
	require.NotNil(t, svc.updatedGroups[0].VisibleUserIDs)
	require.Equal(t, []int64{7}, *svc.updatedGroups[0].VisibleUserIDs)
}

func TestGroupHandlerUpdateAdminCannotSetAuthorizationFields(t *testing.T) {
	svc := newStubAdminService()
	router := setupGroupAuthorizationAdminRouter(svc, service.RoleAdmin)
	body := `{"access_mode":"public","min_user_level":5}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/groups/9", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Empty(t, svc.updatedGroups)
}

func TestGroupHandlerCreateAdminCannotSetExclusiveGroup(t *testing.T) {
	svc := newStubAdminService()
	router := setupGroupAuthorizationAdminRouter(svc, service.RoleAdmin)
	body := `{"name":"exclusive","platform":"anthropic","is_exclusive":true}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/groups", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Empty(t, svc.createdGroups)
}

func TestGroupHandlerUpdateAdminCannotChangeExclusiveFlag(t *testing.T) {
	svc := newStubAdminService()
	router := setupGroupAuthorizationAdminRouter(svc, service.RoleAdmin)
	body := `{"is_exclusive":false}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/groups/9", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Empty(t, svc.updatedGroups)
}

func TestAdminAPIKeyHandlerUpdateGroupRequiresSuperAdmin(t *testing.T) {
	svc := newStubAdminService()
	router := setupGroupAuthorizationAdminRouter(svc, service.RoleAdmin)
	body := `{"group_id":2}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestAdminAPIKeyHandlerResetRateLimitDoesNotRequireSuperAdmin(t *testing.T) {
	svc := newStubAdminService()
	router := setupGroupAuthorizationAdminRouter(svc, service.RoleAdmin)
	body := `{"reset_rate_limit_usage":true}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}
