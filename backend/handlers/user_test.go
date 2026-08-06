package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

func TestUserProfileReturnsAuthenticatedUser(t *testing.T) {
	router, handler := newUserTestRouter()
	username := "alice"
	createdAt := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	handler.getProfile = func(_ context.Context, userID string) (userProfile, error) {
		if userID != "user-1" {
			t.Fatalf("userID = %q, want user-1", userID)
		}
		return userProfile{
			ID:        "user-1",
			Email:     "alice@example.com",
			Username:  &username,
			CreatedAt: createdAt,
		}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Profile returned %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"id":"user-1"`, `"email":"alice@example.com"`, `"username":"alice"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("Profile body = %q, want %s", body, want)
		}
	}
	if strings.Contains(body, "password") {
		t.Fatalf("Profile body = %q, must not include password fields", body)
	}
}

func TestUserProfileRequiresAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewUserHandler(nil)
	router.GET("/api/v1/user/profile", handler.Profile)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("Profile returned %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestUpdateProfileChangesUsername(t *testing.T) {
	router, handler := newUserTestRouter()
	createdAt := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	handler.updateUsername = func(_ context.Context, userID, username string) (userProfile, error) {
		if userID != "user-1" {
			t.Fatalf("userID = %q, want user-1", userID)
		}
		if username != "alice-dev" {
			t.Fatalf("username = %q, want alice-dev", username)
		}
		return userProfile{
			ID:        "user-1",
			Email:     "alice@example.com",
			Username:  &username,
			CreatedAt: createdAt,
		}, nil
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/user/profile", strings.NewReader(`{"username":" Alice-Dev "}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("UpdateProfile returned %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"username":"alice-dev"`) {
		t.Fatalf("UpdateProfile body = %q, want normalized username", rec.Body.String())
	}
}

func TestUpdateProfileRejectsInvalidUsername(t *testing.T) {
	router, handler := newUserTestRouter()
	handler.updateUsername = func(context.Context, string, string) (userProfile, error) {
		t.Fatal("updateUsername must not be called for invalid username")
		return userProfile{}, nil
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/user/profile", strings.NewReader(`{"username":"ab"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("UpdateProfile returned %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateProfileRejectsDuplicateUsername(t *testing.T) {
	router, handler := newUserTestRouter()
	handler.updateUsername = func(context.Context, string, string) (userProfile, error) {
		return userProfile{}, &pq.Error{Code: "23505"}
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/user/profile", strings.NewReader(`{"username":"alice"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("UpdateProfile returned %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestUpdateProfileReturnsNotFoundWhenUserMissing(t *testing.T) {
	router, handler := newUserTestRouter()
	handler.updateUsername = func(context.Context, string, string) (userProfile, error) {
		return userProfile{}, sql.ErrNoRows
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/user/profile", strings.NewReader(`{"username":"alice"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("UpdateProfile returned %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestChangePasswordRejectsShortNewPassword(t *testing.T) {
	router := newPasswordTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password", strings.NewReader(`{"current_password":"correct","new_password":"short"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("ChangePassword returned %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestChangePasswordRejectsWrongCurrentPassword(t *testing.T) {
	router, handler := newPasswordTestRouterWithHandler()
	handler.changePassword = func(_ context.Context, userID, current, newPassword string) error {
		if userID != "user-1" {
			t.Fatalf("userID = %q, want user-1", userID)
		}
		return errWrongPassword
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password", strings.NewReader(`{"current_password":"wrong","new_password":"correct-password"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("ChangePassword returned %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "current password is incorrect") {
		t.Fatalf("ChangePassword body = %q, want wrong-password error", rec.Body.String())
	}
}

func TestChangePasswordUpdatesPassword(t *testing.T) {
	router, handler := newPasswordTestRouterWithHandler()
	handler.changePassword = func(_ context.Context, userID, current, newPassword string) error {
		if userID != "user-1" || current != "current-pass" || newPassword != "new-pass-123" {
			t.Fatalf("changePassword(%q, %q, %q)", userID, current, newPassword)
		}
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password", strings.NewReader(`{"current_password":"current-pass","new_password":"new-pass-123"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ChangePassword returned %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestDeleteAccountRequiresConfirmation(t *testing.T) {
	router := newDeleteAccountTestRouter()

	for name, body := range map[string]string{
		"missing":  `{}`,
		"rejected": `{"confirm":false}`,
	} {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/user", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("DeleteAccount (%s) returned %d, want %d", name, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestDeleteAccountRemovesUser(t *testing.T) {
	router, handler := newDeleteAccountTestRouterWithHandler()
	handler.deleteAccount = func(_ context.Context, userID string) error {
		if userID != "user-1" {
			t.Fatalf("userID = %q, want user-1", userID)
		}
		return nil
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/user", strings.NewReader(`{"confirm":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("DeleteAccount returned %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestDeleteAccountReturnsNotFoundWhenUserMissing(t *testing.T) {
	router, handler := newDeleteAccountTestRouterWithHandler()
	handler.deleteAccount = func(context.Context, string) error {
		return sql.ErrNoRows
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/user", strings.NewReader(`{"confirm":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("DeleteAccount returned %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestIsUniqueConstraint(t *testing.T) {
	if !isUniqueConstraint(&pq.Error{Code: "23505"}) {
		t.Fatal("isUniqueConstraint returned false for postgres unique violation")
	}
	if !isUniqueConstraint(errors.New("duplicate key value violates unique constraint")) {
		t.Fatal("isUniqueConstraint returned false for duplicate key error")
	}
	if isUniqueConstraint(sql.ErrNoRows) {
		t.Fatal("isUniqueConstraint returned true for sql.ErrNoRows")
	}
}

func newUserTestRouter() (*gin.Engine, *UserHandler) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewUserHandler(nil)
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Next()
	})
	router.GET("/api/v1/user/profile", handler.Profile)
	router.PATCH("/api/v1/user/profile", handler.UpdateProfile)
	return router, handler
}

func newPasswordTestRouter() *gin.Engine {
	router, _ := newPasswordTestRouterWithHandler()
	return router
}

func newPasswordTestRouterWithHandler() (*gin.Engine, *UserHandler) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewUserHandler(nil)
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Next()
	})
	router.POST("/api/v1/user/password", handler.ChangePassword)
	return router, handler
}

func newDeleteAccountTestRouter() *gin.Engine {
	router, _ := newDeleteAccountTestRouterWithHandler()
	return router
}

func newDeleteAccountTestRouterWithHandler() (*gin.Engine, *UserHandler) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewUserHandler(nil)
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Next()
	})
	router.DELETE("/api/v1/user", handler.DeleteAccount)
	return router, handler
}
