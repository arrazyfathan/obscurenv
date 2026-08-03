package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/obscurenv/obscurenv/cli/pkg/api"
)

func TestInitLinksExistingRemoteProjectAfterCreateConflict(t *testing.T) {
	withTempWorkingDir(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OBE_API_URL", "")

	var sawCreate bool
	var sawGet bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			sawCreate = true
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"project already exists"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/obsecurenv":
			sawGet = true
			_, _ = w.Write([]byte(`{"id":"project-id","name":"Obsecurenv","slug":"obsecurenv"}`))
		default:
			http.NotFound(w, r)
		}
	})

	if err := saveCredentials(Credentials{Token: "test-token", APIURL: "http://obe.test"}); err != nil {
		t.Fatalf("saveCredentials: %v", err)
	}

	oldProject := initProject
	oldProjectName := initProjectName
	oldCreateProject := initCreateProject
	oldEnvironment := initEnvironment
	oldNewAPIClient := newAPIClient
	t.Cleanup(func() {
		initProject = oldProject
		initProjectName = oldProjectName
		initCreateProject = oldCreateProject
		initEnvironment = oldEnvironment
		newAPIClient = oldNewAPIClient
		initCmd.SetOut(nil)
	})
	newAPIClient = func(baseURL, token string) *api.Client {
		client := api.New(baseURL, token)
		client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return responseFromHandler(handler, req), nil
		})}
		return client
	}
	initProject = "obsecurenv"
	initProjectName = "Obsecurenv"
	initCreateProject = true
	initEnvironment = "production"
	var out bytes.Buffer
	initCmd.SetOut(&out)

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if !sawCreate {
		t.Fatal("expected init to try creating the remote project")
	}
	if !sawGet {
		t.Fatal("expected init to verify the existing remote project")
	}

	config, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}
	if config.ProjectSlug != "obsecurenv" {
		t.Fatalf("ProjectSlug = %q, want obsecurenv", config.ProjectSlug)
	}
	if config.ActiveEnvironment != "production" {
		t.Fatalf("ActiveEnvironment = %q, want production", config.ActiveEnvironment)
	}
	if !bytes.Contains(out.Bytes(), []byte(`already exists; linked local config`)) {
		t.Fatalf("output = %q, want link message", out.String())
	}
}

func TestInitDoesNotLinkWhenConflictCannotBeVerified(t *testing.T) {
	client := apiClientForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"project already exists"}`))
		case http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"project not found"}`))
		default:
			http.NotFound(w, r)
		}
	}))

	_, _, err := createOrLinkProject(client, createProjectRequestForTest())
	if err == nil {
		t.Fatal("expected createOrLinkProject to fail")
	}
}

func TestCreateOrLinkProjectUsesCreatedSlug(t *testing.T) {
	client := apiClientForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id":   "project-id",
			"slug": "created-slug",
		})
	}))

	resp, created, err := createOrLinkProject(client, createProjectRequestForTest())
	if err != nil {
		t.Fatalf("createOrLinkProject: %v", err)
	}
	if !created {
		t.Fatal("created = false, want true")
	}
	if resp.Slug != "created-slug" {
		t.Fatalf("Slug = %q, want created-slug", resp.Slug)
	}
}

func apiClientForTest(t *testing.T, handler http.Handler) *api.Client {
	t.Helper()

	client := api.New("http://obe.test", "test-token")
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return responseFromHandler(handler, req), nil
	})}
	return client
}

func createProjectRequestForTest() api.CreateProjectRequest {
	return api.CreateProjectRequest{
		Name: "Obsecurenv",
		Slug: "obsecurenv",
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func responseFromHandler(handler http.Handler, req *http.Request) *http.Response {
	rec := newResponseRecorder()
	handler.ServeHTTP(rec, req)
	return &http.Response{
		StatusCode: rec.statusCode(),
		Header:     rec.header,
		Body:       io.NopCloser(bytes.NewReader(rec.body.Bytes())),
		Request:    req,
	}
}

type responseRecorder struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{header: make(http.Header)}
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(data)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
}

func (r *responseRecorder) statusCode() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}
