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

func TestRegisterAdminRoutes_RegistersChatGPTSessionImportRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")

	h := &roothandler.Handlers{
		Admin: &roothandler.AdminHandlers{
			Account: adminhandler.NewAccountHandler(
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
			),
		},
	}

	RegisterAdminRoutes(v1, h, middleware.AdminAuthMiddleware(func(c *gin.Context) {
		c.Next()
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/import/chatgpt-session", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.NotEqual(t, http.StatusNotFound, rec.Code)
}
