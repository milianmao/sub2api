package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminOnlyAllowsAdminRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, role := range []string{service.RoleAdmin, service.RoleSuperAdmin} {
		t.Run(role, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(ContextKeyUserRole), role)
				c.Next()
			})
			router.Use(AdminOnly())
			router.GET("/t", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/t", nil)
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

func TestAdminOnlyRejectsNonAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyUserRole), service.RoleUser)
		c.Next()
	})
	router.Use(AdminOnly())
	router.GET("/t", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}
