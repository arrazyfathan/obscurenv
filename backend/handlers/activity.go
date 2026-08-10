package handlers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/obscurenv/obscurenv/backend/middleware"
)

const (
	ActionProjectCreated     = "project.created"
	ActionProjectDeleted     = "project.deleted"
	ActionProjectRenamed     = "project.renamed"
	ActionProjectExported    = "project.exported"
	ActionTransferRequested  = "project.transfer_requested"
	ActionTransferCanceled   = "project.transfer_canceled"
	ActionTransferDeclined   = "project.transfer_declined"
	ActionProjectTransferred = "project.transferred"
	ActionShareInvited       = "project.share_invited"
	ActionShareAccepted      = "project.share_accepted"
	ActionShareDeclined      = "project.share_declined"
	ActionShareCanceled      = "project.share_canceled"
	ActionShareRemoved       = "project.share_removed"
	ActionShareLeft          = "project.share_left"
	ActionEnvPushed          = "env.pushed"
	ActionEnvDeleted         = "env.deleted"
)

var activityActions = map[string]bool{
	ActionProjectCreated:     true,
	ActionProjectDeleted:     true,
	ActionProjectRenamed:     true,
	ActionProjectExported:    true,
	ActionTransferRequested:  true,
	ActionTransferCanceled:   true,
	ActionTransferDeclined:   true,
	ActionProjectTransferred: true,
	ActionShareInvited:       true,
	ActionShareAccepted:      true,
	ActionShareDeclined:      true,
	ActionShareCanceled:      true,
	ActionShareRemoved:       true,
	ActionShareLeft:          true,
	ActionEnvPushed:          true,
	ActionEnvDeleted:         true,
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
	list func(context.Context, string, activityFilter) ([]activityItem, int, string, error)
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
	ProjectSlug     string
	Action          string
	Limit           int
	Offset          int
	From            time.Time
	To              time.Time
	HasFrom         bool
	HasTo           bool
	BeforeID        string
	BeforeCreatedAt time.Time
	HasBefore       bool
}

func (h *ActivityHandler) List(c *gin.Context) {
	filter, ok := parseActivityFilter(c)
	if !ok {
		return
	}
	activities, total, nextCursor, err := h.list(c.Request.Context(), middleware.UserID(c), filter)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to list activity")
		return
	}
	resp := gin.H{"activities": activities, "total": total}
	if nextCursor != "" {
		resp["next_cursor"] = nextCursor
	}
	c.JSON(http.StatusOK, resp)
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
	if from := c.Query("from"); from != "" {
		value, err := time.Parse(time.RFC3339, from)
		if err != nil {
			badRequest(c, "from must be an RFC3339 timestamp")
			return filter, false
		}
		filter.From = value
		filter.HasFrom = true
	}
	if to := c.Query("to"); to != "" {
		value, err := time.Parse(time.RFC3339, to)
		if err != nil {
			badRequest(c, "to must be an RFC3339 timestamp")
			return filter, false
		}
		filter.To = value
		filter.HasTo = true
	}
	if cursor := c.Query("cursor"); cursor != "" {
		cursor, err := decodeActivityCursor(cursor)
		if err != nil {
			badRequest(c, "invalid cursor")
			return filter, false
		}
		filter.BeforeID = cursor.ID
		filter.BeforeCreatedAt = cursor.CreatedAt
		filter.HasBefore = true
	}
	if filter.Action != "" && !activityActions[filter.Action] {
		badRequest(c, "unknown action")
		return filter, false
	}
	return filter, true
}

type activityCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func encodeActivityCursor(createdAt time.Time, id string) string {
	data, _ := json.Marshal(activityCursor{CreatedAt: createdAt, ID: id})
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeActivityCursor(encoded string) (activityCursor, error) {
	var cursor activityCursor
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return cursor, err
	}
	if err := json.Unmarshal(data, &cursor); err != nil {
		return cursor, err
	}
	if cursor.CreatedAt.IsZero() || cursor.ID == "" {
		return cursor, errors.New("incomplete cursor")
	}
	return cursor, nil
}

func (h *ActivityHandler) listFromDB(ctx context.Context, userID string, filter activityFilter) ([]activityItem, int, string, error) {
	fetchLimit := filter.Limit
	if filter.HasBefore {
		fetchLimit = filter.Limit + 1
	}
	query, args := listActivityQuery(userID, filter, fetchLimit)
	rows, err := h.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, "", err
	}
	defer rows.Close()

	type rowWithTime struct {
		item      activityItem
		createdAt time.Time
	}
	rowsWithTime := make([]rowWithTime, 0)
	for rows.Next() {
		var row rowWithTime
		var projectSlug sql.NullString
		var environmentName sql.NullString
		var metadata []byte
		if err := rows.Scan(&row.item.ID, &row.item.Action, &projectSlug, &environmentName, &metadata, &row.createdAt); err != nil {
			return nil, 0, "", err
		}
		if projectSlug.Valid {
			row.item.ProjectSlug = &projectSlug.String
		}
		if environmentName.Valid {
			row.item.EnvironmentName = &environmentName.String
		}
		if len(metadata) > 0 {
			row.item.Metadata = json.RawMessage(metadata)
		}
		row.item.CreatedAt = formatTime(row.createdAt)
		rowsWithTime = append(rowsWithTime, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, "", err
	}

	nextCursor := ""
	if len(rowsWithTime) > filter.Limit {
		last := rowsWithTime[filter.Limit-1]
		nextCursor = encodeActivityCursor(last.createdAt, last.item.ID)
		rowsWithTime = rowsWithTime[:filter.Limit]
	}

	activities := make([]activityItem, 0, len(rowsWithTime))
	for _, row := range rowsWithTime {
		activities = append(activities, row.item)
	}

	countQuery, countArgs := countActivityQuery(userID, filter)
	var total int
	if err := h.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, "", err
	}
	return activities, total, nextCursor, nil
}

func activityWhereClause(userID string, filter activityFilter) (string, []any) {
	args := []any{userID}
	where := "/* WHERE al.user_id = $1 */ WHERE (al.user_id = $1 OR EXISTS (SELECT 1 FROM projects p WHERE p.id = al.project_id AND (p.user_id = $1 OR EXISTS (SELECT 1 FROM project_members pm WHERE pm.project_id = p.id AND pm.user_id = $1))))"
	if filter.ProjectSlug != "" {
		args = append(args, filter.ProjectSlug)
		where += " AND al.project_slug = $" + strconv.Itoa(len(args))
	}
	if filter.Action != "" {
		args = append(args, filter.Action)
		where += " AND al.action = $" + strconv.Itoa(len(args))
	}
	if filter.HasFrom {
		args = append(args, filter.From)
		where += " AND al.created_at >= $" + strconv.Itoa(len(args))
	}
	if filter.HasTo {
		args = append(args, filter.To)
		where += " AND al.created_at <= $" + strconv.Itoa(len(args))
	}
	if filter.HasBefore {
		createdAtArg := len(args) + 1
		idArg := len(args) + 2
		args = append(args, filter.BeforeCreatedAt, filter.BeforeID)
		where += " AND (al.created_at < $" + strconv.Itoa(createdAtArg) + " OR (al.created_at = $" + strconv.Itoa(createdAtArg) + " AND al.id < $" + strconv.Itoa(idArg) + "))"
	}
	return where, args
}

func listActivityQuery(userID string, filter activityFilter, fetchLimit int) (string, []any) {
	where, args := activityWhereClause(userID, filter)
	query := `
		SELECT al.id, al.action, al.project_slug, al.environment_name, al.metadata, al.created_at
		FROM activity_logs al
		` + where + `
		ORDER BY al.created_at DESC, al.id DESC
		LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2)
	args = append(args, fetchLimit, filter.Offset)
	return query, args
}

func countActivityQuery(userID string, filter activityFilter) (string, []any) {
	where, args := activityWhereClause(userID, filter)
	return `
		SELECT COUNT(*)
		FROM activity_logs al
		` + where, args
}
