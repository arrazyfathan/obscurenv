package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/obscurenv/obscurenv/backend/middleware"
)

type TokenHandler struct {
	db *sql.DB
}

func NewTokenHandler(database *sql.DB) *TokenHandler {
	return &TokenHandler{db: database}
}

type tokenSummary struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	CreatedAt string  `json:"created_at"`
	ExpiresAt *string `json:"expires_at"`
}

type createTokenRequest struct {
	Name           string `json:"name" binding:"required"`
	ExpiresInDays  *int   `json:"expires_in_days"`
}

func (h *TokenHandler) List(c *gin.Context) {
	userID := middleware.UserID(c)
	if userID == "" {
		errorJSON(c, http.StatusUnauthorized, "missing authenticated user")
		return
	}

	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT id, name, created_at, expires_at
		FROM api_tokens
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to list tokens")
		return
	}
	defer rows.Close()

	tokens := make([]tokenSummary, 0)
	for rows.Next() {
		var token tokenSummary
		var createdAt time.Time
		var expiresAt sql.NullTime
		if err := rows.Scan(&token.ID, &token.Name, &createdAt, &expiresAt); err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to read tokens")
			return
		}
		token.CreatedAt = formatTime(createdAt)
		if expiresAt.Valid {
			value := formatTime(expiresAt.Time)
			token.ExpiresAt = &value
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to read tokens")
		return
	}
	c.JSON(http.StatusOK, gin.H{"tokens": tokens})
}

func (h *TokenHandler) Create(c *gin.Context) {
	userID := middleware.UserID(c)
	if userID == "" {
		errorJSON(c, http.StatusUnauthorized, "missing authenticated user")
		return
	}

	var req createTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid token request")
		return
	}
	if req.ExpiresInDays != nil && *req.ExpiresInDays < 1 {
		badRequest(c, "expires_in_days must be a positive integer")
		return
	}

	token, hash, err := newAPIToken()
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to generate token")
		return
	}

	var id string
	var expiresAt *time.Time
	if req.ExpiresInDays != nil {
		value := time.Now().UTC().Add(time.Duration(*req.ExpiresInDays) * 24 * time.Hour)
		expiresAt = &value
	}
	err = h.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO api_tokens (user_id, token_hash, name, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, userID, hash, req.Name, nullIfTime(expiresAt)).Scan(&id)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to store token")
		return
	}

	var expiresAtStr *string
	if expiresAt != nil {
		value := formatTime(*expiresAt)
		expiresAtStr = &value
	}
	c.JSON(http.StatusCreated, gin.H{"token": token, "id": id, "expires_at": expiresAtStr})
}

func (h *TokenHandler) Revoke(c *gin.Context) {
	userID := middleware.UserID(c)
	if userID == "" {
		errorJSON(c, http.StatusUnauthorized, "missing authenticated user")
		return
	}
	id := c.Param("id")

	result, err := h.db.ExecContext(c.Request.Context(), `
		DELETE FROM api_tokens
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to revoke token")
		return
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to confirm token revocation")
		return
	}
	if deleted == 0 {
		errorJSON(c, http.StatusNotFound, "token not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "token revoked"})
}

func nullIfTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}
