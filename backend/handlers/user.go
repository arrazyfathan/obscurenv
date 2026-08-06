package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/obscurenv/obscurenv/backend/middleware"
)

var errWrongPassword = errors.New("current password is incorrect")

type UserHandler struct {
	db             *sql.DB
	getProfile     func(context.Context, string) (userProfile, error)
	updateUsername func(context.Context, string, string) (userProfile, error)
	changePassword func(context.Context, string, string, string) error
	deleteAccount  func(context.Context, string) error
}

func NewUserHandler(database *sql.DB) *UserHandler {
	h := &UserHandler{db: database}
	h.getProfile = h.getProfileFromDB
	h.updateUsername = h.updateUsernameInDB
	h.changePassword = h.changePasswordInDB
	h.deleteAccount = h.deleteAccountInDB
	return h
}

type userProfile struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Username  *string   `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

type updateUserProfileRequest struct {
	Username string `json:"username" binding:"required"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=8"`
}

type deleteAccountRequest struct {
	Confirm *bool `json:"confirm" binding:"required"`
}

func (h *UserHandler) Profile(c *gin.Context) {
	userID := middleware.UserID(c)
	if userID == "" {
		errorJSON(c, http.StatusUnauthorized, "missing authenticated user")
		return
	}

	profile, err := h.getProfile(c.Request.Context(), userID)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "user not found")
		return
	}
	c.JSON(http.StatusOK, profile)
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID := middleware.UserID(c)
	if userID == "" {
		errorJSON(c, http.StatusUnauthorized, "missing authenticated user")
		return
	}

	var req updateUserProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid profile request")
		return
	}
	username := normalizeUsername(req.Username)
	if username == "" {
		badRequest(c, "invalid username")
		return
	}

	profile, err := h.updateUsername(c.Request.Context(), userID, username)
	if err != nil {
		if isUniqueConstraint(err) {
			errorJSON(c, http.StatusConflict, "username already exists")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			errorJSON(c, http.StatusNotFound, "user not found")
			return
		}
		errorJSON(c, http.StatusInternalServerError, "failed to update profile")
		return
	}
	c.JSON(http.StatusOK, profile)
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	userID := middleware.UserID(c)
	if userID == "" {
		errorJSON(c, http.StatusUnauthorized, "missing authenticated user")
		return
	}

	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid password request")
		return
	}
	err := h.changePassword(c.Request.Context(), userID, req.CurrentPassword, req.NewPassword)
	if err != nil {
		if errors.Is(err, errWrongPassword) {
			badRequest(c, "current password is incorrect")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			errorJSON(c, http.StatusNotFound, "user not found")
			return
		}
		errorJSON(c, http.StatusInternalServerError, "failed to change password")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "password updated"})
}

func (h *UserHandler) DeleteAccount(c *gin.Context) {
	userID := middleware.UserID(c)
	if userID == "" {
		errorJSON(c, http.StatusUnauthorized, "missing authenticated user")
		return
	}

	var req deleteAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid delete request")
		return
	}
	if !*req.Confirm {
		badRequest(c, "confirmation is required")
		return
	}
	if err := h.deleteAccount(c.Request.Context(), userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			errorJSON(c, http.StatusNotFound, "user not found")
			return
		}
		errorJSON(c, http.StatusInternalServerError, "failed to delete account")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "account deleted"})
}

func (h *UserHandler) getProfileFromDB(ctx context.Context, userID string) (userProfile, error) {
	var profile userProfile
	var username sql.NullString
	err := h.db.QueryRowContext(ctx, `
		SELECT id, email, username, created_at
		FROM users
		WHERE id = $1
	`, userID).Scan(&profile.ID, &profile.Email, &username, &profile.CreatedAt)
	if err != nil {
		return userProfile{}, err
	}
	if username.Valid {
		profile.Username = &username.String
	}
	return profile, nil
}

func (h *UserHandler) updateUsernameInDB(ctx context.Context, userID, username string) (userProfile, error) {
	var profile userProfile
	var storedUsername string
	err := h.db.QueryRowContext(ctx, `
		UPDATE users
		SET username = $2
		WHERE id = $1
		RETURNING id, email, username, created_at
	`, userID, username).Scan(&profile.ID, &profile.Email, &storedUsername, &profile.CreatedAt)
	if err != nil {
		return userProfile{}, err
	}
	profile.Username = &storedUsername
	return profile, nil
}

func (h *UserHandler) changePasswordInDB(ctx context.Context, userID, current, newPassword string) error {
	var storedHash string
	err := h.db.QueryRowContext(ctx, `
		SELECT password_hash FROM users WHERE id = $1
	`, userID).Scan(&storedHash)
	if err != nil {
		return err
	}
	if !verifyPassword(current, storedHash) {
		return errWrongPassword
	}
	hash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}
	_, err = h.db.ExecContext(ctx, `
		UPDATE users SET password_hash = $2 WHERE id = $1
	`, userID, hash)
	return err
}

func (h *UserHandler) deleteAccountInDB(ctx context.Context, userID string) error {
	result, err := h.db.ExecContext(ctx, `
		DELETE FROM users WHERE id = $1
	`, userID)
	if err != nil {
		return err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if deleted == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505"
	}
	message := err.Error()
	return strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "unique constraint")
}
