package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
