package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
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
		{"wrong kdf", `{"version":2,"kdf":"scrypt","ciphertext":"YWJj","key_slots":{"passphrase":{"kdf":"argon2id","salt":"c2FsdA==","wrapped_key":"d3JhcHBlZA=="}}}`, false},
		{"v2 missing key slot", `{"version":2,"kdf":"argon2id","ciphertext":"YWJj"}`, false},
		{"v2 empty wrapped key", `{"version":2,"kdf":"argon2id","ciphertext":"YWJj","key_slots":{"passphrase":{"kdf":"argon2id","salt":"c2FsdA==","wrapped_key":""}}}`, false},
		{"v2 slot wrong kdf", `{"version":2,"kdf":"argon2id","ciphertext":"YWJj","key_slots":{"passphrase":{"kdf":"scrypt","salt":"c2FsdA==","wrapped_key":"d3JhcHBlZA=="}}}`, false},
		{"v1 missing salt", `{"version":1,"kdf":"argon2id","ciphertext":"YWJj"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validEnvelope(tt.payload); got != tt.want {
				t.Fatalf("validEnvelope(%q) = %v, want %v", tt.payload, got, tt.want)
			}
		})
	}
}

func TestEnvelopeValidationError(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{"raw base64", "1a2b3c4d5e6f7g8h9i0j", "not a JSON object"},
		{"not json", "hello world", "not a JSON object"},
		{"invalid json", `{"version":2`, "invalid JSON"},
		{"empty object", `{}`, "unsupported kdf"},
		{"missing ciphertext", `{"version":2,"kdf":"argon2id"}`, "missing ciphertext"},
		{"unsupported version", `{"version":9,"kdf":"argon2id","ciphertext":"YWJj"}`, "unsupported version"},
		{"missing key slot", `{"version":2,"kdf":"argon2id","ciphertext":"YWJj"}`, "passphrase key slot missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := envelopeValidationError(tt.payload); got != tt.want {
				t.Fatalf("envelopeValidationError(%q) = %q, want %q", tt.payload, got, tt.want)
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

func TestExportReturnsLatestEnvironmentVersions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT latest\.environment_name, latest\.version, latest\.checksum, latest\.encrypted_payload, latest\.created_at`).
		WithArgs("user-1", "app").
		WillReturnRows(sqlmock.NewRows([]string{"environment_name", "version", "checksum", "encrypted_payload", "created_at"}).
			AddRow("production", 3, "sum-production", "payload-production", time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)).
			AddRow("staging", 2, "sum-staging", "payload-staging", time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Next()
	})
	router.GET("/api/v1/env/export", NewEnvHandler(db).Export)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/env/export?project=app", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Export returned %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response struct {
		ProjectSlug  string `json:"project_slug"`
		Environments []struct {
			Environment string `json:"environment"`
			Version     int    `json:"version"`
		} `json:"environments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode export response: %v", err)
	}
	if response.ProjectSlug != "app" || len(response.Environments) != 2 {
		t.Fatalf("response = %+v, want project app with two environments", response)
	}
	if response.Environments[0].Environment != "production" || response.Environments[0].Version != 3 {
		t.Fatalf("first environment = %+v, want production v3", response.Environments[0])
	}
	if response.Environments[1].Environment != "staging" || response.Environments[1].Version != 2 {
		t.Fatalf("second environment = %+v, want staging v2", response.Environments[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}
