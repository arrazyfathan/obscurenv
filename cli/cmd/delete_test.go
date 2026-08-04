package cmd

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestEnvDeleteRemovesRemoteEnvironment(t *testing.T) {
	withTempWorkingDir(t)
	setupDeleteCommandTest(t)

	var sawDelete bool
	restore := stubAPIClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/env" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("project") != "obsecurenv" || r.URL.Query().Get("environment") != "staging" {
			http.Error(w, `{"error":"bad delete query"}`, http.StatusBadRequest)
			return
		}
		sawDelete = true
		_, _ = w.Write([]byte(`{"message":"environment deleted"}`))
	}))
	t.Cleanup(restore)

	envDeleteYes = true
	var out bytes.Buffer
	envDeleteCmd.SetOut(&out)

	if err := envDeleteCmd.RunE(envDeleteCmd, []string{"staging"}); err != nil {
		t.Fatalf("env rm: %v", err)
	}
	if !sawDelete {
		t.Fatal("expected env rm to call delete API")
	}
	if !strings.Contains(out.String(), `Deleted remote environment "staging"`) {
		t.Fatalf("output = %q, want delete confirmation", out.String())
	}
}

func TestProjectDeleteRemovesRemoteProject(t *testing.T) {
	withTempWorkingDir(t)
	setupDeleteCommandTest(t)

	var sawDelete bool
	restore := stubAPIClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodDelete || r.URL.EscapedPath() != "/api/v1/projects/obsecurenv" {
			http.NotFound(w, r)
			return
		}
		sawDelete = true
		_, _ = w.Write([]byte(`{"message":"project deleted"}`))
	}))
	t.Cleanup(restore)

	projectDeleteYes = true
	var out bytes.Buffer
	projectDeleteCmd.SetOut(&out)

	if err := projectDeleteCmd.RunE(projectDeleteCmd, nil); err != nil {
		t.Fatalf("project rm: %v", err)
	}
	if !sawDelete {
		t.Fatal("expected project rm to call delete API")
	}
	if !strings.Contains(out.String(), `Deleted remote project "obsecurenv"`) {
		t.Fatalf("output = %q, want delete confirmation", out.String())
	}
}

func TestDeleteRequiresConfirmationInNonInteractiveMode(t *testing.T) {
	err := confirmRemoteDeletion(false, `Delete remote project "obsecurenv"?`)
	if err == nil {
		t.Fatal("expected delete without --yes to be canceled in non-interactive mode")
	}
	if !strings.Contains(err.Error(), "pass --yes") {
		t.Fatalf("error = %q, want --yes hint", err)
	}
}

func setupDeleteCommandTest(t *testing.T) {
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

	oldEnvDeleteYes := envDeleteYes
	oldProjectDeleteYes := projectDeleteYes
	t.Cleanup(func() {
		envDeleteYes = oldEnvDeleteYes
		projectDeleteYes = oldProjectDeleteYes
		envDeleteCmd.SetOut(nil)
		projectDeleteCmd.SetOut(nil)
	})
	envDeleteYes = false
	projectDeleteYes = false
}
