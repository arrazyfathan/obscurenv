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

const shareInvitationLifetime = 7 * 24 * time.Hour

const (
	sharePending  = "pending"
	shareAccepted = "accepted"
	shareDeclined = "declined"
	shareCanceled = "canceled"
	shareExpired  = "expired"
)

type ProjectAccess struct {
	ProjectID string
	OwnerID   string
	Role      string
}

func (a ProjectAccess) IsOwner() bool { return a.Role == "owner" }

func resolveProjectAccess(db *sql.DB, userID, slug string) (ProjectAccess, error) {
	var access ProjectAccess
	err := db.QueryRow(`
		SELECT p.id, p.user_id,
			CASE WHEN p.user_id = $1 THEN 'owner' ELSE 'collaborator' END
		FROM projects p
		LEFT JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = $1
		WHERE p.slug = $2 AND (p.user_id = $1 OR pm.user_id IS NOT NULL)
	`, userID, slug).Scan(&access.ProjectID, &access.OwnerID, &access.Role)
	return access, err
}

func requireOwner(c *gin.Context, db *sql.DB, slug string) (ProjectAccess, bool) {
	access, err := resolveProjectAccess(db, middleware.UserID(c), slug)
	if errors.Is(err, sql.ErrNoRows) {
		errorJSON(c, http.StatusNotFound, "project not found")
		return access, false
	}
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to resolve project access")
		return access, false
	}
	if !access.IsOwner() {
		errorJSON(c, http.StatusForbidden, "project owner access is required")
		return access, false
	}
	return access, true
}

type accountSummary struct {
	ID          string  `json:"id"`
	Username    *string `json:"username,omitempty"`
	MaskedEmail string  `json:"masked_email"`
}

func accountSummaryFrom(username sql.NullString, email string, id string) accountSummary {
	var name *string
	if username.Valid && username.String != "" {
		value := username.String
		name = &value
	}
	return accountSummary{ID: id, Username: name, MaskedEmail: maskEmail(email)}
}

func maskEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "***"
	}
	local := parts[0][:1] + "***"
	return local + "@" + parts[1]
}

type shareCandidate struct {
	accountSummary
}

type shareInvitationRequest struct {
	RecipientUserID string `json:"recipient_user_id" binding:"required"`
}

type shareInvitation struct {
	ID           string               `json:"id"`
	Project      projectAccessSummary `json:"project"`
	Counterparty accountSummary       `json:"counterparty"`
	Direction    string               `json:"direction"`
	Status       string               `json:"status"`
	CreatedAt    string               `json:"created_at"`
	ExpiresAt    string               `json:"expires_at"`
}

type projectAccessSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type projectMember struct {
	Account  accountSummary `json:"account"`
	JoinedAt string         `json:"joined_at"`
}

type projectAccessResponse struct {
	Owner   accountSummary    `json:"owner"`
	Members []projectMember   `json:"members"`
	Pending []shareInvitation `json:"pending"`
}

type SharingHandler struct{ db *sql.DB }

func NewSharingHandler(database *sql.DB) *SharingHandler { return &SharingHandler{db: database} }

func (h *SharingHandler) Candidates(c *gin.Context) {
	access, ok := requireOwner(c, h.db, c.Param("slug"))
	if !ok {
		return
	}
	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	if len([]rune(query)) < 3 {
		c.JSON(http.StatusOK, gin.H{"candidates": []shareCandidate{}})
		return
	}
	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT u.id, u.username, u.email
		FROM users u
		WHERE u.id <> $1
		  AND u.id <> $2
		  AND NOT EXISTS (SELECT 1 FROM project_members pm WHERE pm.project_id = $3 AND pm.user_id = u.id)
		  AND NOT EXISTS (SELECT 1 FROM project_share_invitations si WHERE si.project_id = $3 AND si.recipient_user_id = u.id AND si.status = 'pending' AND si.expires_at > CURRENT_TIMESTAMP)
		  AND ((u.username IS NOT NULL AND u.username LIKE $4 || '%') OR (u.email = $5 AND POSITION('@' IN $5) > 0))
		ORDER BY u.username NULLS LAST, u.email
		LIMIT 8
	`, middleware.UserID(c), access.OwnerID, access.ProjectID, query, normalizeEmail(query))
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to search accounts")
		return
	}
	defer rows.Close()
	out := make([]shareCandidate, 0)
	for rows.Next() {
		var id, email string
		var username sql.NullString
		if err := rows.Scan(&id, &username, &email); err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to read accounts")
			return
		}
		out = append(out, shareCandidate{accountSummaryFrom(username, email, id)})
	}
	if err := rows.Err(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to read accounts")
		return
	}
	c.JSON(http.StatusOK, gin.H{"candidates": out})
}

func (h *SharingHandler) CreateInvitation(c *gin.Context) {
	access, ok := requireOwner(c, h.db, c.Param("slug"))
	if !ok {
		return
	}
	var req shareInvitationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "recipient_user_id is required")
		return
	}
	userID := middleware.UserID(c)
	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback()
	var lockedUserID string
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT id FROM users WHERE id = $1 FOR UPDATE`, req.RecipientUserID).Scan(&lockedUserID); err != nil {
		errorJSON(c, http.StatusNotFound, "account not found")
		return
	}
	if req.RecipientUserID == userID {
		errorJSON(c, http.StatusBadRequest, "you cannot invite yourself")
		return
	}
	var collision bool
	err = tx.QueryRowContext(c.Request.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM projects p WHERE p.user_id = $1 AND p.slug = $2 AND p.id <> $3
			UNION ALL
			SELECT 1 FROM project_members pm JOIN projects p ON p.id = pm.project_id WHERE pm.user_id = $1 AND p.slug = $2 AND p.id <> $3
		)
	`, req.RecipientUserID, c.Param("slug"), access.ProjectID).Scan(&collision)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to validate account access")
		return
	}
	if collision {
		errorJSON(c, http.StatusConflict, "recipient already has access to a project with this slug")
		return
	}
	var id string
	expires := time.Now().Add(shareInvitationLifetime)
	err = tx.QueryRowContext(c.Request.Context(), `
		INSERT INTO project_share_invitations (project_id, sender_user_id, recipient_user_id, expires_at)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, access.ProjectID, userID, req.RecipientUserID, expires).Scan(&id)
	if err != nil {
		errorJSON(c, http.StatusConflict, "a pending invitation already exists")
		return
	}
	if err := recordActivity(c.Request.Context(), tx, userID, access.ProjectID, ActionShareInvited, c.Param("slug"), "", nil); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to record activity")
		return
	}
	if err := tx.Commit(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to create invitation")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "expires_at": expires.Format(time.RFC3339)})
}

func (h *SharingHandler) Access(c *gin.Context) {
	access, err := resolveProjectAccess(h.db, middleware.UserID(c), c.Param("slug"))
	if errors.Is(err, sql.ErrNoRows) {
		errorJSON(c, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to resolve project access")
		return
	}
	var response projectAccessResponse
	var ownerUsername sql.NullString
	var ownerEmail, ownerID string
	var projectName string
	if err := h.db.QueryRowContext(c.Request.Context(), `SELECT p.id, p.name, u.username, u.email FROM projects p JOIN users u ON u.id = p.user_id WHERE p.id = $1`, access.ProjectID).Scan(&ownerID, &projectName, &ownerUsername, &ownerEmail); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to read project owner")
		return
	}
	response.Owner = accountSummaryFrom(ownerUsername, ownerEmail, ownerID)
	rows, err := h.db.QueryContext(c.Request.Context(), `SELECT u.id, u.username, u.email, pm.joined_at FROM project_members pm JOIN users u ON u.id = pm.user_id WHERE pm.project_id = $1 ORDER BY pm.joined_at`, access.ProjectID)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to list members")
		return
	}
	defer rows.Close()
	response.Members = make([]projectMember, 0)
	for rows.Next() {
		var id, email string
		var username sql.NullString
		var joined time.Time
		if err := rows.Scan(&id, &username, &email, &joined); err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to read members")
			return
		}
		response.Members = append(response.Members, projectMember{Account: accountSummaryFrom(username, email, id), JoinedAt: joined.Format(time.RFC3339)})
	}
	if access.IsOwner() {
		response.Pending, err = h.outgoing(c, access.ProjectID, projectName, c.Param("slug"))
		if err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to list invitations")
			return
		}
	} else {
		response.Pending = []shareInvitation{}
	}
	c.JSON(http.StatusOK, response)
}

func (h *SharingHandler) outgoing(c *gin.Context, projectID, name, slug string) ([]shareInvitation, error) {
	rows, err := h.db.QueryContext(c.Request.Context(), `SELECT si.id, si.status, si.created_at, si.expires_at, u.id, u.username, u.email FROM project_share_invitations si JOIN users u ON u.id = si.recipient_user_id WHERE si.project_id = $1 AND si.status = 'pending' AND si.expires_at > CURRENT_TIMESTAMP ORDER BY si.created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]shareInvitation, 0)
	for rows.Next() {
		var item shareInvitation
		var id, email string
		var username sql.NullString
		var created, expires time.Time
		if err := rows.Scan(&item.ID, &item.Status, &created, &expires, &id, &username, &email); err != nil {
			return nil, err
		}
		item.Project = projectAccessSummary{ID: projectID, Name: name, Slug: slug}
		item.Counterparty = accountSummaryFrom(username, email, id)
		item.Direction = "outgoing"
		item.CreatedAt = created.Format(time.RFC3339)
		item.ExpiresAt = expires.Format(time.RFC3339)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (h *SharingHandler) Invitations(c *gin.Context) {
	userID := middleware.UserID(c)
	rows, err := h.db.QueryContext(c.Request.Context(), `SELECT si.id, p.id, p.name, p.slug, p.user_id, si.status, si.created_at, si.expires_at, u.id, u.username, u.email FROM project_share_invitations si JOIN projects p ON p.id = si.project_id JOIN users u ON u.id = si.sender_user_id WHERE si.recipient_user_id = $1 AND si.status = 'pending' AND si.expires_at > CURRENT_TIMESTAMP ORDER BY si.created_at DESC`, userID)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to list invitations")
		return
	}
	defer rows.Close()
	incoming := make([]shareInvitation, 0)
	for rows.Next() {
		var item shareInvitation
		var projectID, name, slug, ownerID, senderID, email string
		var status string
		var senderUsername sql.NullString
		var created, expires time.Time
		if err := rows.Scan(&item.ID, &projectID, &name, &slug, &ownerID, &status, &created, &expires, &senderID, &senderUsername, &email); err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to read invitations")
			return
		}
		item.Project = projectAccessSummary{ID: projectID, Name: name, Slug: slug}
		item.Counterparty = accountSummaryFrom(senderUsername, email, senderID)
		item.Direction = "incoming"
		item.Status = status
		item.CreatedAt = created.Format(time.RFC3339)
		item.ExpiresAt = expires.Format(time.RFC3339)
		incoming = append(incoming, item)
	}
	if err := rows.Err(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to read invitations")
		return
	}
	c.JSON(http.StatusOK, gin.H{"incoming": incoming})
}

func (h *SharingHandler) Accept(c *gin.Context)  { h.resolveInvitation(c, shareAccepted) }
func (h *SharingHandler) Decline(c *gin.Context) { h.resolveInvitation(c, shareDeclined) }

func (h *SharingHandler) resolveInvitation(c *gin.Context, result string) {
	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback()
	var status, projectID, slug, ownerID string
	var expires time.Time
	err = tx.QueryRowContext(c.Request.Context(), `SELECT si.status, si.project_id, p.slug, p.user_id, si.expires_at FROM project_share_invitations si JOIN projects p ON p.id = si.project_id WHERE si.id = $1 AND si.recipient_user_id = $2 FOR UPDATE`, c.Param("id"), middleware.UserID(c)).Scan(&status, &projectID, &slug, &ownerID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		errorJSON(c, http.StatusNotFound, "invitation not found")
		return
	}
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to read invitation")
		return
	}
	if status != sharePending || time.Now().After(expires) {
		_, _ = tx.ExecContext(c.Request.Context(), `UPDATE project_share_invitations SET status = 'expired', resolved_at = CURRENT_TIMESTAMP WHERE id = $1 AND status = 'pending'`, c.Param("id"))
		errorJSON(c, http.StatusConflict, "invitation is no longer available")
		return
	}
	if result == shareAccepted {
		var collision bool
		if err := tx.QueryRowContext(c.Request.Context(), `SELECT EXISTS(SELECT 1 FROM projects WHERE user_id = $1 AND slug = $2 AND id <> $3 UNION ALL SELECT 1 FROM project_members pm JOIN projects p ON p.id = pm.project_id WHERE pm.user_id = $1 AND p.slug = $2 AND p.id <> $3)`, middleware.UserID(c), slug, projectID).Scan(&collision); err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to validate project access")
			return
		}
		if collision {
			errorJSON(c, http.StatusConflict, "you already have access to a project with this slug")
			return
		}
		if _, err := tx.ExecContext(c.Request.Context(), `INSERT INTO project_members (project_id, user_id, invited_by) VALUES ($1, $2, $3)`, projectID, middleware.UserID(c), ownerID); err != nil {
			errorJSON(c, http.StatusConflict, "project membership already exists")
			return
		}
	}
	if _, err := tx.ExecContext(c.Request.Context(), `UPDATE project_share_invitations SET status = $2, resolved_at = CURRENT_TIMESTAMP WHERE id = $1`, c.Param("id"), result); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to resolve invitation")
		return
	}
	action := ActionShareDeclined
	if result == shareAccepted {
		action = ActionShareAccepted
	}
	if err := recordActivity(c.Request.Context(), tx, middleware.UserID(c), projectID, action, slug, "", nil); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to record activity")
		return
	}
	if err := tx.Commit(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to resolve invitation")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "invitation " + result})
}

func (h *SharingHandler) Cancel(c *gin.Context) {
	result, err := h.db.ExecContext(c.Request.Context(), `UPDATE project_share_invitations si SET status = 'canceled', resolved_at = CURRENT_TIMESTAMP FROM projects p WHERE si.id = $1 AND si.project_id = p.id AND p.user_id = $2 AND si.status = 'pending'`, c.Param("id"), middleware.UserID(c))
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to cancel invitation")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		errorJSON(c, http.StatusNotFound, "invitation not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "invitation canceled"})
}

func (h *SharingHandler) RemoveMember(c *gin.Context) {
	access, ok := requireOwner(c, h.db, c.Param("slug"))
	if !ok {
		return
	}
	if c.Param("user_id") == access.OwnerID {
		badRequest(c, "project owner cannot be removed")
		return
	}
	result, err := h.db.ExecContext(c.Request.Context(), `DELETE FROM project_members WHERE project_id = $1 AND user_id = $2`, access.ProjectID, c.Param("user_id"))
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to remove member")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		errorJSON(c, http.StatusNotFound, "member not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "member removed"})
}

func (h *SharingHandler) Leave(c *gin.Context) {
	access, err := resolveProjectAccess(h.db, middleware.UserID(c), c.Param("slug"))
	if errors.Is(err, sql.ErrNoRows) {
		errorJSON(c, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to resolve project access")
		return
	}
	if access.IsOwner() {
		badRequest(c, "project owner cannot leave")
		return
	}
	if _, err := h.db.ExecContext(c.Request.Context(), `DELETE FROM project_members WHERE project_id = $1 AND user_id = $2`, access.ProjectID, middleware.UserID(c)); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to leave project")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "left project"})
}
