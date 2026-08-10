package cmd

import (
	"net/http"
	"testing"
)

func TestValidateLinkedProjectAllowsLegacyConfig(t *testing.T) {
	_, err := validateLinkedProject(&ProjectConfig{ProjectSlug: "app"}, apiClientForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("legacy project validation should not require an identity lookup")
	})))
	if err != nil {
		t.Fatalf("error = %v, want legacy config to remain usable", err)
	}
}

func TestValidateLinkedProjectRejectsTransferredProject(t *testing.T) {
	client := apiClientForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := validateLinkedProject(
		&ProjectConfig{ProjectID: "project-id", ProjectSlug: "app"},
		client,
	)
	if err == nil || err.Error() != "project \"app\" is no longer owned by this account; run obe init to relink this directory" {
		t.Fatalf("error = %v, want ownership-loss error", err)
	}
}

func TestValidateLinkedProjectRejectsReusedSlug(t *testing.T) {
	client := apiClientForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"new-project-id","slug":"app"}`))
	}))

	_, err := validateLinkedProject(
		&ProjectConfig{ProjectID: "old-project-id", ProjectSlug: "app"},
		client,
	)
	if err == nil || err.Error() != "project link for \"app\" is stale; run obe init to relink this directory" {
		t.Fatalf("error = %v, want stale-link error", err)
	}
}

func TestValidateLinkedProjectAcceptsMatchingProject(t *testing.T) {
	client := apiClientForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"project-id","slug":"app"}`))
	}))

	project, err := validateLinkedProject(
		&ProjectConfig{ProjectID: "project-id", ProjectSlug: "app"},
		client,
	)
	if err != nil {
		t.Fatalf("validateLinkedProject: %v", err)
	}
	if project.ID != "project-id" {
		t.Fatalf("project ID = %q, want project-id", project.ID)
	}
}
