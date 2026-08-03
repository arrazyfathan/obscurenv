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

type UserHandler struct {
	db             *sql.DB
	getProfile     func(context.Context, string) (userProfile, error)
	updateUsername func(context.Context, string, string) (userProfile, error)
}

func NewUserHandler(database *sql.DB) *UserHandler {
	h := &UserHandler{db: database}
	h.getProfile = h.getProfileFromDB
	h.updateUsername = h.updateUsernameInDB
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
