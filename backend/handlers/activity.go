package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/obscurenv/obscurenv/backend/middleware"
)

const (
	ActionProjectCreated = "project.created"
	ActionProjectDeleted = "project.deleted"
	ActionEnvPushed      = "env.pushed"
	ActionEnvDeleted     = "env.deleted"
)

var activityActions = map[string]bool{
	ActionProjectCreated: true,
	ActionProjectDeleted: true,
	ActionEnvPushed:      true,
	ActionEnvDeleted:     true,
}

type activityExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func recordActivity(ctx context.Context, db activityExecer, userID, projectID, action, slug, env string, metadata any) error {
	var payload any
	if metadata != nil {
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		payload = encoded
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO activity_logs (user_id, project_id, action, project_slug, environment_name, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, userID, nullIfEmpty(projectID), action, nullIfEmpty(slug), nullIfEmpty(env), payload)
	return err
}

type ActivityHandler struct {
	db   *sql.DB
	list func(context.Context, string, activityFilter) ([]activityItem, int, error)
}

func NewActivityHandler(database *sql.DB) *ActivityHandler {
	h := &ActivityHandler{db: database}
	h.list = h.listFromDB
	return h
}

type activityItem struct {
	ID              string          `json:"id"`
	Action          string          `json:"action"`
	ProjectSlug     *string         `json:"project_slug,omitempty"`
	EnvironmentName *string         `json:"environment_name,omitempty"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
	CreatedAt       string          `json:"created_at"`
}

type activityFilter struct {
	ProjectSlug string
	Action      string
	Limit       int
	Offset      int
}

func (h *ActivityHandler) List(c *gin.Context) {
	filter, ok := parseActivityFilter(c)
	if !ok {
		return
	}
	activities, total, err := h.list(c.Request.Context(), middleware.UserID(c), filter)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to list activity")
		return
	}
	c.JSON(http.StatusOK, gin.H{"activities": activities, "total": total})
}

func parseActivityFilter(c *gin.Context) (activityFilter, bool) {
	filter := activityFilter{
		ProjectSlug: c.Query("project"),
		Action:      c.Query("action"),
		Limit:       50,
		Offset:      0,
	}
	if limit := c.Query("limit"); limit != "" {
		value, err := strconv.Atoi(limit)
		if err != nil || value < 1 || value > 100 {
			badRequest(c, "limit must be an integer between 1 and 100")
			return filter, false
		}
		filter.Limit = value
	}
	if offset := c.Query("offset"); offset != "" {
		value, err := strconv.Atoi(offset)
		if err != nil || value < 0 {
			badRequest(c, "offset must be a non-negative integer")
			return filter, false
		}
		filter.Offset = value
	}
	if filter.Action != "" && !activityActions[filter.Action] {
		badRequest(c, "unknown action")
		return filter, false
	}
	return filter, true
}

func (h *ActivityHandler) listFromDB(ctx context.Context, userID string, filter activityFilter) ([]activityItem, int, error) {
	query, args := listActivityQuery(userID, filter)
	rows, err := h.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	activities := make([]activityItem, 0)
	for rows.Next() {
		var item activityItem
		var projectSlug sql.NullString
		var environmentName sql.NullString
		var metadata []byte
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.Action, &projectSlug, &environmentName, &metadata, &createdAt); err != nil {
			return nil, 0, err
		}
		if projectSlug.Valid {
			item.ProjectSlug = &projectSlug.String
		}
		if environmentName.Valid {
			item.EnvironmentName = &environmentName.String
		}
		if len(metadata) > 0 {
			item.Metadata = json.RawMessage(metadata)
		}
		item.CreatedAt = formatTime(createdAt)
		activities = append(activities, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	countQuery, countArgs := countActivityQuery(userID, filter)
	var total int
	if err := h.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	return activities, total, nil
}

func activityWhereClause(userID string, filter activityFilter) (string, []any) {
	args := []any{userID}
	where := "WHERE al.user_id = $1"
	if filter.ProjectSlug != "" {
		args = append(args, filter.ProjectSlug)
		where += " AND al.project_slug = $" + strconv.Itoa(len(args))
	}
	if filter.Action != "" {
		args = append(args, filter.Action)
		where += " AND al.action = $" + strconv.Itoa(len(args))
	}
	return where, args
}

func listActivityQuery(userID string, filter activityFilter) (string, []any) {
	where, args := activityWhereClause(userID, filter)
	query := `
		SELECT al.id, al.action, al.project_slug, al.environment_name, al.metadata, al.created_at
		FROM activity_logs al
		` + where + `
		ORDER BY al.created_at DESC, al.id DESC
		LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2)
	args = append(args, filter.Limit, filter.Offset)
	return query, args
}

func countActivityQuery(userID string, filter activityFilter) (string, []any) {
	where, args := activityWhereClause(userID, filter)
	return `
		SELECT COUNT(*)
		FROM activity_logs al
		` + where, args
}
