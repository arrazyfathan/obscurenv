package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestListActivityQueryDefaultsToListAllUserActivity(t *testing.T) {
	query, args := listActivityQuery("user-1", activityFilter{Limit: 50, Offset: 0}, 50)

	if !strings.Contains(query, "WHERE al.user_id = $1") {
		t.Fatalf("query = %q, want user scope", query)
	}
	if !strings.Contains(query, "LIMIT $2 OFFSET $3") {
		t.Fatalf("query = %q, want LIMIT $2 OFFSET $3", query)
	}
	if !strings.Contains(query, "ORDER BY al.created_at DESC, al.id DESC") {
		t.Fatalf("query = %q, want newest-first ordering", query)
	}
	if len(args) != 3 || args[0] != "user-1" || args[1] != 50 || args[2] != 0 {
		t.Fatalf("args = %#v, want [user-1 50 0]", args)
	}
}

func TestListActivityQueryWithProjectFilter(t *testing.T) {
	query, args := listActivityQuery("user-1", activityFilter{ProjectSlug: "app", Limit: 50, Offset: 0}, 50)

	if !strings.Contains(query, "AND al.project_slug = $2") {
		t.Fatalf("query = %q, want project slug filter", query)
	}
	if !strings.Contains(query, "LIMIT $3 OFFSET $4") {
		t.Fatalf("query = %q, want LIMIT $3 OFFSET $4", query)
	}
	if len(args) != 4 || args[1] != "app" {
		t.Fatalf("args = %#v, want project slug as second arg", args)
	}
}

func TestListActivityQueryWithActionFilter(t *testing.T) {
	query, args := listActivityQuery("user-1", activityFilter{Action: ActionEnvPushed, Limit: 50, Offset: 0}, 50)

	if !strings.Contains(query, "AND al.action = $2") {
		t.Fatalf("query = %q, want action filter", query)
	}
	if len(args) != 4 || args[1] != ActionEnvPushed {
		t.Fatalf("args = %#v, want action as second arg", args)
	}
}

func TestListActivityQueryWithProjectAndActionFilters(t *testing.T) {
	query, args := listActivityQuery("user-1", activityFilter{ProjectSlug: "app", Action: ActionEnvDeleted, Limit: 10, Offset: 5}, 10)

	if !strings.Contains(query, "AND al.project_slug = $2") || !strings.Contains(query, "AND al.action = $3") {
		t.Fatalf("query = %q, want project and action filters", query)
	}
	if !strings.Contains(query, "LIMIT $4 OFFSET $5") {
		t.Fatalf("query = %q, want LIMIT $4 OFFSET $5", query)
	}
	if len(args) != 5 || args[1] != "app" || args[2] != ActionEnvDeleted || args[3] != 10 || args[4] != 5 {
		t.Fatalf("args = %#v, want [user-1 app env.deleted 10 5]", args)
	}
}

func TestCountActivityQueryScopesByUserAndFilters(t *testing.T) {
	query, args := countActivityQuery("user-1", activityFilter{ProjectSlug: "app", Action: ActionEnvPushed})

	if !strings.Contains(query, "SELECT COUNT(*)") {
		t.Fatalf("query = %q, want COUNT query", query)
	}
	if strings.Contains(query, "LIMIT") || strings.Contains(query, "OFFSET") {
		t.Fatalf("query = %q, count query must not paginate", query)
	}
	if !strings.Contains(query, "AND al.project_slug = $2") || !strings.Contains(query, "AND al.action = $3") {
		t.Fatalf("query = %q, want project and action filters", query)
	}
	if len(args) != 3 {
		t.Fatalf("args = %#v, want 3 args", args)
	}
}

func TestActivityListRejectsInvalidLimit(t *testing.T) {
	router := newActivityTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity?limit=0", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Activity returned %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestActivityListRejectsInvalidOffset(t *testing.T) {
	router := newActivityTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity?offset=-1", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Activity returned %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestActivityListRejectsUnknownAction(t *testing.T) {
	router := newActivityTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity?action=bogus", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Activity returned %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestActivityListReturnsScopedActivity(t *testing.T) {
	router, handler := newActivityTestRouterWithHandler()
	handler.list = func(_ context.Context, userID string, filter activityFilter) ([]activityItem, int, string, error) {
		if userID != "user-1" {
			t.Fatalf("userID = %q, want user-1", userID)
		}
		if filter.ProjectSlug != "app" || filter.Action != ActionEnvPushed || filter.Limit != 10 || filter.Offset != 0 {
			t.Fatalf("filter = %#v, want parsed query filters", filter)
		}
		slug := "app"
		env := "production"
		return []activityItem{
			{ID: "act-1", Action: ActionEnvPushed, ProjectSlug: &slug, EnvironmentName: &env, CreatedAt: "2026-08-06T10:00:00Z"},
		}, 1, "", nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity?limit=10&project=app&action=env.pushed", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Activity returned %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"total":1`, `"action":"env.pushed"`, `"project_slug":"app"`, `"environment_name":"production"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("Activity body = %q, want %s", body, want)
		}
	}
}

func TestActivityListReturnsErrorOnLookupFailure(t *testing.T) {
	router, handler := newActivityTestRouterWithHandler()
	handler.list = func(context.Context, string, activityFilter) ([]activityItem, int, string, error) {
		return nil, 0, "", context.DeadlineExceeded
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("Activity returned %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestListActivityQueryWithDateRangeFilter(t *testing.T) {
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	query, args := listActivityQuery("user-1", activityFilter{Limit: 10, Offset: 0, From: from, To: to, HasFrom: true, HasTo: true}, 10)

	if !strings.Contains(query, "AND al.created_at >= $2") || !strings.Contains(query, "AND al.created_at <= $3") {
		t.Fatalf("query = %q, want date range filters", query)
	}
	if len(args) != 5 || args[1] != from || args[2] != to {
		t.Fatalf("args = %#v, want [user-1 from to 10 0]", args)
	}
}

func TestListActivityQueryWithCursor(t *testing.T) {
	before := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	query, args := listActivityQuery("user-1", activityFilter{Limit: 10, Offset: 0, BeforeID: "act-9", BeforeCreatedAt: before, HasBefore: true}, 11)

	if !strings.Contains(query, "al.created_at < $2") || !strings.Contains(query, "AND al.id < $3") {
		t.Fatalf("query = %q, want keyset cursor condition", query)
	}
	if !strings.Contains(query, "LIMIT $4 OFFSET $5") {
		t.Fatalf("query = %q, want LIMIT $4 OFFSET $5", query)
	}
	if len(args) != 5 || args[1] != before || args[2] != "act-9" || args[3] != 11 {
		t.Fatalf("args = %#v, want [user-1 before act-9 11 0]", args)
	}
}

func TestActivityCursorRoundTrip(t *testing.T) {
	created := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	encoded := encodeActivityCursor(created, "act-9")

	cursor, err := decodeActivityCursor(encoded)
	if err != nil {
		t.Fatalf("decodeActivityCursor: %v", err)
	}
	if !cursor.CreatedAt.Equal(created) || cursor.ID != "act-9" {
		t.Fatalf("cursor = %#v, want round-tripped values", cursor)
	}
}

func TestActivityListRejectsInvalidCursor(t *testing.T) {
	router := newActivityTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity?cursor=not-base64", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Activity returned %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestActivityListRejectsInvalidDateRange(t *testing.T) {
	router := newActivityTestRouter()

	for _, query := range []string{"from=2026-13-99", "to=yesterday"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/activity?"+query, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("Activity (%s) returned %d, want %d", query, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestActivityListParsesCursorAndDateFilters(t *testing.T) {
	router, handler := newActivityTestRouterWithHandler()
	before := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	encoded := encodeActivityCursor(before, "act-9")
	handler.list = func(_ context.Context, userID string, filter activityFilter) ([]activityItem, int, string, error) {
		if userID != "user-1" {
			t.Fatalf("userID = %q, want user-1", userID)
		}
		if !filter.HasBefore || filter.BeforeID != "act-9" || !filter.BeforeCreatedAt.Equal(before) {
			t.Fatalf("filter cursor = %#v, want parsed cursor", filter)
		}
		if !filter.HasFrom || !filter.From.Equal(from) {
			t.Fatalf("filter.From = %v, want parsed from", filter.From)
		}
		return []activityItem{
			{ID: "act-1", Action: ActionEnvPushed, CreatedAt: "2026-08-06T09:00:00Z"},
		}, 1, "next-cursor-value", nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity?cursor="+encoded+"&from=2026-08-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Activity returned %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"next_cursor":"next-cursor-value"`) {
		t.Fatalf("Activity body = %q, want next_cursor", rec.Body.String())
	}
}

func newActivityTestRouter() *gin.Engine {
	router, _ := newActivityTestRouterWithHandler()
	return router
}

func newActivityTestRouterWithHandler() (*gin.Engine, *ActivityHandler) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewActivityHandler(nil)
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Next()
	})
	router.GET("/api/v1/activity", handler.List)
	return router, handler
}
