package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/obscurenv/obscurenv/backend/middleware"
)

const (
	passkeyRegisterSession = "passkey_register"
	passkeyLoginSession    = "passkey_login"
	defaultPasskeyName     = "Passkey"
	passkeySessionTTL      = 5 * time.Minute
)

type passkeyUser struct {
	id          string
	email       string
	username    sql.NullString
	handle      []byte
	credentials []webauthn.Credential
}

func (u passkeyUser) WebAuthnID() []byte {
	return u.handle
}

func (u passkeyUser) WebAuthnName() string {
	if u.username.Valid && u.username.String != "" {
		return u.username.String
	}
	return u.email
}

func (u passkeyUser) WebAuthnDisplayName() string {
	return u.WebAuthnName()
}

func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

type passkeyOptionsRequest struct {
	Name string `json:"name"`
}

type passkeyOptionsResponse struct {
	CeremonyID string `json:"ceremony_id"`
	Options    any    `json:"options"`
}

type passkeySummary struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at"`
}

func (h *AuthHandler) PasskeyRegisterOptions(c *gin.Context) {
	if h.passkey == nil {
		errorJSON(c, http.StatusServiceUnavailable, "passkey authentication is not configured")
		return
	}
	var req passkeyOptionsRequest
	_ = c.ShouldBindJSON(&req)

	userID := middleware.UserID(c)
	user, err := h.loadPasskeyUser(c.Request.Context(), userID)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "user not found")
		return
	}
	options, session, err := h.passkey.BeginRegistration(user)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to create passkey challenge")
		return
	}
	ceremonyID, err := h.storePasskeySession(c.Request.Context(), passkeyRegisterSession, userID, passkeyName(req.Name), *session)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to store passkey challenge")
		return
	}
	c.JSON(http.StatusOK, passkeyOptionsResponse{CeremonyID: ceremonyID, Options: options})
}

func (h *AuthHandler) PasskeyRegisterFinish(c *gin.Context) {
	if h.passkey == nil {
		errorJSON(c, http.StatusServiceUnavailable, "passkey authentication is not configured")
		return
	}
	ceremonyID := c.Query("ceremony_id")
	if ceremonyID == "" {
		badRequest(c, "ceremony_id is required")
		return
	}
	userID := middleware.UserID(c)
	session, label, err := h.consumePasskeySession(c.Request.Context(), ceremonyID, passkeyRegisterSession, userID)
	if err != nil {
		errorJSON(c, http.StatusUnauthorized, "invalid passkey challenge")
		return
	}
	user, err := h.loadPasskeyUser(c.Request.Context(), userID)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "user not found")
		return
	}
	credential, err := h.passkey.FinishRegistration(user, session, c.Request)
	if err != nil {
		errorJSON(c, http.StatusUnauthorized, "passkey registration failed")
		return
	}
	if err := h.storePasskeyCredential(c.Request.Context(), userID, passkeyName(label), *credential); err != nil {
		errorJSON(c, http.StatusConflict, "passkey already registered")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AuthHandler) PasskeyLoginOptions(c *gin.Context) {
	if h.passkey == nil {
		errorJSON(c, http.StatusServiceUnavailable, "passkey authentication is not configured")
		return
	}
	options, session, err := h.passkey.BeginDiscoverableLogin()
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to create passkey challenge")
		return
	}
	ceremonyID, err := h.storePasskeySession(c.Request.Context(), passkeyLoginSession, "", "", *session)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to store passkey challenge")
		return
	}
	c.JSON(http.StatusOK, passkeyOptionsResponse{CeremonyID: ceremonyID, Options: options})
}

func (h *AuthHandler) PasskeyLoginFinish(c *gin.Context) {
	if h.passkey == nil {
		errorJSON(c, http.StatusServiceUnavailable, "passkey authentication is not configured")
		return
	}
	ceremonyID := c.Query("ceremony_id")
	if ceremonyID == "" {
		badRequest(c, "ceremony_id is required")
		return
	}
	session, _, err := h.consumePasskeySession(c.Request.Context(), ceremonyID, passkeyLoginSession, "")
	if err != nil {
		errorJSON(c, http.StatusUnauthorized, "invalid passkey challenge")
		return
	}
	user, credential, err := h.passkey.FinishPasskeyLogin(h.discoverPasskeyUser(c.Request.Context()), session, c.Request)
	if err != nil {
		errorJSON(c, http.StatusUnauthorized, "passkey login failed")
		return
	}
	passkeyUser, ok := user.(passkeyUser)
	if !ok {
		errorJSON(c, http.StatusInternalServerError, "failed to resolve passkey user")
		return
	}
	if err := h.updatePasskeyCredential(c.Request.Context(), passkeyUser.id, *credential); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to update passkey")
		return
	}
	token, hash, err := newAPIToken()
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to generate token")
		return
	}
	if err := rotateToken(c.Request.Context(), h.db, passkeyUser.id, "passkey-web", hash); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to store token")
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}

func (h *AuthHandler) ListPasskeys(c *gin.Context) {
	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT id, name, created_at, last_used_at
		FROM passkeys
		WHERE user_id = $1 AND rp_id = $2
		ORDER BY created_at DESC
	`, middleware.UserID(c), h.rpID)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to list passkeys")
		return
	}
	defer rows.Close()

	passkeys := make([]passkeySummary, 0)
	for rows.Next() {
		var item passkeySummary
		var createdAt time.Time
		var lastUsedAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.Name, &createdAt, &lastUsedAt); err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to read passkeys")
			return
		}
		item.CreatedAt = createdAt.Format(time.RFC3339)
		if lastUsedAt.Valid {
			value := lastUsedAt.Time.Format(time.RFC3339)
			item.LastUsedAt = &value
		}
		passkeys = append(passkeys, item)
	}
	if err := rows.Err(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to read passkeys")
		return
	}
	c.JSON(http.StatusOK, gin.H{"passkeys": passkeys})
}

func (h *AuthHandler) RevokePasskey(c *gin.Context) {
	result, err := h.db.ExecContext(c.Request.Context(), `
		DELETE FROM passkeys
		WHERE id = $1 AND user_id = $2 AND rp_id = $3
	`, c.Param("id"), middleware.UserID(c), h.rpID)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to revoke passkey")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		errorJSON(c, http.StatusNotFound, "passkey not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AuthHandler) loadPasskeyUser(ctx context.Context, userID string) (passkeyUser, error) {
	var user passkeyUser
	err := h.db.QueryRowContext(ctx, `
		SELECT id, email, username
		FROM users
		WHERE id = $1
	`, userID).Scan(&user.id, &user.email, &user.username)
	if err != nil {
		return user, err
	}
	handle, err := h.ensurePasskeyHandle(ctx, userID)
	if err != nil {
		return user, err
	}
	user.handle = handle
	user.credentials, err = h.loadUserPasskeyCredentials(ctx, userID)
	return user, err
}

func (h *AuthHandler) ensurePasskeyHandle(ctx context.Context, userID string) ([]byte, error) {
	handle, err := randomBytes(32)
	if err != nil {
		return nil, err
	}
	var out []byte
	err = h.db.QueryRowContext(ctx, `
		WITH existing AS (
			SELECT user_handle FROM webauthn_users WHERE user_id = $1 AND rp_id = $2
		), inserted AS (
			INSERT INTO webauthn_users (user_id, rp_id, user_handle)
			SELECT $1, $2, $3
			WHERE NOT EXISTS (SELECT 1 FROM existing)
			RETURNING user_handle
		)
		SELECT user_handle FROM inserted
		UNION ALL
		SELECT user_handle FROM existing
		LIMIT 1
	`, userID, h.rpID, handle).Scan(&out)
	return out, err
}

func (h *AuthHandler) loadUserPasskeyCredentials(ctx context.Context, userID string) ([]webauthn.Credential, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT credential
		FROM passkeys
		WHERE user_id = $1 AND rp_id = $2
	`, userID, h.rpID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	credentials := make([]webauthn.Credential, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var credential webauthn.Credential
		if err := json.Unmarshal(raw, &credential); err != nil {
			return nil, err
		}
		credentials = append(credentials, credential)
	}
	return credentials, rows.Err()
}

func (h *AuthHandler) discoverPasskeyUser(ctx context.Context) webauthn.DiscoverableUserHandler {
	return func(rawID, userHandle []byte) (webauthn.User, error) {
		var userID string
		err := h.db.QueryRowContext(ctx, `
			SELECT wu.user_id
			FROM webauthn_users wu
			JOIN passkeys p ON p.user_id = wu.user_id AND p.rp_id = wu.rp_id
			WHERE wu.rp_id = $1 AND wu.user_handle = $2 AND p.credential_id = $3
		`, h.rpID, userHandle, rawID).Scan(&userID)
		if err != nil {
			return nil, err
		}
		user, err := h.loadPasskeyUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		return user, nil
	}
}

func (h *AuthHandler) storePasskeySession(ctx context.Context, kind, userID, label string, session webauthn.SessionData) (string, error) {
	expiresAt := passkeySessionExpires(session)
	session.Expires = expiresAt
	data, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	var ceremonyID string
	err = h.db.QueryRowContext(ctx, `
		INSERT INTO webauthn_sessions (user_id, kind, label, session_json, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, nullIfEmpty(userID), kind, nullIfEmpty(label), string(data), expiresAt).Scan(&ceremonyID)
	return ceremonyID, err
}

func passkeySessionExpires(session webauthn.SessionData) time.Time {
	if !session.Expires.IsZero() {
		return session.Expires
	}
	return time.Now().UTC().Add(passkeySessionTTL)
}

func (h *AuthHandler) consumePasskeySession(ctx context.Context, ceremonyID, kind, userID string) (webauthn.SessionData, string, error) {
	var session webauthn.SessionData
	var raw []byte
	var label sql.NullString
	query := `
		DELETE FROM webauthn_sessions
		WHERE id = $1 AND kind = $2 AND expires_at > $3
	`
	args := []any{ceremonyID, kind, time.Now().UTC()}
	if userID != "" {
		query += " AND user_id = $4"
		args = append(args, userID)
	} else {
		query += " AND user_id IS NULL"
	}
	query += " RETURNING session_json, label"
	if err := h.db.QueryRowContext(ctx, query, args...).Scan(&raw, &label); err != nil {
		return session, "", err
	}
	if err := json.Unmarshal(raw, &session); err != nil {
		return session, "", err
	}
	return session, label.String, nil
}

func (h *AuthHandler) storePasskeyCredential(ctx context.Context, userID, name string, credential webauthn.Credential) error {
	data, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	_, err = h.db.ExecContext(ctx, `
		INSERT INTO passkeys (user_id, rp_id, name, credential_id, credential)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, h.rpID, name, credential.ID, string(data))
	return err
}

func (h *AuthHandler) updatePasskeyCredential(ctx context.Context, userID string, credential webauthn.Credential) error {
	data, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	result, err := h.db.ExecContext(ctx, `
		UPDATE passkeys
		SET credential = $1, last_used_at = $2
		WHERE user_id = $3 AND rp_id = $4 AND credential_id = $5
	`, string(data), time.Now().UTC(), userID, h.rpID, credential.ID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func passkeyName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultPasskeyName
	}
	if len(value) > 100 {
		return value[:100]
	}
	return value
}
