package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/obscurenv/obscurenv/cli/pkg/api"
)

func TestInitLinksExistingRemoteProjectBeforeCreate(t *testing.T) {
	withTempWorkingDir(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OBE_API_URL", "")

	var sawCreate bool
	var sawGet bool
	var sawList bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			sawCreate = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"project-id","slug":"obsecurenv"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/obsecurenv":
			sawGet = true
			_, _ = w.Write([]byte(`{"id":"project-id","name":"Obsecurenv","slug":"obsecurenv"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/env/list":
			sawList = true
			_, _ = w.Write([]byte(`{"environments":[]}`))
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
	oldFile := initFile
	oldNewAPIClient := newAPIClient
	t.Cleanup(func() {
		initProject = oldProject
		initProjectName = oldProjectName
		initCreateProject = oldCreateProject
		initEnvironment = oldEnvironment
		initFile = oldFile
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
	initProjectName = ""
	initCreateProject = false
	initEnvironment = ""
	initFile = ""
	var out bytes.Buffer
	initCmd.SetOut(&out)

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if !sawGet {
		t.Fatal("expected init to check the existing remote project")
	}
	if sawCreate {
		t.Fatal("expected init not to create when remote project exists")
	}
	if !sawList {
		t.Fatal("expected init to list remote environments")
	}

	config, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}
	if config.ProjectSlug != "obsecurenv" {
		t.Fatalf("ProjectSlug = %q, want obsecurenv", config.ProjectSlug)
	}
	if config.ActiveEnvironment != "development" {
		t.Fatalf("ActiveEnvironment = %q, want development", config.ActiveEnvironment)
	}
	if !bytes.Contains(out.Bytes(), []byte(`Linked local config to remote project "obsecurenv".`)) {
		t.Fatalf("output = %q, want link message", out.String())
	}
}

func TestInitAutoLinkRequiresEnvironmentWhenRemoteEnvironmentsExist(t *testing.T) {
	withTempWorkingDir(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OBE_API_URL", "")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/obsecurenv":
			_, _ = w.Write([]byte(`{"id":"project-id","name":"Obsecurenv","slug":"obsecurenv"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/env/list":
			_, _ = w.Write([]byte(`{"environments":["development","staging","production"]}`))
		default:
			http.NotFound(w, r)
		}
	})

	setupInitCommandTest(t, handler)
	initProject = "obsecurenv"
	initEnvironment = ""
	var out bytes.Buffer
	initCmd.SetOut(&out)

	err := initCmd.RunE(initCmd, nil)
	if err == nil {
		t.Fatal("expected init to require --env in non-interactive mode")
	}
	if !strings.Contains(err.Error(), "environment is required") {
		t.Fatalf("error = %q, want environment required", err)
	}
	for _, want := range []string{
		"Available environments:",
		"1. development",
		"2. staging",
		"3. production",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output = %q, want %q", out.String(), want)
		}
	}
}

func TestInitAutoLinkUsesEnvFlagWithoutListingRemoteEnvironments(t *testing.T) {
	withTempWorkingDir(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OBE_API_URL", "")

	var sawList bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/obsecurenv":
			_, _ = w.Write([]byte(`{"id":"project-id","name":"Obsecurenv","slug":"obsecurenv"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/env/list":
			sawList = true
			_, _ = w.Write([]byte(`{"environments":["development"]}`))
		default:
			http.NotFound(w, r)
		}
	})

	setupInitCommandTest(t, handler)
	initProject = "obsecurenv"
	initEnvironment = "production"
	var out bytes.Buffer
	initCmd.SetOut(&out)

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if sawList {
		t.Fatal("expected init not to list remote environments when --env is provided")
	}
	config, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}
	if config.ActiveEnvironment != "production" {
		t.Fatalf("ActiveEnvironment = %q, want production", config.ActiveEnvironment)
	}
}

func TestResolveEnvironmentChoiceAcceptsNumberAndName(t *testing.T) {
	environments := []string{"development", "staging", "production"}

	got, err := resolveEnvironmentChoice("2", environments)
	if err != nil {
		t.Fatalf("resolveEnvironmentChoice number: %v", err)
	}
	if got != "staging" {
		t.Fatalf("number choice = %q, want staging", got)
	}

	got, err = resolveEnvironmentChoice("production", environments)
	if err != nil {
		t.Fatalf("resolveEnvironmentChoice name: %v", err)
	}
	if got != "production" {
		t.Fatalf("name choice = %q, want production", got)
	}
}

func TestWriteInitConfigCreatesGitignoreForProjectConfig(t *testing.T) {
	withTempWorkingDir(t)
	initFile = ""
	t.Cleanup(func() {
		initFile = ""
	})

	if err := writeInitConfig("obsecurenv", "development"); err != nil {
		t.Fatalf("writeInitConfig: %v", err)
	}

	data, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if string(data) != ".obe.json\n" {
		t.Fatalf(".gitignore = %q, want .obe.json entry", data)
	}
}

func TestWriteInitConfigAppendsProjectConfigToGitignore(t *testing.T) {
	withTempWorkingDir(t)
	initFile = ""
	t.Cleanup(func() {
		initFile = ""
	})
	if err := os.WriteFile(".gitignore", []byte(".env"), 0600); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	if err := writeInitConfig("obsecurenv", "development"); err != nil {
		t.Fatalf("writeInitConfig: %v", err)
	}

	data, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if string(data) != ".env\n.obe.json\n" {
		t.Fatalf(".gitignore = %q, want appended .obe.json entry", data)
	}
}

func TestWriteInitConfigDoesNotDuplicateProjectConfigInGitignore(t *testing.T) {
	withTempWorkingDir(t)
	initFile = ""
	t.Cleanup(func() {
		initFile = ""
	})
	if err := os.WriteFile(".gitignore", []byte(".env\n.obe.json\n"), 0600); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	if err := writeInitConfig("obsecurenv", "development"); err != nil {
		t.Fatalf("writeInitConfig: %v", err)
	}

	data, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if string(data) != ".env\n.obe.json\n" {
		t.Fatalf(".gitignore = %q, want unchanged entries", data)
	}
}

func TestInitCreatesRemoteProjectWhenSlugIsMissing(t *testing.T) {
	withTempWorkingDir(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OBE_API_URL", "")

	var sawCreate bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/obsecurenv":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"project not found"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			sawCreate = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"project-id","slug":"obsecurenv"}`))
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
	oldFile := initFile
	oldNewAPIClient := newAPIClient
	t.Cleanup(func() {
		initProject = oldProject
		initProjectName = oldProjectName
		initCreateProject = oldCreateProject
		initEnvironment = oldEnvironment
		initFile = oldFile
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
	initCreateProject = false
	initEnvironment = "production"
	initFile = ""
	var out bytes.Buffer
	initCmd.SetOut(&out)

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if !sawCreate {
		t.Fatal("expected init to create the remote project")
	}
	if bytes.Contains(out.Bytes(), []byte(`not found`)) {
		t.Fatalf("output = %q, did not want not-found message", out.String())
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
}

func TestInitReportsExistingConfigWithoutOverwrite(t *testing.T) {
	withTempWorkingDir(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OBE_API_URL", "")

	if err := saveCredentials(Credentials{Token: "test-token", APIURL: "http://obe.test"}); err != nil {
		t.Fatalf("saveCredentials: %v", err)
	}
	if err := saveProjectConfig(ProjectConfig{
		ProjectSlug:       "linked-app",
		ActiveEnvironment: "staging",
		EnvFile:           ".env",
	}); err != nil {
		t.Fatalf("saveProjectConfig: %v", err)
	}

	var sawGet bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/linked-app":
			sawGet = true
			_, _ = w.Write([]byte(`{"id":"project-id","name":"Linked App","slug":"linked-app"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			t.Fatal("expected init not to create when local config already exists")
		default:
			http.NotFound(w, r)
		}
	})

	setupInitCommandClient(t, handler)
	initProject = "other-app"
	initEnvironment = "production"
	var out bytes.Buffer
	initCmd.SetOut(&out)

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	if !sawGet {
		t.Fatal("expected init to verify existing local config remotely")
	}
	if !bytes.Contains(out.Bytes(), []byte("Already initialized.")) || !bytes.Contains(out.Bytes(), []byte("Remote: linked")) {
		t.Fatalf("output = %q, want already initialized linked status", out.String())
	}

	config, err := loadProjectConfig()
	if err != nil {
		t.Fatalf("loadProjectConfig: %v", err)
	}
	if config.ProjectSlug != "linked-app" || config.ActiveEnvironment != "staging" {
		t.Fatalf("config overwritten: %+v", config)
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

func setupInitCommandTest(t *testing.T, handler http.Handler) {
	t.Helper()

	if err := saveCredentials(Credentials{Token: "test-token", APIURL: "http://obe.test"}); err != nil {
		t.Fatalf("saveCredentials: %v", err)
	}
	setupInitCommandClient(t, handler)
}

func setupInitCommandClient(t *testing.T, handler http.Handler) {
	t.Helper()

	oldProject := initProject
	oldProjectName := initProjectName
	oldCreateProject := initCreateProject
	oldEnvironment := initEnvironment
	oldFile := initFile
	oldNewAPIClient := newAPIClient
	t.Cleanup(func() {
		initProject = oldProject
		initProjectName = oldProjectName
		initCreateProject = oldCreateProject
		initEnvironment = oldEnvironment
		initFile = oldFile
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
