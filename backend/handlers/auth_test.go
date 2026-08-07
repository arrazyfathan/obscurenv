package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestNormalizeUsername(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "lowercases and trims", in: " Alice-Dev ", want: "alice-dev"},
		{name: "allows underscore", in: "alice_dev", want: "alice_dev"},
		{name: "rejects short", in: "ab", want: ""},
		{name: "rejects invalid character", in: "alice@example", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeUsername(tt.in); got != tt.want {
				t.Fatalf("normalizeUsername(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestLoginRotatesSameNameTokenAndIgnoresExpiry(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	passwordHash, err := hashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT id, password_hash FROM users WHERE email = \$1`).
		WithArgs("alice@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"id", "password_hash"}).AddRow("user-1", passwordHash))
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM api_tokens`).
		WithArgs("user-1", "local-cli").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO api_tokens`).
		WithArgs("user-1", sqlmock.AnyArg(), "local-cli").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAuthHandler(db)
	router.POST("/api/v1/auth/login", handler.Login)

	body := `{"email":"alice@example.com","password":"correct-password","token_name":"local-cli","expires_in_days":30}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Login returned %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "obe_tok_") {
		t.Fatalf("Login response missing token: %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLoginRejectsInvalidCredentialsWithoutTouchingTokens(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	passwordHash, err := hashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT id, password_hash FROM users WHERE email = \$1`).
		WithArgs("alice@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"id", "password_hash"}).AddRow("user-1", passwordHash))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAuthHandler(db)
	router.POST("/api/v1/auth/login", handler.Login)

	body := `{"email":"alice@example.com","password":"wrong-password","token_name":"local-cli"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("Login returned %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
