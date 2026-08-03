package handlers

import (
	"database/sql"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

type AuthHandler struct {
	db      *sql.DB
	passkey *webauthn.WebAuthn
	rpID    string
}

func NewAuthHandler(database *sql.DB) *AuthHandler {
	passkey, rpID, err := newWebAuthnProvider()
	if err != nil {
		passkey = nil
	}
	return &AuthHandler{db: database, passkey: passkey, rpID: rpID}
}

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Username string `json:"username"`
	Password string `json:"password" binding:"required,min=8"`
}

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_-]{2,31}$`)

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid registration request")
		return
	}
	username := normalizeUsername(req.Username)
	if req.Username != "" && username == "" {
		badRequest(c, "invalid username")
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to hash password")
		return
	}
	_, err = h.db.ExecContext(c.Request.Context(), `
		INSERT INTO users (email, username, password_hash) VALUES ($1, $2, $3)
	`, normalizeEmail(req.Email), nullIfEmpty(username), hash)
	if err != nil {
		errorJSON(c, http.StatusConflict, "user already exists")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "User registered successfully"})
}

type loginRequest struct {
	Email     string `json:"email"`
	Username  string `json:"username"`
	Password  string `json:"password" binding:"required"`
	TokenName string `json:"token_name" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid login request")
		return
	}
	email := normalizeEmail(req.Email)
	username := normalizeUsername(req.Username)
	if email == "" && username == "" {
		badRequest(c, "email or username is required")
		return
	}

	var userID, passwordHash string
	var err error
	if username != "" {
		err = h.db.QueryRowContext(c.Request.Context(), `
			SELECT id, password_hash FROM users WHERE username = $1
		`, username).Scan(&userID, &passwordHash)
	} else {
		err = h.db.QueryRowContext(c.Request.Context(), `
			SELECT id, password_hash FROM users WHERE email = $1
		`, email).Scan(&userID, &passwordHash)
	}
	if err != nil || !verifyPassword(req.Password, passwordHash) {
		errorJSON(c, http.StatusUnauthorized, "invalid credentials")
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

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func normalizeUsername(username string) string {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return ""
	}
	if !usernamePattern.MatchString(username) {
		return ""
	}
	return username
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func newWebAuthnProvider() (*webauthn.WebAuthn, string, error) {
	rpID := strings.TrimSpace(os.Getenv("OBE_WEBAUTHN_RP_ID"))
	if rpID == "" {
		rpID = "localhost"
	}
	origins := splitCSV(os.Getenv("OBE_WEBAUTHN_ORIGINS"))
	if len(origins) == 0 {
		origins = []string{"http://localhost:3000", "http://localhost:3001"}
	}
	displayName := strings.TrimSpace(os.Getenv("OBE_WEBAUTHN_RP_NAME"))
	if displayName == "" {
		displayName = "Obscurenv"
	}
	provider, err := webauthn.New(&webauthn.Config{
		RPID:                  rpID,
		RPDisplayName:         displayName,
		RPOrigins:             origins,
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationPreferred,
		},
	})
	return provider, rpID, err
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
