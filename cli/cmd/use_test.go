package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/obscurenv/obscurenv/cli/pkg/api"
	obecrypto "github.com/obscurenv/obscurenv/cli/pkg/crypto"
)

func TestUsePullsTargetWithoutPushingCurrent(t *testing.T) {
	withTempWorkingDir(t)
	setupUseCommandTest(t, false)

	var sawPush bool
	var sawPull bool
	handler := useHandler(t, &sawPush, &sawPull)
	restore := stubAPIClient(handler)
	t.Cleanup(restore)

	if err := useCmd.RunE(useCmd, []string{"production"}); err != nil {
		t.Fatalf("use: %v", err)
	}
	if sawPush {
		t.Fatal("use pushed current environment without --push-current")
	}
	if !sawPull {
		t.Fatal("use did not pull target environment")
	}

	config, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}
	if config.ActiveEnvironment != "production" {
		t.Fatalf("ActiveEnvironment = %q, want production", config.ActiveEnvironment)
	}
	data, err := os.ReadFile(".env")
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if string(data) != "TARGET=value\n" {
		t.Fatalf(".env = %q, want target payload", data)
	}
}

func TestUsePushCurrentFlagPreservesOldPushBehavior(t *testing.T) {
	withTempWorkingDir(t)
	setupUseCommandTest(t, true)

	var sawPush bool
	var sawPull bool
	handler := useHandler(t, &sawPush, &sawPull)
	restore := stubAPIClient(handler)
	t.Cleanup(restore)

	if err := useCmd.RunE(useCmd, []string{"production"}); err != nil {
		t.Fatalf("use: %v", err)
	}
	if !sawPush {
		t.Fatal("use --push-current did not push current environment")
	}
	if !sawPull {
		t.Fatal("use --push-current did not pull target environment")
	}
}

func TestUsePushCurrentUsesConfiguredLocalProperties(t *testing.T) {
	withTempWorkingDir(t)
	setupUseCommandTest(t, true)

	config, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}
	config.EnvFile = gradleEnvFile
	if err := saveProjectConfig(*config); err != nil {
		t.Fatalf("saveProjectConfig: %v", err)
	}
	if err := os.Remove(defaultEnvFile); err != nil {
		t.Fatalf("remove .env: %v", err)
	}
	if err := os.WriteFile(gradleEnvFile, []byte("sdk.dir=/existing\n"), 0600); err != nil {
		t.Fatalf("write local.properties: %v", err)
	}

	targetPayload, err := obecrypto.EncryptWithPassphrase([]byte("sdk.dir=/target\n"), "passphrase")
	if err != nil {
		t.Fatalf("EncryptWithPassphrase: %v", err)
	}
	var pushedPlaintext string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/env/push":
			var req struct {
				EncryptedPayload string `json:"encrypted_payload"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode push request: %v", err)
			}
			plaintext, err := obecrypto.DecryptWithPassphrase(req.EncryptedPayload, "passphrase")
			if err != nil {
				t.Fatalf("decrypt push payload: %v", err)
			}
			pushedPlaintext = string(plaintext)
			_, _ = w.Write([]byte(`{"message":"Pushed successfully","version":2}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/env/pull":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"project_slug":      "obsecurenv",
				"environment":       "production",
				"version":           1,
				"encrypted_payload": targetPayload,
				"checksum":          "checksum",
			})
		default:
			http.NotFound(w, r)
		}
	})
	restore := stubAPIClient(handler)
	t.Cleanup(restore)

	if err := useCmd.RunE(useCmd, []string{"production"}); err != nil {
		t.Fatalf("use: %v", err)
	}
	if pushedPlaintext != "sdk.dir=/existing\n" {
		t.Fatalf("pushed plaintext = %q, want local.properties content", pushedPlaintext)
	}
	data, err := os.ReadFile(gradleEnvFile)
	if err != nil {
		t.Fatalf("read local.properties: %v", err)
	}
	if string(data) != "sdk.dir=/target\n" {
		t.Fatalf("local.properties = %q, want target payload", data)
	}
}

func TestUseWithoutArgumentListsRemoteEnvironments(t *testing.T) {
	withTempWorkingDir(t)
	setupUseCommandTest(t, false)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/env/list" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("project") != "obsecurenv" {
			http.Error(w, `{"error":"bad project"}`, http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"environments":["development","production","staging"]}`))
	})
	restore := stubAPIClient(handler)
	t.Cleanup(restore)
	var out bytes.Buffer
	useCmd.SetOut(&out)
	t.Cleanup(func() {
		useCmd.SetOut(nil)
	})

	err := useCmd.RunE(useCmd, nil)
	if err == nil {
		t.Fatal("expected non-interactive use without argument to require an environment")
	}
	if !strings.Contains(err.Error(), "environment is required") {
		t.Fatalf("error = %q, want environment required", err)
	}
	got := out.String()
	for _, want := range []string{
		"Available environments:",
		"1. development (current)",
		"2. production",
		"3. staging",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

func TestUseListShowsRemoteEnvironmentsWithoutSwitching(t *testing.T) {
	withTempWorkingDir(t)
	setupUseCommandTest(t, false)

	var sawWrite bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/env/list":
			_, _ = w.Write([]byte(`{"environments":["development","production","staging"]}`))
		case r.URL.Path == "/api/v1/env/pull" || r.URL.Path == "/api/v1/env/push":
			sawWrite = true
			http.Error(w, `{"error":"unexpected write path"}`, http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	})
	restore := stubAPIClient(handler)
	t.Cleanup(restore)
	var out bytes.Buffer
	useListCmd.SetOut(&out)
	t.Cleanup(func() {
		useListCmd.SetOut(nil)
	})

	if err := useListCmd.RunE(useListCmd, nil); err != nil {
		t.Fatalf("use list: %v", err)
	}
	if sawWrite {
		t.Fatal("use list pulled or pushed an environment")
	}
	if out.String() != "development\nproduction\nstaging\n" {
		t.Fatalf("output = %q, want environment list", out.String())
	}
	config, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}
	if config.ActiveEnvironment != "development" {
		t.Fatalf("ActiveEnvironment = %q, want unchanged development", config.ActiveEnvironment)
	}
}

func setupUseCommandTest(t *testing.T, pushCurrent bool) {
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
	if err := os.WriteFile(".env", []byte("CURRENT=value\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	oldKey := useKey
	oldPushCurrent := usePushCurrent
	oldFile := useFile
	t.Cleanup(func() {
		useKey = oldKey
		usePushCurrent = oldPushCurrent
		useFile = oldFile
	})
	useKey = "passphrase"
	usePushCurrent = pushCurrent
	useFile = ""
}

func useHandler(t *testing.T, sawPush, sawPull *bool) http.Handler {
	t.Helper()

	targetPayload, err := obecrypto.EncryptWithPassphrase([]byte("TARGET=value\n"), "passphrase")
	if err != nil {
		t.Fatalf("EncryptWithPassphrase: %v", err)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/env/push":
			*sawPush = true
			_, _ = w.Write([]byte(`{"message":"Pushed successfully","version":2}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/env/pull":
			*sawPull = true
			if r.URL.Query().Get("project") != "obsecurenv" || r.URL.Query().Get("environment") != "production" {
				http.Error(w, `{"error":"bad pull query"}`, http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"project_slug":      "obsecurenv",
				"environment":       "production",
				"version":           1,
				"encrypted_payload": targetPayload,
				"checksum":          "checksum",
			})
		default:
			http.NotFound(w, r)
		}
	})
}

func stubAPIClient(handler http.Handler) func() {
	oldNewAPIClient := newAPIClient
	newAPIClient = func(baseURL, token string) *api.Client {
		client := api.New(baseURL, token)
		client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return responseFromHandler(handler, req), nil
		})}
		return client
	}
	return func() {
		newAPIClient = oldNewAPIClient
	}
}
