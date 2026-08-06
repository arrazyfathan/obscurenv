package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdateProjectNameSendsPatchRequest(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var gotBody UpdateProjectNameRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.EscapedPath()
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"id-1","name":"My App","slug":"my-app"}`))
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	project, err := client.UpdateProjectName("my app", "My App")
	if err != nil {
		t.Fatalf("UpdateProjectName returned error: %v", err)
	}

	if gotMethod != http.MethodPatch {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodPatch)
	}
	if gotPath != "/api/v1/projects/my%20app" {
		t.Fatalf("path = %q, want encoded project path", gotPath)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("Authorization = %q, want bearer token", gotAuth)
	}
	if gotBody.Name != "My App" {
		t.Fatalf("request name = %q, want My App", gotBody.Name)
	}
	if project.Name != "My App" || project.Slug != "my-app" {
		t.Fatalf("project = %#v, want renamed project", project)
	}
}

func TestListTokensDecodesTokenList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/tokens" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"tokens":[{"id":"tok-1","name":"ci","created_at":"2026-08-01T00:00:00Z","expires_at":"2026-08-31T00:00:00Z"},{"id":"tok-2","name":"local-cli","created_at":"2026-08-02T00:00:00Z","expires_at":null}]}`))
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	tokens, err := client.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens returned error: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("tokens = %d, want 2", len(tokens))
	}
	if tokens[0].ID != "tok-1" || tokens[0].Name != "ci" {
		t.Fatalf("tokens[0] = %#v, want ci token", tokens[0])
	}
	if tokens[1].ExpiresAt != nil {
		t.Fatalf("tokens[1].ExpiresAt = %v, want nil", tokens[1].ExpiresAt)
	}
}

func TestCreateTokenSendsRequestAndReturnsRawToken(t *testing.T) {
	var gotBody CreateTokenRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/tokens" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"token":"obe_tok_new","id":"tok-1","expires_at":null}`))
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	resp, err := client.CreateToken("ci", nil)
	if err != nil {
		t.Fatalf("CreateToken returned error: %v", err)
	}
	if gotBody.Name != "ci" {
		t.Fatalf("request name = %q, want ci", gotBody.Name)
	}
	if resp.Token != "obe_tok_new" {
		t.Fatalf("token = %q, want obe_tok_new", resp.Token)
	}
}

func TestRevokeTokenSendsAuthenticatedDeleteRequest(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.EscapedPath()
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	if err := client.RevokeToken("tok 1"); err != nil {
		t.Fatalf("RevokeToken returned error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodDelete)
	}
	if gotPath != "/api/v1/tokens/tok%201" {
		t.Fatalf("path = %q, want encoded token path", gotPath)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("Authorization = %q, want bearer token", gotAuth)
	}
}

func TestChangePasswordSendsCurrentAndNewPassword(t *testing.T) {
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/user/password" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	if err := client.ChangePassword("old-pass", "new-pass-123"); err != nil {
		t.Fatalf("ChangePassword returned error: %v", err)
	}
	if gotBody["current_password"] != "old-pass" || gotBody["new_password"] != "new-pass-123" {
		t.Fatalf("request body = %#v, want current and new passwords", gotBody)
	}
}

func TestDeleteAccountSendsConfirmation(t *testing.T) {
	var gotBody map[string]bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/user" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	if err := client.DeleteAccount(); err != nil {
		t.Fatalf("DeleteAccount returned error: %v", err)
	}
	if !gotBody["confirm"] {
		t.Fatalf("request confirm = %v, want true", gotBody["confirm"])
	}
}

func TestExportFetchesEncryptedEnvironments(t *testing.T) {
	var gotProject string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/env/export" {
			http.NotFound(w, r)
			return
		}
		gotProject = r.URL.Query().Get("project")
		_, _ = w.Write([]byte(`{"project_slug":"my app","environments":[{"environment":"production","version":2,"checksum":"abc","encrypted_payload":"opaque","created_at":"2026-08-06T00:00:00Z"}]}`))
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	resp, err := client.Export("my app")
	if err != nil {
		t.Fatalf("Export returned error: %v", err)
	}
	if gotProject != "my app" {
		t.Fatalf("project = %q, want decoded project query", gotProject)
	}
	if len(resp.Environments) != 1 || resp.Environments[0].Environment != "production" {
		t.Fatalf("environments = %#v, want production env", resp.Environments)
	}
}

func TestDeleteProjectUsesAuthenticatedDeleteRequest(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.EscapedPath()
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	if err := client.DeleteProject("my app"); err != nil {
		t.Fatalf("DeleteProject returned error: %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodDelete)
	}
	if gotPath != "/api/v1/projects/my%20app" {
		t.Fatalf("path = %q, want encoded project path", gotPath)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("Authorization = %q, want bearer token", gotAuth)
	}
}

func TestDeleteEnvironmentUsesAuthenticatedDeleteRequest(t *testing.T) {
	var gotMethod, gotPath, gotProject, gotEnvironment, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotProject = r.URL.Query().Get("project")
		gotEnvironment = r.URL.Query().Get("environment")
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(server.URL, "test-token")
	if err := client.DeleteEnvironment("my app", "staging/local"); err != nil {
		t.Fatalf("DeleteEnvironment returned error: %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodDelete)
	}
	if gotPath != "/api/v1/env" {
		t.Fatalf("path = %q, want env path", gotPath)
	}
	if gotProject != "my app" {
		t.Fatalf("project = %q, want decoded project query", gotProject)
	}
	if gotEnvironment != "staging/local" {
		t.Fatalf("environment = %q, want decoded environment query", gotEnvironment)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("Authorization = %q, want bearer token", gotAuth)
	}
}
