package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type PushRequest struct {
	ProjectSlug      string `json:"project_slug"`
	Environment      string `json:"environment"`
	EncryptedPayload string `json:"encrypted_payload"`
	Checksum         string `json:"checksum"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Username string `json:"username,omitempty"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email     string `json:"email"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password"`
	TokenName string `json:"token_name"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

type CreateProjectRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type CreateProjectResponse struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
}

type UpdateProjectNameRequest struct {
	Name string `json:"name"`
}

type ProjectResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("api error %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("api error %d", e.StatusCode)
}

func IsStatus(err error, statusCode int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == statusCode
}

type PushResponse struct {
	Message string `json:"message"`
	Version int    `json:"version"`
}

type PullResponse struct {
	ProjectSlug      string `json:"project_slug"`
	Environment      string `json:"environment"`
	Version          int    `json:"version"`
	EncryptedPayload string `json:"encrypted_payload"`
	Checksum         string `json:"checksum"`
}

type ListResponse struct {
	Environments []string `json:"environments"`
}

type Token struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	CreatedAt string  `json:"created_at"`
	ExpiresAt *string `json:"expires_at"`
}

type CreateTokenRequest struct {
	Name          string `json:"name"`
	ExpiresInDays *int   `json:"expires_in_days,omitempty"`
}

type CreateTokenResponse struct {
	Token     string  `json:"token"`
	ID        string  `json:"id"`
	ExpiresAt *string `json:"expires_at"`
}

type ExportItem struct {
	Environment      string `json:"environment"`
	Version          int    `json:"version"`
	Checksum         string `json:"checksum"`
	EncryptedPayload string `json:"encrypted_payload"`
	CreatedAt        string `json:"created_at"`
}

type ExportResponse struct {
	ProjectSlug  string       `json:"project_slug"`
	Environments []ExportItem `json:"environments"`
}

func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) Register(req RegisterRequest) error {
	return c.do(http.MethodPost, "/api/v1/auth/register", req, nil)
}

func (c *Client) Login(req LoginRequest) (*LoginResponse, error) {
	var out LoginResponse
	if err := c.do(http.MethodPost, "/api/v1/auth/login", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CreateProject(req CreateProjectRequest) (*CreateProjectResponse, error) {
	var out CreateProjectResponse
	if err := c.do(http.MethodPost, "/api/v1/projects", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetProject(slug string) (*ProjectResponse, error) {
	path := "/api/v1/projects/" + url.PathEscape(slug)
	var out ProjectResponse
	if err := c.do(http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteProject(slug string) error {
	path := "/api/v1/projects/" + url.PathEscape(slug)
	return c.do(http.MethodDelete, path, nil, nil)
}

func (c *Client) UpdateProjectName(slug, name string) (*ProjectResponse, error) {
	path := "/api/v1/projects/" + url.PathEscape(slug)
	var out ProjectResponse
	if err := c.do(http.MethodPatch, path, UpdateProjectNameRequest{Name: name}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Push(req PushRequest) (*PushResponse, error) {
	var out PushResponse
	if err := c.do(http.MethodPost, "/api/v1/env/push", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Pull(project, environment string) (*PullResponse, error) {
	path := "/api/v1/env/pull?project=" + url.QueryEscape(project) + "&environment=" + url.QueryEscape(environment)
	var out PullResponse
	if err := c.do(http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) List(project string) (*ListResponse, error) {
	path := "/api/v1/env/list?project=" + url.QueryEscape(project)
	var out ListResponse
	if err := c.do(http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteEnvironment(project, environment string) error {
	path := "/api/v1/env?project=" + url.QueryEscape(project) + "&environment=" + url.QueryEscape(environment)
	return c.do(http.MethodDelete, path, nil, nil)
}

func (c *Client) ListTokens() ([]Token, error) {
	var out struct {
		Tokens []Token `json:"tokens"`
	}
	if err := c.do(http.MethodGet, "/api/v1/tokens", nil, &out); err != nil {
		return nil, err
	}
	return out.Tokens, nil
}

func (c *Client) CreateToken(name string, expiresInDays *int) (*CreateTokenResponse, error) {
	var out CreateTokenResponse
	if err := c.do(http.MethodPost, "/api/v1/tokens", CreateTokenRequest{Name: name, ExpiresInDays: expiresInDays}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RevokeToken(id string) error {
	return c.do(http.MethodDelete, "/api/v1/tokens/"+url.PathEscape(id), nil, nil)
}

func (c *Client) ChangePassword(current, newPassword string) error {
	req := struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}{CurrentPassword: current, NewPassword: newPassword}
	return c.do(http.MethodPost, "/api/v1/user/password", req, nil)
}

func (c *Client) DeleteAccount() error {
	req := struct {
		Confirm bool `json:"confirm"`
	}{Confirm: true}
	return c.do(http.MethodDelete, "/api/v1/user", req, nil)
}

func (c *Client) Export(project string) (*ExportResponse, error) {
	path := "/api/v1/env/export?project=" + url.QueryEscape(project)
	var out ExportResponse
	if err := c.do(http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) do(method, path string, body any, out any) error {
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return &APIError{StatusCode: resp.StatusCode, Message: errBody.Error}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
