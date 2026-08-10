package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/obscurenv/obscurenv/backend/middleware"
)

const projectTransferLifetime = 7 * 24 * time.Hour

const (
	transferPending  = "pending"
	transferAccepted = "accepted"
	transferDeclined = "declined"
	transferCanceled = "canceled"
	transferExpired  = "expired"
)

type projectTransferRequest struct {
	Recipient    string `json:"recipient" binding:"required"`
	Confirmation string `json:"confirmation" binding:"required"`
}

type transferAccount struct {
	ID       string
	Username sql.NullString
	Email    string
}

func (account transferAccount) Label() string {
	if account.Username.Valid && account.Username.String != "" {
		return account.Username.String
	}
	return account.Email
}

type projectTransferSummary struct {
	ID           string                 `json:"id"`
	Project      projectTransferProject `json:"project"`
	Counterparty string                 `json:"counterparty"`
	Direction    string                 `json:"direction"`
	Status       string                 `json:"status"`
	CreatedAt    string                 `json:"created_at"`
	ExpiresAt    string                 `json:"expires_at"`
}

type projectTransferProject struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Slug             string `json:"slug"`
	EnvironmentCount int    `json:"environment_count"`
}

type projectTransferHandler struct {
	db *sql.DB
}

func NewProjectTransferHandler(database *sql.DB) *projectTransferHandler {
	return &projectTransferHandler{db: database}
}

func (h *projectTransferHandler) Create(c *gin.Context) {
	userID := middleware.UserID(c)
	var req projectTransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid transfer request")
		return
	}
	identifier := strings.TrimSpace(req.Recipient)
	if identifier == "" || strings.TrimSpace(req.Confirmation) == "" {
		badRequest(c, "recipient and confirmation are required")
		return
	}

	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to start transfer")
		return
	}
	defer tx.Rollback()

	project, err := lockOwnedProject(c, tx, userID, c.Param("slug"))
	if errors.Is(err, sql.ErrNoRows) {
		errorJSON(c, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to find project")
		return
	}
	if req.Confirmation != project.Slug {
		badRequest(c, "confirmation must match the project slug")
		return
	}

	recipient, err := findTransferAccount(c, tx, identifier)
	if errors.Is(err, sql.ErrNoRows) {
		errorJSON(c, http.StatusNotFound, "recipient account not found")
		return
	}
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to find recipient")
		return
	}
	if recipient.ID == userID {
		badRequest(c, "project is already owned by this account")
		return
	}

	var collision bool
	if err := tx.QueryRowContext(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM projects WHERE user_id = $1 AND slug = $2)
	`, recipient.ID, project.Slug).Scan(&collision); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to check recipient projects")
		return
	}
	if collision {
		errorJSON(c, http.StatusConflict, "recipient already has a project with this slug")
		return
	}

	// Clear stale invitations so the partial unique index can accept a new one.
	_, err = tx.ExecContext(c.Request.Context(), `
		UPDATE project_transfers
		SET status = $2, resolved_at = CURRENT_TIMESTAMP
		WHERE project_id = $1 AND status = $3 AND expires_at <= CURRENT_TIMESTAMP
	`, project.ID, transferExpired, transferPending)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to expire old transfer")
		return
	}

	expiresAt := time.Now().UTC().Add(projectTransferLifetime)
	var transferID string
	var createdAt time.Time
	err = tx.QueryRowContext(c.Request.Context(), `
		INSERT INTO project_transfers (project_id, sender_user_id, recipient_user_id, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`, project.ID, userID, recipient.ID, expiresAt).Scan(&transferID, &createdAt)
	if err != nil {
		if isUniqueConstraint(err) {
			errorJSON(c, http.StatusConflict, "a transfer is already pending for this project")
			return
		}
		errorJSON(c, http.StatusInternalServerError, "failed to create transfer")
		return
	}
	metadata := gin.H{"direction": "outgoing", "counterparty": recipient.Label()}
	if err := recordActivity(c.Request.Context(), tx, userID, project.ID, ActionTransferRequested, project.Slug, "", metadata); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to record transfer activity")
		return
	}
	if err := recordActivity(c.Request.Context(), tx, recipient.ID, project.ID, ActionTransferRequested, project.Slug, "", gin.H{"direction": "incoming", "counterparty": projectOwnerLabel(c, tx, userID)}); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to record transfer activity")
		return
	}
	if err := tx.Commit(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to create transfer")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"transfer": projectTransferSummary{
		ID: transferID, Project: project, Counterparty: recipient.Label(), Direction: "outgoing", Status: transferPending,
		CreatedAt: createdAt.Format(time.RFC3339), ExpiresAt: expiresAt.Format(time.RFC3339),
	}})
}

func (h *projectTransferHandler) List(c *gin.Context) {
	userID := middleware.UserID(c)
	incoming, err := listProjectTransfers(c, h.db, userID, true)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to list incoming transfers")
		return
	}
	outgoing, err := listProjectTransfers(c, h.db, userID, false)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to list outgoing transfers")
		return
	}
	c.JSON(http.StatusOK, gin.H{"incoming": incoming, "outgoing": outgoing})
}

func (h *projectTransferHandler) Accept(c *gin.Context) {
	userID := middleware.UserID(c)
	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to start transfer")
		return
	}
	defer tx.Rollback()

	transfer, sender, recipient, project, err := lockTransfer(c, tx, c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		errorJSON(c, http.StatusNotFound, "transfer not found")
		return
	}
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to find transfer")
		return
	}
	if recipient.ID != userID {
		errorJSON(c, http.StatusForbidden, "transfer is not addressed to this account")
		return
	}
	if transfer != transferPending {
		errorJSON(c, http.StatusConflict, "transfer is no longer pending")
		return
	}
	if time.Now().UTC().After(project.ExpiresAt) {
		if _, err := tx.ExecContext(c.Request.Context(), `UPDATE project_transfers SET status = $2, resolved_at = CURRENT_TIMESTAMP WHERE id = $1`, c.Param("id"), transferExpired); err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to expire transfer")
			return
		}
		if err := tx.Commit(); err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to expire transfer")
			return
		}
		errorJSON(c, http.StatusGone, "transfer invitation has expired")
		return
	}
	if project.OwnerID != sender.ID {
		errorJSON(c, http.StatusConflict, "project ownership has changed")
		return
	}
	var collision bool
	if err := tx.QueryRowContext(c.Request.Context(), `SELECT EXISTS(SELECT 1 FROM projects WHERE user_id = $1 AND slug = $2 AND id <> $3)`, userID, project.Slug, project.ID).Scan(&collision); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to check project slug")
		return
	}
	if collision {
		errorJSON(c, http.StatusConflict, "you already have a project with this slug")
		return
	}
	if _, err := tx.ExecContext(c.Request.Context(), `UPDATE projects SET user_id = $2 WHERE id = $1`, project.ID, userID); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to transfer project")
		return
	}
	if _, err := tx.ExecContext(c.Request.Context(), `UPDATE project_transfers SET status = $2, resolved_at = CURRENT_TIMESTAMP WHERE id = $1`, c.Param("id"), transferAccepted); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to resolve transfer")
		return
	}
	if err := recordActivity(c.Request.Context(), tx, sender.ID, project.ID, ActionProjectTransferred, project.Slug, "", gin.H{"direction": "outgoing", "counterparty": recipient.Label()}); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to record transfer activity")
		return
	}
	if err := recordActivity(c.Request.Context(), tx, recipient.ID, project.ID, ActionProjectTransferred, project.Slug, "", gin.H{"direction": "incoming", "counterparty": sender.Label()}); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to record transfer activity")
		return
	}
	if err := tx.Commit(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to transfer project")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "project transferred", "project": projectTransferProject{ID: project.ID, Name: project.Name, Slug: project.Slug, EnvironmentCount: project.EnvironmentCount}})
}

func (h *projectTransferHandler) Decline(c *gin.Context) {
	h.resolve(c, transferDeclined, false)
}

func (h *projectTransferHandler) Cancel(c *gin.Context) {
	h.resolve(c, transferCanceled, true)
}

func (h *projectTransferHandler) resolve(c *gin.Context, status string, senderAction bool) {
	userID := middleware.UserID(c)
	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to start transfer")
		return
	}
	defer tx.Rollback()
	transfer, sender, recipient, project, err := lockTransfer(c, tx, c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		errorJSON(c, http.StatusNotFound, "transfer not found")
		return
	}
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to find transfer")
		return
	}
	authorized := (senderAction && sender.ID == userID) || (!senderAction && recipient.ID == userID)
	if !authorized {
		errorJSON(c, http.StatusForbidden, "you cannot resolve this transfer")
		return
	}
	if transfer != transferPending {
		errorJSON(c, http.StatusConflict, "transfer is no longer pending")
		return
	}
	if time.Now().UTC().After(project.ExpiresAt) {
		if _, err := tx.ExecContext(c.Request.Context(), `UPDATE project_transfers SET status = $2, resolved_at = CURRENT_TIMESTAMP WHERE id = $1`, c.Param("id"), transferExpired); err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to expire transfer")
			return
		}
		if err := tx.Commit(); err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to expire transfer")
			return
		}
		errorJSON(c, http.StatusGone, "transfer invitation has expired")
		return
	}
	if _, err := tx.ExecContext(c.Request.Context(), `UPDATE project_transfers SET status = $2, resolved_at = CURRENT_TIMESTAMP WHERE id = $1`, c.Param("id"), status); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to resolve transfer")
		return
	}
	action := ActionTransferDeclined
	if senderAction {
		action = ActionTransferCanceled
	}
	actorMetadata := gin.H{"direction": map[bool]string{true: "outgoing", false: "incoming"}[senderAction], "counterparty": map[bool]string{true: recipient.Label(), false: sender.Label()}[senderAction]}
	if err := recordActivity(c.Request.Context(), tx, userID, project.ID, action, project.Slug, "", actorMetadata); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to record transfer activity")
		return
	}
	otherID := recipient.ID
	otherDirection := "incoming"
	if !senderAction {
		otherID = sender.ID
		otherDirection = "outgoing"
	}
	if err := recordActivity(c.Request.Context(), tx, otherID, project.ID, action, project.Slug, "", gin.H{"direction": otherDirection, "counterparty": map[bool]string{true: recipient.Label(), false: sender.Label()}[!senderAction]}); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to record transfer activity")
		return
	}
	if err := tx.Commit(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to resolve transfer")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "transfer resolved", "status": status})
}

type transferProjectRow struct {
	ID               string
	Name             string
	Slug             string
	OwnerID          string
	EnvironmentCount int
	ExpiresAt        time.Time
}

func lockOwnedProject(c *gin.Context, tx *sql.Tx, userID, slug string) (projectTransferProject, error) {
	var project projectTransferProject
	err := tx.QueryRowContext(c.Request.Context(), `SELECT p.id, p.name, p.slug, COUNT(DISTINCT ev.environment_name) FROM projects p LEFT JOIN env_versions ev ON ev.project_id = p.id WHERE p.user_id = $1 AND p.slug = $2 GROUP BY p.id`, userID, slug).Scan(&project.ID, &project.Name, &project.Slug, &project.EnvironmentCount)
	if err == nil {
		var lockedID string
		err = tx.QueryRowContext(c.Request.Context(), `SELECT id FROM projects WHERE id = $1 FOR UPDATE`, project.ID).Scan(&lockedID)
	}
	return project, err
}

func findTransferAccount(c *gin.Context, tx *sql.Tx, identifier string) (transferAccount, error) {
	var account transferAccount
	column, value := "username", normalizeUsername(identifier)
	if strings.Contains(identifier, "@") {
		column, value = "email", normalizeEmail(identifier)
	}
	if value == "" {
		return account, sql.ErrNoRows
	}
	err := tx.QueryRowContext(c.Request.Context(), "SELECT id, username, email FROM users WHERE "+column+" = $1", value).Scan(&account.ID, &account.Username, &account.Email)
	return account, err
}

func projectOwnerLabel(c *gin.Context, tx *sql.Tx, userID string) string {
	var account transferAccount
	if err := tx.QueryRowContext(c.Request.Context(), `SELECT id, username, email FROM users WHERE id = $1`, userID).Scan(&account.ID, &account.Username, &account.Email); err != nil {
		return "another account"
	}
	return account.Label()
}

func listProjectTransfers(c *gin.Context, db *sql.DB, userID string, incoming bool) ([]projectTransferSummary, error) {
	partyColumn := "pt.sender_user_id"
	counterpartyColumn := "pt.recipient_user_id"
	direction := "outgoing"
	if incoming {
		partyColumn, counterpartyColumn, direction = "pt.recipient_user_id", "pt.sender_user_id", "incoming"
	}
	rows, err := db.QueryContext(c.Request.Context(), `
		SELECT pt.id, p.id, p.name, p.slug, COUNT(DISTINCT ev.environment_name), u.username, u.email, pt.created_at, pt.expires_at
		FROM project_transfers pt
		JOIN projects p ON p.id = pt.project_id
		JOIN users u ON u.id = `+counterpartyColumn+`
		LEFT JOIN env_versions ev ON ev.project_id = p.id
		WHERE `+partyColumn+` = $1 AND pt.status = 'pending' AND pt.expires_at > CURRENT_TIMESTAMP
		GROUP BY pt.id, p.id, p.name, p.slug, u.username, u.email
		ORDER BY pt.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	transfers := make([]projectTransferSummary, 0)
	for rows.Next() {
		var item projectTransferSummary
		var username sql.NullString
		var email string
		var createdAt, expiresAt time.Time
		if err := rows.Scan(&item.ID, &item.Project.ID, &item.Project.Name, &item.Project.Slug, &item.Project.EnvironmentCount, &username, &email, &createdAt, &expiresAt); err != nil {
			return nil, err
		}
		item.Counterparty = transferAccount{Username: username, Email: email}.Label()
		item.Direction, item.Status = direction, transferPending
		item.CreatedAt, item.ExpiresAt = createdAt.Format(time.RFC3339), expiresAt.Format(time.RFC3339)
		transfers = append(transfers, item)
	}
	return transfers, rows.Err()
}

func lockTransfer(c *gin.Context, tx *sql.Tx, id string) (string, transferAccount, transferAccount, transferProjectRow, error) {
	var status string
	var sender, recipient transferAccount
	var project transferProjectRow
	err := tx.QueryRowContext(c.Request.Context(), `
		SELECT pt.status, s.id, s.username, s.email, r.id, r.username, r.email, p.id, p.name, p.slug, p.user_id, COUNT(DISTINCT ev.environment_name), pt.expires_at
		FROM project_transfers pt
		JOIN projects p ON p.id = pt.project_id
		JOIN users s ON s.id = pt.sender_user_id
		JOIN users r ON r.id = pt.recipient_user_id
		LEFT JOIN env_versions ev ON ev.project_id = p.id
		WHERE pt.id = $1
		GROUP BY pt.id, s.id, r.id, p.id
	`, id).Scan(&status, &sender.ID, &sender.Username, &sender.Email, &recipient.ID, &recipient.Username, &recipient.Email, &project.ID, &project.Name, &project.Slug, &project.OwnerID, &project.EnvironmentCount, &project.ExpiresAt)
	if err == nil {
		var lockedID string
		if lockErr := tx.QueryRowContext(c.Request.Context(), `SELECT id FROM projects WHERE id = $1 FOR UPDATE`, project.ID).Scan(&lockedID); lockErr != nil {
			err = lockErr
		}
		if err == nil {
			if lockErr := tx.QueryRowContext(c.Request.Context(), `SELECT id FROM project_transfers WHERE id = $1 FOR UPDATE`, id).Scan(&lockedID); lockErr != nil {
				err = lockErr
			}
		}
	}
	return status, sender, recipient, project, err
}
