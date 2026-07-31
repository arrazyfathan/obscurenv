package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestListenAddrUsesAddrWhenSet(t *testing.T) {
	t.Setenv("ADDR", ":9090")
	t.Setenv("PORT", "3000")

	if got := listenAddr(); got != ":9090" {
		t.Fatalf("listenAddr() = %q, want %q", got, ":9090")
	}
}

func TestListenAddrUsesVercelPort(t *testing.T) {
	t.Setenv("ADDR", "")
	t.Setenv("PORT", "3000")

	if got := listenAddr(); got != ":3000" {
		t.Fatalf("listenAddr() = %q, want %q", got, ":3000")
	}
}

func TestListenAddrDefaultsToLocalPort(t *testing.T) {
	t.Setenv("ADDR", "")
	t.Setenv("PORT", "")

	if got := listenAddr(); got != ":8080" {
		t.Fatalf("listenAddr() = %q, want %q", got, ":8080")
	}
}

func TestHealthRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerHealthRoutes(router)

	for _, path := range []string{"/", "/healthz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s returned %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}
