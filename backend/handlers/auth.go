package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	db *sql.DB
}

func NewAuthHandler(database *sql.DB) *AuthHandler {
	return &AuthHandler{db: database}
}

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid registration request")
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to hash password")
		return
	}
	_, err = h.db.ExecContext(c.Request.Context(), `
		INSERT INTO users (email, password_hash) VALUES ($1, $2)
	`, strings.ToLower(req.Email), hash)
	if err != nil {
		errorJSON(c, http.StatusConflict, "user already exists")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "User registered successfully"})
}

type loginRequest struct {
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required"`
	TokenName string `json:"token_name" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid login request")
		return
	}

	var userID, passwordHash string
	err := h.db.QueryRowContext(c.Request.Context(), `
		SELECT id, password_hash FROM users WHERE email = $1
	`, strings.ToLower(req.Email)).Scan(&userID, &passwordHash)
	if err != nil || !verifyPassword(req.Password, passwordHash) {
		errorJSON(c, http.StatusUnauthorized, "invalid email or password")
		return
	}

	token, hash, err := newAPIToken()
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to generate token")
		return
	}
	_, err = h.db.ExecContext(c.Request.Context(), `
		INSERT INTO api_tokens (user_id, token_hash, name) VALUES ($1, $2, $3)
	`, userID, hash, req.TokenName)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to store token")
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}
