package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTokenTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewTokenHandler(nil)
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Next()
	})
	router.GET("/api/v1/tokens", handler.List)
	router.POST("/api/v1/tokens", handler.Create)
	router.DELETE("/api/v1/tokens/:id", handler.Revoke)
	return router
}

func TestTokenCreateRejectsInvalidExpiry(t *testing.T) {
	router := newTokenTestRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", strings.NewReader(`{"name":"ci","expires_in_days":0}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("TokenCreate returned %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestTokenCreateRequiresName(t *testing.T) {
	router := newTokenTestRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("TokenCreate returned %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestTokenEndpointsRequireAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewTokenHandler(nil)
	router.GET("/api/v1/tokens", handler.List)
	router.POST("/api/v1/tokens", handler.Create)
	router.DELETE("/api/v1/tokens/:id", handler.Revoke)

	for name, req := range map[string]*http.Request{
		"list":   httptest.NewRequest(http.MethodGet, "/api/v1/tokens", nil),
		"create": httptest.NewRequest(http.MethodPost, "/api/v1/tokens", strings.NewReader(`{"name":"ci"}`)),
		"revoke": httptest.NewRequest(http.MethodDelete, "/api/v1/tokens/tok-1", nil),
	} {
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("Token %s returned %d, want %d", name, rec.Code, http.StatusUnauthorized)
		}
	}
}
