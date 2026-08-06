package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestProjectRenameUpdatesCurrentProjectName(t *testing.T) {
	withTempWorkingDir(t)
	setupRenameCommandTest(t)

	var sawPatch bool
	var gotBody map[string]string
	restore := stubAPIClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPatch || r.URL.EscapedPath() != "/api/v1/projects/obsecurenv" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		sawPatch = true
		_, _ = w.Write([]byte(`{"id":"id-1","name":"New Name","slug":"obsecurenv"}`))
	}))
	t.Cleanup(restore)

	var out bytes.Buffer
	projectRenameCmd.SetOut(&out)

	if err := projectRenameCmd.RunE(projectRenameCmd, []string{"New Name"}); err != nil {
		t.Fatalf("project rename: %v", err)
	}
	if !sawPatch {
		t.Fatal("expected project rename to call update API")
	}
	if gotBody["name"] != "New Name" {
		t.Fatalf("request name = %q, want New Name", gotBody["name"])
	}
	if !strings.Contains(out.String(), `Renamed project "obsecurenv" to "New Name".`) {
		t.Fatalf("output = %q, want rename confirmation", out.String())
	}
}

func TestProjectRenameUsesExplicitSlug(t *testing.T) {
	withTempWorkingDir(t)
	setupRenameCommandTest(t)

	var sawPatch bool
	restore := stubAPIClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.EscapedPath() != "/api/v1/projects/other-app" {
			http.NotFound(w, r)
			return
		}
		sawPatch = true
		_, _ = w.Write([]byte(`{"id":"id-1","name":"Other","slug":"other-app"}`))
	}))
	t.Cleanup(restore)

	var out bytes.Buffer
	projectRenameCmd.SetOut(&out)

	if err := projectRenameCmd.RunE(projectRenameCmd, []string{"Other", "other-app"}); err != nil {
		t.Fatalf("project rename: %v", err)
	}
	if !sawPatch {
		t.Fatal("expected project rename to use the explicit slug")
	}
}

func TestProjectRenameRejectsEmptyName(t *testing.T) {
	withTempWorkingDir(t)
	setupRenameCommandTest(t)

	var out bytes.Buffer
	projectRenameCmd.SetOut(&out)

	err := projectRenameCmd.RunE(projectRenameCmd, []string{"   "})
	if err == nil {
		t.Fatal("expected project rename to reject empty name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("error = %q, want name required", err)
	}
}

func setupRenameCommandTest(t *testing.T) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OBE_API_URL", "")
	if err := saveCredentials(Credentials{Token: "test-token", APIURL: "http://obe.test"}); err != nil {
		t.Fatalf("saveCredentials: %v", err)
	}
	if err := saveProjectConfig(ProjectConfig{
		ProjectSlug:       "obsecurenv",
		ActiveEnvironment: "development",
	}); err != nil {
		t.Fatalf("saveProjectConfig: %v", err)
	}

	t.Cleanup(func() {
		projectRenameCmd.SetOut(nil)
	})
}
