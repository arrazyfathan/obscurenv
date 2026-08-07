package handlers

import (
	"crypto/sha256"
	"encoding/hex"
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

func TestPushRejectsNonEnvelopePayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewEnvHandler(nil)
	router.POST("/api/v1/env/push", handler.Push)

	raw := "1a2b3c4d5e6f7g8h9i0j"
	sum := sha256.Sum256([]byte(raw))
	body := `{"project_slug":"app","environment":"production","encrypted_payload":"` + raw + `","checksum":"` + hex.EncodeToString(sum[:]) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/env/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Push returned %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "encrypted payload is not a valid envelope") {
		t.Fatalf("Push body = %q, want envelope validation error", rec.Body.String())
	}
}

func TestValidEnvelope(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    bool
	}{
		{"v2 passphrase envelope", `{"version":2,"kdf":"argon2id","ciphertext":"YWJj","key_slots":{"passphrase":{"kdf":"argon2id","salt":"c2FsdA==","wrapped_key":"d3JhcHBlZA=="}}}`, true},
		{"v1 envelope", `{"version":1,"kdf":"argon2id","salt":"c2FsdA==","ciphertext":"YWJj"}`, true},
		{"raw base64", "1a2b3c4d5e6f7g8h9i0j", false},
		{"empty object", `{}`, false},
		{"missing ciphertext", `{"version":2,"ciphertext":""}`, false},
		{"not json", "hello world", false},
		{"unsupported version", `{"version":9,"ciphertext":"YWJj"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validEnvelope(tt.payload); got != tt.want {
				t.Fatalf("validEnvelope(%q) = %v, want %v", tt.payload, got, tt.want)
			}
		})
	}
}

func TestDeleteEnvironmentRequiresProjectAndEnvironment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewEnvHandler(nil)
	router.DELETE("/api/v1/env", handler.Delete)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/env?project=app", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Delete returned %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "project and environment are required") {
		t.Fatalf("Delete body = %q, want missing parameter error", rec.Body.String())
	}
}

func TestExportRequiresProject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewEnvHandler(nil)
	router.GET("/api/v1/env/export", handler.Export)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/env/export", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Export returned %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "project is required") {
		t.Fatalf("Export body = %q, want missing project error", rec.Body.String())
	}
}
