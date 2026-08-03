package cmd

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	obecrypto "github.com/obscurenv/obscurenv/cli/pkg/crypto"
)

func TestResolveManagedFileAutoDetectsDotenvOnly(t *testing.T) {
	withTempWorkingDir(t)
	writeTestFile(t, defaultEnvFile, "SECRET=value\n")

	got, err := resolveManagedFile("", nil, true)
	if err != nil {
		t.Fatalf("resolveManagedFile: %v", err)
	}
	if got != defaultEnvFile {
		t.Fatalf("file = %q, want %q", got, defaultEnvFile)
	}
}

func TestResolveManagedFileAutoDetectsLocalPropertiesOnly(t *testing.T) {
	withTempWorkingDir(t)
	writeTestFile(t, gradleEnvFile, "sdk.dir=/opt/android\n")

	got, err := resolveManagedFile("", nil, true)
	if err != nil {
		t.Fatalf("resolveManagedFile: %v", err)
	}
	if got != gradleEnvFile {
		t.Fatalf("file = %q, want %q", got, gradleEnvFile)
	}
}

func TestResolveManagedFileRejectsAmbiguousFiles(t *testing.T) {
	withTempWorkingDir(t)
	writeTestFile(t, defaultEnvFile, "SECRET=value\n")
	writeTestFile(t, gradleEnvFile, "sdk.dir=/opt/android\n")

	_, err := resolveManagedFile("", nil, true)
	if err == nil {
		t.Fatal("expected ambiguous file error")
	}
	if !strings.Contains(err.Error(), "both .env and local.properties exist") {
		t.Fatalf("error = %q, want ambiguous file message", err)
	}
}

func TestResolveManagedFileRequiresExistingForPush(t *testing.T) {
	withTempWorkingDir(t)

	_, err := resolveManagedFile("", nil, true)
	if err == nil {
		t.Fatal("expected missing managed file error")
	}
	if !strings.Contains(err.Error(), "no managed file found") {
		t.Fatalf("error = %q, want missing file message", err)
	}
}

func TestResolveManagedFileDefaultsDotenvWhenNotRequired(t *testing.T) {
	withTempWorkingDir(t)

	got, err := resolveManagedFile("", nil, false)
	if err != nil {
		t.Fatalf("resolveManagedFile: %v", err)
	}
	if got != defaultEnvFile {
		t.Fatalf("file = %q, want %q", got, defaultEnvFile)
	}
}

func TestResolveManagedFileUsesConfigAndFlagPrecedence(t *testing.T) {
	withTempWorkingDir(t)
	config := &ProjectConfig{EnvFile: gradleEnvFile}

	got, err := resolveManagedFile("", config, true)
	if err != nil {
		t.Fatalf("resolveManagedFile config: %v", err)
	}
	if got != gradleEnvFile {
		t.Fatalf("config file = %q, want %q", got, gradleEnvFile)
	}

	got, err = resolveManagedFile(defaultEnvFile, config, true)
	if err != nil {
		t.Fatalf("resolveManagedFile flag: %v", err)
	}
	if got != defaultEnvFile {
		t.Fatalf("flag file = %q, want %q", got, defaultEnvFile)
	}
}

func TestValidateManagedFileRejectsUnsafePaths(t *testing.T) {
	for _, path := range []string{"", "/tmp/.env", "../.env", "config/../.env"} {
		if _, err := validateManagedFile(path); err == nil {
			t.Fatalf("validateManagedFile(%q) succeeded, want error", path)
		}
	}
}

func TestPushAutoDetectsLocalProperties(t *testing.T) {
	withTempWorkingDir(t)
	setupFileCommandTest(t, ProjectConfig{
		ProjectSlug:       "obsecurenv",
		ActiveEnvironment: "development",
	})
	writeTestFile(t, gradleEnvFile, "sdk.dir=/opt/android\nAPI_KEY=android-secret\n")

	var gotPlaintext string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/env/push" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			ProjectSlug      string `json:"project_slug"`
			Environment      string `json:"environment"`
			EncryptedPayload string `json:"encrypted_payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode push request: %v", err)
		}
		plaintext, err := obecrypto.DecryptWithPassphrase(req.EncryptedPayload, "passphrase")
		if err != nil {
			t.Fatalf("decrypt pushed payload: %v", err)
		}
		gotPlaintext = string(plaintext)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"Pushed successfully","version":2}`))
	})
	restore := stubAPIClient(handler)
	t.Cleanup(restore)

	version, err := pushEnvironment("development", "passphrase", gradleEnvFile)
	if err != nil {
		t.Fatalf("pushEnvironment: %v", err)
	}
	if version != 2 {
		t.Fatalf("version = %d, want 2", version)
	}
	if gotPlaintext != "sdk.dir=/opt/android\nAPI_KEY=android-secret\n" {
		t.Fatalf("pushed plaintext = %q, want local.properties content", gotPlaintext)
	}
	config, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}
	if config.EnvFile != gradleEnvFile {
		t.Fatalf("EnvFile = %q, want %q", config.EnvFile, gradleEnvFile)
	}
}

func TestPushCommandRemembersLatestPushedFileArgument(t *testing.T) {
	withTempWorkingDir(t)
	setupFileCommandTest(t, ProjectConfig{
		ProjectSlug:       "obsecurenv",
		ActiveEnvironment: "development",
	})
	writeTestFile(t, defaultEnvFile, "SECRET=dotenv\n")
	writeTestFile(t, gradleEnvFile, "sdk.dir=/opt/android\n")

	restore := stubAPIClient(pushSuccessHandler())
	t.Cleanup(restore)
	oldKey := pushKey
	oldEnv := pushEnv
	oldFile := pushFile
	t.Cleanup(func() {
		pushKey = oldKey
		pushEnv = oldEnv
		pushFile = oldFile
	})
	pushKey = "passphrase"
	pushEnv = ""
	pushFile = ""

	if err := pushCmd.RunE(pushCmd, []string{gradleEnvFile}); err != nil {
		t.Fatalf("push local.properties: %v", err)
	}
	config, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}
	if config.EnvFile != gradleEnvFile {
		t.Fatalf("EnvFile = %q, want %q", config.EnvFile, gradleEnvFile)
	}

	if err := pushCmd.RunE(pushCmd, []string{defaultEnvFile}); err != nil {
		t.Fatalf("push .env: %v", err)
	}
	config, err = loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}
	if config.EnvFile != defaultEnvFile {
		t.Fatalf("EnvFile = %q, want %q", config.EnvFile, defaultEnvFile)
	}
}

func TestPushCommandAutoDetectsLocalPropertiesOverStaleConfig(t *testing.T) {
	withTempWorkingDir(t)
	setupFileCommandTest(t, ProjectConfig{
		ProjectSlug:       "obsecurenv",
		ActiveEnvironment: "development",
		EnvFile:           defaultEnvFile,
	})
	writeTestFile(t, gradleEnvFile, "sdk.dir=/opt/android\n")

	restore := stubAPIClient(pushSuccessHandler())
	t.Cleanup(restore)
	oldKey := pushKey
	oldEnv := pushEnv
	oldFile := pushFile
	t.Cleanup(func() {
		pushKey = oldKey
		pushEnv = oldEnv
		pushFile = oldFile
	})
	pushKey = "passphrase"
	pushEnv = ""
	pushFile = ""

	if err := pushCmd.RunE(pushCmd, nil); err != nil {
		t.Fatalf("push auto-detected local.properties: %v", err)
	}
	config, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}
	if config.EnvFile != gradleEnvFile {
		t.Fatalf("EnvFile = %q, want %q", config.EnvFile, gradleEnvFile)
	}
}

func TestPushCommandAutoDetectsDotenvOverStaleConfig(t *testing.T) {
	withTempWorkingDir(t)
	setupFileCommandTest(t, ProjectConfig{
		ProjectSlug:       "obsecurenv",
		ActiveEnvironment: "development",
		EnvFile:           gradleEnvFile,
	})
	writeTestFile(t, defaultEnvFile, "SECRET=dotenv\n")

	restore := stubAPIClient(pushSuccessHandler())
	t.Cleanup(restore)
	oldKey := pushKey
	oldEnv := pushEnv
	oldFile := pushFile
	t.Cleanup(func() {
		pushKey = oldKey
		pushEnv = oldEnv
		pushFile = oldFile
	})
	pushKey = "passphrase"
	pushEnv = ""
	pushFile = ""

	if err := pushCmd.RunE(pushCmd, nil); err != nil {
		t.Fatalf("push auto-detected .env: %v", err)
	}
	config, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}
	if config.EnvFile != defaultEnvFile {
		t.Fatalf("EnvFile = %q, want %q", config.EnvFile, defaultEnvFile)
	}
}

func TestPullWritesResolvedLocalProperties(t *testing.T) {
	withTempWorkingDir(t)
	setupFileCommandTest(t, ProjectConfig{
		ProjectSlug:       "obsecurenv",
		ActiveEnvironment: "development",
		EnvFile:           gradleEnvFile,
	})

	payload, err := obecrypto.EncryptWithPassphrase([]byte("sdk.dir=/opt/android\n"), "passphrase")
	if err != nil {
		t.Fatalf("EncryptWithPassphrase: %v", err)
	}
	restore := stubAPIClient(pullPayloadHandler(t, payload))
	t.Cleanup(restore)

	if _, err := pullEnvironment("development", "passphrase", gradleEnvFile, true); err != nil {
		t.Fatalf("pullEnvironment: %v", err)
	}
	data, err := os.ReadFile(gradleEnvFile)
	if err != nil {
		t.Fatalf("read local.properties: %v", err)
	}
	if string(data) != "sdk.dir=/opt/android\n" {
		t.Fatalf("local.properties = %q, want pulled content", data)
	}
}

func TestPullRemembersExplicitLocalProperties(t *testing.T) {
	withTempWorkingDir(t)
	setupFileCommandTest(t, ProjectConfig{
		ProjectSlug:       "obsecurenv",
		ActiveEnvironment: "development",
	})

	payload, err := obecrypto.EncryptWithPassphrase([]byte("sdk.dir=/opt/android\n"), "passphrase")
	if err != nil {
		t.Fatalf("EncryptWithPassphrase: %v", err)
	}
	restore := stubAPIClient(pullPayloadHandler(t, payload))
	t.Cleanup(restore)

	if _, err := pullEnvironment("development", "passphrase", gradleEnvFile, true); err != nil {
		t.Fatalf("pullEnvironment: %v", err)
	}
	config, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}
	if config.EnvFile != gradleEnvFile {
		t.Fatalf("EnvFile = %q, want %q", config.EnvFile, gradleEnvFile)
	}
}

func TestPullDecryptFailureLeavesResolvedFileUnchanged(t *testing.T) {
	withTempWorkingDir(t)
	setupFileCommandTest(t, ProjectConfig{
		ProjectSlug:       "obsecurenv",
		ActiveEnvironment: "development",
		EnvFile:           gradleEnvFile,
	})
	writeTestFile(t, gradleEnvFile, "sdk.dir=/existing\n")

	payload, err := obecrypto.EncryptWithPassphrase([]byte("sdk.dir=/new\n"), "correct-passphrase")
	if err != nil {
		t.Fatalf("EncryptWithPassphrase: %v", err)
	}
	restore := stubAPIClient(pullPayloadHandler(t, payload))
	t.Cleanup(restore)

	_, err = pullEnvironment("development", "wrong-passphrase", gradleEnvFile, true)
	if err == nil {
		t.Fatal("expected decrypt failure")
	}
	data, err := os.ReadFile(gradleEnvFile)
	if err != nil {
		t.Fatalf("read local.properties: %v", err)
	}
	if string(data) != "sdk.dir=/existing\n" {
		t.Fatalf("local.properties = %q, want unchanged content", data)
	}
}

func setupFileCommandTest(t *testing.T, config ProjectConfig) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OBE_API_URL", "")
	if err := saveCredentials(Credentials{Token: "test-token", APIURL: "http://obe.test"}); err != nil {
		t.Fatalf("saveCredentials: %v", err)
	}
	if err := saveProjectConfig(config); err != nil {
		t.Fatalf("saveProjectConfig: %v", err)
	}
}

func pushSuccessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/env/push" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"Pushed successfully","version":2}`))
	})
}

func pullPayloadHandler(t *testing.T, payload string) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/env/pull" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"project_slug":      "obsecurenv",
			"environment":       "development",
			"version":           1,
			"encrypted_payload": payload,
			"checksum":          "checksum",
		})
	})
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
