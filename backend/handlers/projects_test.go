package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
