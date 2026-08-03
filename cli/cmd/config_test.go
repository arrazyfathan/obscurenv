package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProjectConfigMissingSuggestsInit(t *testing.T) {
	withTempWorkingDir(t)

	_, err := loadProjectConfig()
	if err == nil {
		t.Fatal("expected missing project config error")
	}
	if !strings.Contains(err.Error(), "run: obe init") {
		t.Fatalf("error = %q, want init suggestion", err)
	}
}

func TestLoadCredentialsMissingSuggestsLogin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, err := loadCredentials()
	if err == nil {
		t.Fatal("expected missing credentials error")
	}
	if !strings.Contains(err.Error(), "run: obe login") {
		t.Fatalf("error = %q, want login suggestion", err)
	}
}

func TestProjectConfigFileUsesOBEName(t *testing.T) {
	withTempWorkingDir(t)

	if err := saveProjectConfig(ProjectConfig{
		ProjectSlug:       "my-app",
		ActiveEnvironment: "development",
	}); err != nil {
		t.Fatalf("saveProjectConfig: %v", err)
	}

	if _, err := os.Stat(".obe.json"); err != nil {
		t.Fatalf("expected .obe.json to exist: %v", err)
	}
	if _, err := os.Stat(".obv.json"); !os.IsNotExist(err) {
		t.Fatalf("expected .obv.json not to exist, got err=%v", err)
	}
}

func TestCredentialsPathUsesOBEDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := credentialsPath()
	if err != nil {
		t.Fatalf("credentialsPath: %v", err)
	}

	want := filepath.Join(home, ".obe", "credentials.json")
	if path != want {
		t.Fatalf("credentialsPath = %q, want %q", path, want)
	}
}

func TestAPIBaseURLUsesOBEEnvVar(t *testing.T) {
	t.Setenv("OBE_API_URL", "http://obe.example.test")
	t.Setenv("OBV_API_URL", "http://obv.example.test")

	if got := apiBaseURL(); got != "http://obe.example.test" {
		t.Fatalf("apiBaseURL = %q, want OBE_API_URL", got)
	}
}

func TestAPIBaseURLIgnoresOBVEnvVar(t *testing.T) {
	t.Setenv("OBV_API_URL", "http://obv.example.test")

	if got := apiBaseURL(); got != defaultAPIURL {
		t.Fatalf("apiBaseURL = %q, want default API URL", got)
	}
}

func TestEnvUseUpdatesActiveEnvironment(t *testing.T) {
	withTempWorkingDir(t)

	if err := saveProjectConfig(ProjectConfig{
		ProjectSlug:       "my-app",
		ActiveEnvironment: "development",
	}); err != nil {
		t.Fatalf("saveProjectConfig: %v", err)
	}

	if err := envUseCmd.RunE(envUseCmd, []string{"staging"}); err != nil {
		t.Fatalf("env use: %v", err)
	}

	config, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}
	if config.ActiveEnvironment != "staging" {
		t.Fatalf("ActiveEnvironment = %q, want staging", config.ActiveEnvironment)
	}
}

func withTempWorkingDir(t *testing.T) {
	t.Helper()

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}
