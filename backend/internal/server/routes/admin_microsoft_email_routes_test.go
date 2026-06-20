package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	roothandler "github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterAdminRoutes_RegistersMicrosoftEmailRoutesWithoutBatchFetchCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")

	h := &roothandler.Handlers{
		Admin: &roothandler.AdminHandlers{
			MicrosoftEmail: adminhandler.NewMicrosoftEmailHandler(nil),
		},
	}

	RegisterAdminRoutes(v1, h, middleware.AdminAuthMiddleware(func(c *gin.Context) {
		c.Next()
	}), nil)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/admin/microsoft-emails"},
		{http.MethodPost, "/api/v1/admin/microsoft-emails/import"},
		{http.MethodPost, "/api/v1/admin/microsoft-emails/1/check"},
		{http.MethodPost, "/api/v1/admin/microsoft-emails/batch-check"},
		{http.MethodPost, "/api/v1/admin/microsoft-emails/1/fetch-code"},
		{http.MethodDelete, "/api/v1/admin/microsoft-emails/1"},
		{http.MethodPost, "/api/v1/admin/microsoft-emails/batch-delete"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.NotEqualf(t, http.StatusNotFound, rec.Code, "%s %s should be registered", tc.method, tc.path)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/microsoft-emails/batch-fetch-code", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
