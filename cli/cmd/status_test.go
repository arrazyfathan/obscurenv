package cmd

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestStatusLegacyConfigShowsCollaboratorRole(t *testing.T) {
	out, err := runStatusWithHandler(t, ProjectConfig{
		ProjectSlug:       "transfer-project",
		ActiveEnvironment: "development",
	}, statusHandler("collaborator"))
	if err != nil {
		t.Fatalf("obe status: %v", err)
	}
	for _, want := range []string{"linked", "Role", "collaborator", "legacy config"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "access lost") {
		t.Fatalf("output should not report lost access:\n%s", out)
	}
}

func TestStatusLegacyConfigShowsOwnerRole(t *testing.T) {
	out, err := runStatusWithHandler(t, ProjectConfig{
		ProjectSlug:       "transfer-project",
		ActiveEnvironment: "development",
	}, statusHandler("owner"))
	if err != nil {
		t.Fatalf("obe status: %v", err)
	}
	if !strings.Contains(out, "owner") {
		t.Fatalf("output missing owner role:\n%s", out)
	}
	if strings.Contains(out, "collaborator") {
		t.Fatalf("output should not report collaborator role:\n%s", out)
	}
}

func TestStatusNonLegacyConfigShowsRoleRow(t *testing.T) {
	out, err := runStatusWithHandler(t, ProjectConfig{
		ProjectID:         "project-1",
		ProjectSlug:       "transfer-project",
		ActiveEnvironment: "development",
	}, statusHandler("collaborator"))
	if err != nil {
		t.Fatalf("obe status: %v", err)
	}
	if !strings.Contains(out, "Role") || !strings.Contains(out, "collaborator") {
		t.Fatalf("output missing role row:\n%s", out)
	}
	if strings.Contains(out, "legacy config") {
		t.Fatalf("non-legacy config should not show the legacy upgrade note:\n%s", out)
	}
}

func TestStatusLegacyConfigAccessLost(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	out, err := runStatusWithHandler(t, ProjectConfig{
		ProjectSlug:       "transfer-project",
		ActiveEnvironment: "development",
	}, handler)
	if err == nil {
		t.Fatal("expected access-lost error")
	}
	if !strings.Contains(err.Error(), "no longer accessible") {
		t.Fatalf("error = %q, want access-lost error", err)
	}
	if !strings.Contains(out, "access lost; run obe init to relink") {
		t.Fatalf("output missing access-lost message:\n%s", out)
	}
}

func statusHandler(accessLevel string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/projects/"):
			_, _ = io.WriteString(w, `{"id":"project-1","name":"Transfer Project","slug":"transfer-project","access_level":"`+accessLevel+`"}`)
		case r.URL.Path == "/api/v1/env/list":
			_, _ = io.WriteString(w, `{"environments":["development","production"]}`)
		default:
			http.NotFound(w, r)
		}
	})
}

func runStatusWithHandler(t *testing.T, config ProjectConfig, handler http.Handler) (string, error) {
	t.Helper()

	withTempWorkingDir(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := saveProjectConfig(config); err != nil {
		t.Fatalf("saveProjectConfig: %v", err)
	}
	if err := saveCredentials(Credentials{Token: "test-token"}); err != nil {
		t.Fatalf("saveCredentials: %v", err)
	}
	t.Cleanup(stubAPIClient(handler))

	var out bytes.Buffer
	statusCmd.SetOut(&out)
	t.Cleanup(func() { statusCmd.SetOut(nil) })

	err := statusCmd.RunE(statusCmd, nil)
	return out.String(), err
}
