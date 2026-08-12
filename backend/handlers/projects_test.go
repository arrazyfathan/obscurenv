package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestListProjectsQueryWithoutSearchListsUserProjects(t *testing.T) {
	query, args := listProjectsQuery("user-1", " ")

	if strings.Contains(query, "ILIKE") {
		t.Fatalf("query = %q, should not include search filter", query)
	}
	if len(args) != 1 || args[0] != "user-1" {
		t.Fatalf("args = %#v, want user id only", args)
	}
}

func TestListProjectsQueryWithSearchFiltersByNameOrSlug(t *testing.T) {
	query, args := listProjectsQuery("user-1", " api ")

	if !strings.Contains(query, "p.name ILIKE $2") || !strings.Contains(query, "p.slug ILIKE $2") {
		t.Fatalf("query = %q, want name and slug search filter", query)
	}
	if len(args) != 2 || args[0] != "user-1" || args[1] != "%api%" {
		t.Fatalf("args = %#v, want user id and search pattern", args)
	}
}

func TestListProjectsQueryWithSearchAlsoMatchesEnvironmentName(t *testing.T) {
	query, args := listProjectsQuery("user-1", "staging")

	if !strings.Contains(query, "env.environment_name ILIKE $2") {
		t.Fatalf("query = %q, want environment name search filter", query)
	}
	if len(args) != 2 || args[0] != "user-1" || args[1] != "%staging%" {
		t.Fatalf("args = %#v, want user id and search pattern", args)
	}
}

func TestListProjectsQueryWithSearchMatchesEnvironmentWithoutProjectMatch(t *testing.T) {
	query, _ := listProjectsQuery("user-1", "staging")

	if !strings.Contains(query, "EXISTS (") {
		t.Fatalf("query = %q, want EXISTS subquery for environment search", query)
	}
	if !strings.Contains(query, "env.project_id = p.id") {
		t.Fatalf("query = %q, want environment correlated to project", query)
	}
}

func TestEscapePostgresLikeEscapesWildcards(t *testing.T) {
	got := escapePostgresLike(`prod_%\api`)
	want := `prod\_\%\\api`

	if got != want {
		t.Fatalf("escapePostgresLike() = %q, want %q", got, want)
	}
}

func TestListProjectsReturnsProjectsWithLatestEnvironments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	createdAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	latestAt := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "created_at", "environment_count", "latest_version", "latest_updated_at"}).
			AddRow("11111111-1111-1111-1111-111111111111", "App", "app", createdAt, 1, 3, latestAt))
	mock.ExpectQuery(`SELECT user_id FROM projects WHERE id = \$1`).
		WithArgs("11111111-1111-1111-1111-111111111111").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("user-1"))
	mock.ExpectQuery("SELECT project_id, environment_name, version, checksum, created_at").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"project_id", "environment_name", "version", "checksum", "created_at"}).
			AddRow("11111111-1111-1111-1111-111111111111", "production", 3, "checksum-production", latestAt))

	router := newProjectListTestRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("List returned %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response struct {
		Projects []projectSummary `json:"projects"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(response.Projects) != 1 {
		t.Fatalf("projects = %+v, want one project", response.Projects)
	}
	project := response.Projects[0]
	if project.Slug != "app" || project.AccessLevel != "owner" || len(project.Environments) != 1 {
		t.Fatalf("project = %+v, want owner app with one environment", project)
	}
	if project.Environments[0].Name != "production" || project.Environments[0].LatestVersion != 3 {
		t.Fatalf("environment = %+v, want production v3", project.Environments[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestListProjectsReturnsEmptyProjects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "created_at", "environment_count", "latest_version", "latest_updated_at"}))

	router := newProjectListTestRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("List returned %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"projects":[]`) {
		t.Fatalf("List body = %q, want empty projects array", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestListProjectsLogsInitialQueryFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT").
		WithArgs("user-1").
		WillReturnError(errors.New(`pq: relation "project_members" does not exist`))

	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)

	router := newProjectListTestRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("List returned %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "failed to list projects") {
		t.Fatalf("List body = %q, want generic list error", rec.Body.String())
	}
	if !strings.Contains(logs.String(), "project_members") {
		t.Fatalf("logs = %q, want underlying database error", logs.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestListProjectsReturnsServerErrorWhenLatestEnvironmentsFail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	createdAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "created_at", "environment_count", "latest_version", "latest_updated_at"}).
			AddRow("11111111-1111-1111-1111-111111111111", "App", "app", createdAt, 0, sql.NullInt64{}, sql.NullTime{}))
	mock.ExpectQuery(`SELECT user_id FROM projects WHERE id = \$1`).
		WithArgs("11111111-1111-1111-1111-111111111111").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("user-1"))
	mock.ExpectQuery("SELECT project_id, environment_name, version, checksum, created_at").
		WithArgs(sqlmock.AnyArg()).
		WillReturnError(errors.New(`pq: relation "env_versions" does not exist`))

	router := newProjectListTestRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("List returned %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "failed to list environments") {
		t.Fatalf("List body = %q, want generic environments error", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestRenameProjectRejectsInvalidName(t *testing.T) {
	router := newRenameTestRouter()

	for name, body := range map[string]string{
		"missing":    `{}`,
		"empty":      `{"name":""}`,
		"whitespace": `{"name":"   "}`,
	} {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/my-app", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("Rename (%s) returned %d, want %d", name, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestRenameProjectRejectsOverlongName(t *testing.T) {
	router := newRenameTestRouter()

	body := `{"name":"` + strings.Repeat("a", 101) + `"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/my-app", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Rename returned %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRenameProjectRequiresAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewProjectHandler(nil)
	router.PATCH("/api/v1/projects/:slug", handler.Rename)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/my-app", strings.NewReader(`{"name":"My App"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("Rename returned %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func newRenameTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewProjectHandler(nil)
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Next()
	})
	router.PATCH("/api/v1/projects/:slug", handler.Rename)
	return router
}

func newProjectListTestRouter(db *sql.DB) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Next()
	})
	router.GET("/api/v1/projects", NewProjectHandler(db).List)
	return router
}
