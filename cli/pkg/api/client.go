package api

import (
	"bytes"
	"encoding/json"
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

func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
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
		if errBody.Error != "" {
			return fmt.Errorf("api error %d: %s", resp.StatusCode, errBody.Error)
		}
		return fmt.Errorf("api error %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
