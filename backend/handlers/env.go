package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/obscurenv/obscurenv/backend/middleware"
	"github.com/obscurenv/obscurenv/backend/models"
)

const (
	supportedEnvelopeVersion = 2
	kdfName                  = "argon2id"
)

type envelope struct {
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	Salt       string `json:"salt"`
	Ciphertext string `json:"ciphertext"`
	KeySlots   struct {
		Passphrase struct {
			KDF        string `json:"kdf"`
			Salt       string `json:"salt"`
			WrappedKey string `json:"wrapped_key"`
		} `json:"passphrase"`
	} `json:"key_slots"`
}

type EnvHandler struct {
	db *sql.DB
}

func NewEnvHandler(database *sql.DB) *EnvHandler {
	return &EnvHandler{db: database}
}

type pushRequest struct {
	ProjectSlug      string `json:"project_slug" binding:"required"`
	Environment      string `json:"environment" binding:"required"`
	EncryptedPayload string `json:"encrypted_payload" binding:"required"`
	Checksum         string `json:"checksum" binding:"required,len=64"`
}

func (h *EnvHandler) Push(c *gin.Context) {
	var req pushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid push request")
		return
	}
	sum := sha256.Sum256([]byte(req.EncryptedPayload))
	if req.Checksum != hex.EncodeToString(sum[:]) {
		badRequest(c, "checksum does not match encrypted payload")
		return
	}
	if !validEnvelope(req.EncryptedPayload) {
		badRequest(c, "encrypted payload is not a valid envelope")
		return
	}
	userID := middleware.UserID(c)

	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback()

	var projectID string
	err = tx.QueryRowContext(c.Request.Context(), `
		SELECT id FROM projects WHERE user_id = $1 AND slug = $2
	`, userID, req.ProjectSlug).Scan(&projectID)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "project not found")
		return
	}

	var version int
	err = tx.QueryRowContext(c.Request.Context(), `
		SELECT COALESCE(MAX(version), 0) + 1
		FROM env_versions
		WHERE project_id = $1 AND environment_name = $2
	`, projectID, req.Environment).Scan(&version)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to calculate version")
		return
	}

	_, err = tx.ExecContext(c.Request.Context(), `
		INSERT INTO env_versions (project_id, environment_name, version, encrypted_payload, checksum)
		VALUES ($1, $2, $3, $4, $5)
	`, projectID, req.Environment, version, req.EncryptedPayload, req.Checksum)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to store environment")
		return
	}
	if err := recordActivity(c.Request.Context(), tx, userID, projectID, ActionEnvPushed, req.ProjectSlug, req.Environment, gin.H{"version": version}); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to record activity")
		return
	}
	if err := tx.Commit(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to commit environment")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Pushed successfully", "version": version})
}

func validEnvelope(payload string) bool {
	return envelopeValidationError(payload) == ""
}

func envelopeValidationError(payload string) string {
	if !strings.HasPrefix(strings.TrimSpace(payload), "{") {
		return "not a JSON object"
	}
	var env envelope
	if err := json.Unmarshal([]byte(payload), &env); err != nil {
		return "invalid JSON"
	}
	if env.KDF != kdfName {
		return "unsupported kdf"
	}
	if env.Ciphertext == "" {
		return "missing ciphertext"
	}
	switch env.Version {
	case 1:
		if env.Salt == "" {
			return "missing salt"
		}
		return ""
	case supportedEnvelopeVersion:
		slot := env.KeySlots.Passphrase
		switch {
		case slot.KDF != kdfName:
			return "passphrase key slot missing"
		case slot.Salt == "":
			return "passphrase key slot missing salt"
		case slot.WrappedKey == "":
			return "passphrase key slot missing wrapped key"
		}
		return ""
	default:
		return "unsupported version"
	}
}

func (h *EnvHandler) Pull(c *gin.Context) {
	projectSlug := c.Query("project")
	environment := c.Query("environment")
	if projectSlug == "" || environment == "" {
		badRequest(c, "project and environment are required")
		return
	}
	versionParam := c.Query("version")

	var out models.EnvVersion
	query := `
		SELECT p.slug, ev.environment_name, ev.version, ev.encrypted_payload, ev.checksum
		FROM env_versions ev
		JOIN projects p ON p.id = ev.project_id
		WHERE p.user_id = $1 AND p.slug = $2 AND ev.environment_name = $3
	`
	args := []any{middleware.UserID(c), projectSlug, environment}
	if versionParam != "" {
		version, err := strconv.Atoi(versionParam)
		if err != nil || version < 1 {
			badRequest(c, "version must be a positive integer")
			return
		}
		query += " AND ev.version = $4"
		args = append(args, version)
	}
	query += " ORDER BY ev.version DESC LIMIT 1"

	err := h.db.QueryRowContext(c.Request.Context(), query, args...).Scan(
		&out.ProjectSlug,
		&out.Environment,
		&out.Version,
		&out.EncryptedPayload,
		&out.Checksum,
	)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "environment not found")
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *EnvHandler) List(c *gin.Context) {
	projectSlug := c.Query("project")
	if projectSlug == "" {
		badRequest(c, "project is required")
		return
	}
	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT DISTINCT ev.environment_name
		FROM env_versions ev
		JOIN projects p ON p.id = ev.project_id
		WHERE p.user_id = $1 AND p.slug = $2
		ORDER BY ev.environment_name
	`, middleware.UserID(c), projectSlug)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to list environments")
		return
	}
	defer rows.Close()

	environments := make([]string, 0)
	for rows.Next() {
		var environment string
		if err := rows.Scan(&environment); err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to read environments")
			return
		}
		environments = append(environments, environment)
	}
	if err := rows.Err(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to read environments")
		return
	}
	c.JSON(http.StatusOK, gin.H{"environments": environments})
}

type envVersionMeta struct {
	ProjectSlug string `json:"project_slug"`
	Environment string `json:"environment"`
	Version     int    `json:"version"`
	Checksum    string `json:"checksum"`
	CreatedAt   string `json:"created_at"`
}

func (h *EnvHandler) Versions(c *gin.Context) {
	projectSlug := c.Query("project")
	environment := c.Query("environment")
	if projectSlug == "" || environment == "" {
		badRequest(c, "project and environment are required")
		return
	}

	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT p.slug, ev.environment_name, ev.version, ev.checksum, ev.created_at
		FROM env_versions ev
		JOIN projects p ON p.id = ev.project_id
		WHERE p.user_id = $1 AND p.slug = $2 AND ev.environment_name = $3
		ORDER BY ev.version DESC
	`, middleware.UserID(c), projectSlug, environment)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to list versions")
		return
	}
	defer rows.Close()

	versions := make([]envVersionMeta, 0)
	for rows.Next() {
		var item envVersionMeta
		var createdAt time.Time
		if err := rows.Scan(&item.ProjectSlug, &item.Environment, &item.Version, &item.Checksum, &createdAt); err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to read versions")
			return
		}
		item.CreatedAt = createdAt.Format(time.RFC3339)
		versions = append(versions, item)
	}
	if err := rows.Err(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to read versions")
		return
	}
	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

type envValidationItem struct {
	ProjectSlug string `json:"project_slug"`
	Environment string `json:"environment"`
	Version     int    `json:"version"`
	Checksum    string `json:"checksum"`
	CreatedAt   string `json:"created_at"`
	Valid       bool   `json:"valid"`
	Reason      string `json:"reason,omitempty"`
}

// ValidateList is a diagnostic helper that reports which stored versions hold
// a payload that does not match the envelope format the web and CLI expect.
// It never returns the encrypted payloads themselves.
func (h *EnvHandler) ValidateList(c *gin.Context) {
	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT p.slug, ev.environment_name, ev.version, ev.checksum, ev.encrypted_payload, ev.created_at
		FROM env_versions ev
		JOIN projects p ON p.id = ev.project_id
		WHERE p.user_id = $1
		ORDER BY p.slug, ev.environment_name, ev.version
	`, middleware.UserID(c))
	if err != nil {
		log.Printf("validate environments: %v", err)
		errorJSON(c, http.StatusInternalServerError, "failed to validate environments")
		return
	}
	defer rows.Close()

	versions := make([]envValidationItem, 0)
	for rows.Next() {
		var item envValidationItem
		var payload string
		var createdAt time.Time
		if err := rows.Scan(&item.ProjectSlug, &item.Environment, &item.Version, &item.Checksum, &payload, &createdAt); err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to read versions")
			return
		}
		item.CreatedAt = createdAt.Format(time.RFC3339)
		item.Valid = validEnvelope(payload)
		if !item.Valid {
			item.Reason = envelopeValidationError(payload)
		}
		versions = append(versions, item)
	}
	if err := rows.Err(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to read versions")
		return
	}
	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

type envExportItem struct {
	Environment      string `json:"environment"`
	Version          int    `json:"version"`
	Checksum         string `json:"checksum"`
	EncryptedPayload string `json:"encrypted_payload"`
	CreatedAt        string `json:"created_at"`
}

func (h *EnvHandler) Export(c *gin.Context) {
	projectSlug := c.Query("project")
	if projectSlug == "" {
		badRequest(c, "project is required")
		return
	}

	var projectID string
	if err := h.db.QueryRowContext(c.Request.Context(), `
		SELECT id FROM projects WHERE user_id = $1 AND slug = $2
	`, middleware.UserID(c), projectSlug).Scan(&projectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			errorJSON(c, http.StatusNotFound, "project not found")
			return
		}
		errorJSON(c, http.StatusInternalServerError, "failed to find project")
		return
	}

	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT latest.environment_name, latest.version, latest.checksum, latest.encrypted_payload, latest.created_at
		FROM (
			SELECT
				ev.environment_name,
				ev.version,
				ev.checksum,
				ev.encrypted_payload,
				ev.created_at,
				ROW_NUMBER() OVER (
					PARTITION BY ev.environment_name
					ORDER BY ev.version DESC
				) AS row_number
			FROM env_versions ev
			JOIN projects p ON p.id = ev.project_id
			WHERE p.user_id = $1 AND p.slug = $2
		) latest
		WHERE row_number = 1
		ORDER BY environment_name
	`, middleware.UserID(c), projectSlug)
	if err != nil {
		log.Printf("export environments: %v", err)
		errorJSON(c, http.StatusInternalServerError, "failed to export environments")
		return
	}
	defer rows.Close()

	environments := make([]envExportItem, 0)
	for rows.Next() {
		var item envExportItem
		var createdAt time.Time
		if err := rows.Scan(&item.Environment, &item.Version, &item.Checksum, &item.EncryptedPayload, &createdAt); err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to read environments")
			return
		}
		item.CreatedAt = formatTime(createdAt)
		environments = append(environments, item)
	}
	if err := rows.Err(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to read environments")
		return
	}
	if err := recordActivity(c.Request.Context(), h.db, middleware.UserID(c), projectID, ActionProjectExported, projectSlug, "", gin.H{
		"environment_count": len(environments),
	}); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to record activity")
		return
	}
	c.JSON(http.StatusOK, gin.H{"project_slug": projectSlug, "environments": environments})
}

func (h *EnvHandler) Delete(c *gin.Context) {
	projectSlug := c.Query("project")
	environment := c.Query("environment")
	if projectSlug == "" || environment == "" {
		badRequest(c, "project and environment are required")
		return
	}

	userID := middleware.UserID(c)

	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback()

	var projectID string
	err = tx.QueryRowContext(c.Request.Context(), `
		SELECT id FROM projects WHERE user_id = $1 AND slug = $2
	`, userID, projectSlug).Scan(&projectID)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "project not found")
		return
	}

	result, err := tx.ExecContext(c.Request.Context(), `
		DELETE FROM env_versions
		WHERE project_id = $1 AND environment_name = $2
	`, projectID, environment)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to delete environment")
		return
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to confirm environment deletion")
		return
	}
	if deleted == 0 {
		errorJSON(c, http.StatusNotFound, "environment not found")
		return
	}
	if err := recordActivity(c.Request.Context(), tx, userID, projectID, ActionEnvDeleted, projectSlug, environment, nil); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to record activity")
		return
	}
	if err := tx.Commit(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to delete environment")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "environment deleted"})
}
