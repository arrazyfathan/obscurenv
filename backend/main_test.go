package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestDocsRoutesServeSpecAndUI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerDocsRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/docs/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /docs/openapi.yaml returned %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "openapi: 3.1.0") {
		t.Fatalf("openapi.yaml body does not look like an OpenAPI spec")
	}

	req = httptest.NewRequest(http.MethodGet, "/docs/", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /docs/ returned %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "swagger-ui") {
		t.Fatalf("/docs/ body does not reference swagger-ui")
	}
}
