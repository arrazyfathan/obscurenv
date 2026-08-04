package handlers

import (
	"strings"
	"testing"
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

func TestEscapePostgresLikeEscapesWildcards(t *testing.T) {
	got := escapePostgresLike(`prod_%\api`)
	want := `prod\_\%\\api`

	if got != want {
		t.Fatalf("escapePostgresLike() = %q, want %q", got, want)
	}
}
