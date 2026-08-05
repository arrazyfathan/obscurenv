package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/obscurenv/obscurenv/backend/middleware"
)

type ProjectHandler struct {
	db *sql.DB
}

func NewProjectHandler(database *sql.DB) *ProjectHandler {
	return &ProjectHandler{db: database}
}

type createProjectRequest struct {
	Name string `json:"name" binding:"required"`
	Slug string `json:"slug" binding:"required"`
}

type projectSummary struct {
	ID               string              `json:"id"`
	Name             string              `json:"name"`
	Slug             string              `json:"slug"`
	CreatedAt        string              `json:"created_at"`
	EnvironmentCount int                 `json:"environment_count"`
	LatestVersion    *int                `json:"latest_version"`
	LatestUpdatedAt  *string             `json:"latest_updated_at"`
	Environments     []environmentSummary `json:"environments,omitempty"`
}

type environmentSummary struct {
	Name          string `json:"name"`
	LatestVersion int    `json:"latest_version"`
	Checksum      string `json:"checksum"`
	UpdatedAt     string `json:"updated_at"`
}

func (h *ProjectHandler) Create(c *gin.Context) {
	var req createProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid project request")
		return
	}
	var id string
	err := h.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO projects (user_id, name, slug)
		VALUES ($1, $2, $3)
		RETURNING id
	`, middleware.UserID(c), req.Name, req.Slug).Scan(&id)
	if err != nil {
		errorJSON(c, http.StatusConflict, "project already exists")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "slug": req.Slug})
}

func (h *ProjectHandler) List(c *gin.Context) {
	search := c.Query("search")
	if search == "" {
		search = c.Query("q")
	}

	query, args := listProjectsQuery(middleware.UserID(c), search)
	rows, err := h.db.QueryContext(c.Request.Context(), query, args...)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to list projects")
		return
	}
	defer rows.Close()

	projects := make([]projectSummary, 0)
	for rows.Next() {
		project, err := scanProjectSummary(rows)
		if err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to read projects")
			return
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to read projects")
		return
	}

	if len(projects) > 0 {
		environmentsByProject, err := h.latestEnvironmentsByProject(c.Request.Context(), projects)
		if err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to list environments")
			return
		}
		for i := range projects {
			projects[i].Environments = environmentsByProject[projects[i].ID]
		}
	}
	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

func (h *ProjectHandler) latestEnvironmentsByProject(ctx context.Context, projects []projectSummary) (map[string][]environmentSummary, error) {
	ids := make([]string, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, project.ID)
	}

	rows, err := h.db.QueryContext(ctx, `
		SELECT project_id, environment_name, version, checksum, created_at
		FROM (
			SELECT
				ev.project_id,
				ev.environment_name,
				ev.version,
				ev.checksum,
				ev.created_at,
				ROW_NUMBER() OVER (
					PARTITION BY ev.project_id, ev.environment_name
					ORDER BY ev.version DESC
				) AS row_number
			FROM env_versions ev
			WHERE ev.project_id = ANY($1::uuid[])
		) latest
		WHERE row_number = 1
		ORDER BY environment_name
	`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	environments := make(map[string][]environmentSummary)
	for rows.Next() {
		var projectID string
		var environment environmentSummary
		var updatedAt time.Time
		if err := rows.Scan(&projectID, &environment.Name, &environment.LatestVersion, &environment.Checksum, &updatedAt); err != nil {
			return nil, err
		}
		environment.UpdatedAt = formatTime(updatedAt)
		environments[projectID] = append(environments[projectID], environment)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return environments, nil
}

func listProjectsQuery(userID, search string) (string, []any) {
	args := []any{userID}
	where := "WHERE p.user_id = $1"

	search = strings.TrimSpace(search)
	if search != "" {
		args = append(args, "%"+escapePostgresLike(search)+"%")
		where += ` AND (
			p.name ILIKE $2 ESCAPE '\'
			OR p.slug ILIKE $2 ESCAPE '\'
			OR EXISTS (
				SELECT 1
				FROM env_versions env
				WHERE env.project_id = p.id
					AND env.environment_name ILIKE $2 ESCAPE '\'
			)
		)`
	}

	return `
		SELECT
			p.id,
			p.name,
			p.slug,
			p.created_at,
			COUNT(DISTINCT ev.environment_name) AS environment_count,
			MAX(ev.version) AS latest_version,
			MAX(ev.created_at) AS latest_updated_at
		FROM projects p
		LEFT JOIN env_versions ev ON ev.project_id = p.id
		` + where + `
		GROUP BY p.id, p.name, p.slug, p.created_at
		ORDER BY COALESCE(MAX(ev.created_at), p.created_at) DESC
	`, args
}

func escapePostgresLike(value string) string {
	var builder strings.Builder
	for _, char := range value {
		switch char {
		case '\\', '%', '_':
			builder.WriteRune('\\')
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

func (h *ProjectHandler) Get(c *gin.Context) {
	slug := c.Param("slug")

	project, err := scanProjectSummary(h.db.QueryRowContext(c.Request.Context(), `
		SELECT
			p.id,
			p.name,
			p.slug,
			p.created_at,
			COUNT(DISTINCT ev.environment_name) AS environment_count,
			MAX(ev.version) AS latest_version,
			MAX(ev.created_at) AS latest_updated_at
		FROM projects p
		LEFT JOIN env_versions ev ON ev.project_id = p.id
		WHERE p.user_id = $1 AND p.slug = $2
		GROUP BY p.id, p.name, p.slug, p.created_at
	`, middleware.UserID(c), slug))
	if err != nil {
		errorJSON(c, http.StatusNotFound, "project not found")
		return
	}

	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT environment_name, version, checksum, created_at
		FROM (
			SELECT
				ev.environment_name,
				ev.version,
				ev.checksum,
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
	`, middleware.UserID(c), slug)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to list environments")
		return
	}
	defer rows.Close()

	environments := make([]environmentSummary, 0)
	for rows.Next() {
		var environment environmentSummary
		var updatedAt time.Time
		if err := rows.Scan(&environment.Name, &environment.LatestVersion, &environment.Checksum, &updatedAt); err != nil {
			errorJSON(c, http.StatusInternalServerError, "failed to read environments")
			return
		}
		environment.UpdatedAt = formatTime(updatedAt)
		environments = append(environments, environment)
	}
	if err := rows.Err(); err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to read environments")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":                project.ID,
		"name":              project.Name,
		"slug":              project.Slug,
		"created_at":        project.CreatedAt,
		"environment_count": project.EnvironmentCount,
		"latest_version":    project.LatestVersion,
		"latest_updated_at": project.LatestUpdatedAt,
		"environments":      environments,
	})
}

func (h *ProjectHandler) Delete(c *gin.Context) {
	result, err := h.db.ExecContext(c.Request.Context(), `
		DELETE FROM projects
		WHERE user_id = $1 AND slug = $2
	`, middleware.UserID(c), c.Param("slug"))
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to delete project")
		return
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "failed to confirm project deletion")
		return
	}
	if deleted == 0 {
		errorJSON(c, http.StatusNotFound, "project not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "project deleted"})
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanProjectSummary(row rowScanner) (projectSummary, error) {
	var project projectSummary
	var createdAt time.Time
	var latestVersion sql.NullInt64
	var latestUpdatedAt sql.NullTime
	err := row.Scan(
		&project.ID,
		&project.Name,
		&project.Slug,
		&createdAt,
		&project.EnvironmentCount,
		&latestVersion,
		&latestUpdatedAt,
	)
	if err != nil {
		return project, err
	}
	project.CreatedAt = formatTime(createdAt)
	if latestVersion.Valid {
		value := int(latestVersion.Int64)
		project.LatestVersion = &value
	}
	if latestUpdatedAt.Valid {
		value := formatTime(latestUpdatedAt.Time)
		project.LatestUpdatedAt = &value
	}
	return project, err
}

func formatTime(value time.Time) string {
	return value.Format(time.RFC3339)
}
