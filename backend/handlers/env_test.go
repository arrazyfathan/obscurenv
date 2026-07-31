package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPushRejectsChecksumMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewEnvHandler(nil)
	router.POST("/api/v1/env/push", handler.Push)

	body := `{"project_slug":"app","environment":"production","encrypted_payload":"payload","checksum":"0000000000000000000000000000000000000000000000000000000000000000"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/env/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Push returned %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "checksum does not match encrypted payload") {
		t.Fatalf("Push body = %q, want checksum mismatch error", rec.Body.String())
	}
}
