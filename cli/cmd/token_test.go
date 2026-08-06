package cmd

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestTokenListPrintsTokens(t *testing.T) {
	setupAuthCommandTest(t)

	restore := stubAPIClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/tokens" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"tokens":[{"id":"tok-1","name":"ci","created_at":"2026-08-01T00:00:00Z","expires_at":null}]}`))
	}))
	t.Cleanup(restore)

	var out bytes.Buffer
	tokenListCmd.SetOut(&out)

	if err := tokenListCmd.RunE(tokenListCmd, nil); err != nil {
		t.Fatalf("token ls: %v", err)
	}
	if !strings.Contains(out.String(), "tok-1") || !strings.Contains(out.String(), "ci") || !strings.Contains(out.String(), "never") {
		t.Fatalf("output = %q, want token list", out.String())
	}
}

func TestTokenNewCreatesAndPrintsToken(t *testing.T) {
	setupAuthCommandTest(t)

	oldName := tokenNewName
	oldDays := tokenNewDays
	t.Cleanup(func() {
		tokenNewName = oldName
		tokenNewDays = oldDays
		tokenNewCmd.SetOut(nil)
	})
	tokenNewName = "ci"
	tokenNewDays = 30

	var sawCreate bool
	restore := stubAPIClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/tokens" {
			http.NotFound(w, r)
			return
		}
		sawCreate = true
		_, _ = w.Write([]byte(`{"token":"obe_tok_new","id":"tok-1","expires_at":"2026-09-05T00:00:00Z"}`))
	}))
	t.Cleanup(restore)

	var out bytes.Buffer
	tokenNewCmd.SetOut(&out)

	if err := tokenNewCmd.RunE(tokenNewCmd, nil); err != nil {
		t.Fatalf("token new: %v", err)
	}
	if !sawCreate {
		t.Fatal("expected token new to call create API")
	}
	if strings.TrimSpace(out.String()) != "obe_tok_new" {
		t.Fatalf("output = %q, want raw token", out.String())
	}
}

func TestTokenRmRevokesToken(t *testing.T) {
	setupAuthCommandTest(t)

	var sawRevoke bool
	restore := stubAPIClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/tokens/tok-1" {
			http.NotFound(w, r)
			return
		}
		sawRevoke = true
		_, _ = w.Write([]byte(`{"message":"token revoked"}`))
	}))
	t.Cleanup(restore)

	var out bytes.Buffer
	tokenRmCmd.SetOut(&out)

	if err := tokenRmCmd.RunE(tokenRmCmd, []string{"tok-1"}); err != nil {
		t.Fatalf("token rm: %v", err)
	}
	if !sawRevoke {
		t.Fatal("expected token rm to call revoke API")
	}
	if !strings.Contains(out.String(), `Revoked token "tok-1"`) {
		t.Fatalf("output = %q, want revoke confirmation", out.String())
	}
}

func setupAuthCommandTest(t *testing.T) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OBE_API_URL", "")
	if err := saveCredentials(Credentials{Token: "test-token", APIURL: "http://obe.test"}); err != nil {
		t.Fatalf("saveCredentials: %v", err)
	}
	t.Cleanup(func() {
		tokenListCmd.SetOut(nil)
		tokenRmCmd.SetOut(nil)
	})
}
