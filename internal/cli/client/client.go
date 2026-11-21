// Package client is a thin JSON client for the v2 REST API (Piston compatible).
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client talks to one cfm-sendbox server.
type Client struct {
	Base string
	HTTP *http.Client
}

// New creates a client for the given base URL (e.g. http://127.0.0.1:2000).
func New(base string) *Client {
	return &Client{Base: strings.TrimRight(base, "/"), HTTP: &http.Client{}}
}

// APIError is a non-2xx answer from the server.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return e.Message }

// Do sends a JSON request and decodes a JSON response into out (if not nil).
func (c *Client) Do(method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.Base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Message string `json:"message"`
		}
		msg := ""
		if json.Unmarshal(data, &e) == nil {
			msg = e.Message
		}
		if msg == "" {
			msg = strings.TrimSpace(string(data))
		}
		if msg == "" {
			msg = fmt.Sprintf("request failed: %s", resp.Status)
		}
		return &APIError{Status: resp.StatusCode, Message: msg}
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("invalid response from server: %w", err)
		}
	}
	return nil
}

// Package is an entry of GET /api/v2/packages.
type Package struct {
	Language        string `json:"language"`
	LanguageVersion string `json:"language_version"`
	Installed       bool   `json:"installed"`
}

// PackageRef selects a package for install/uninstall.
type PackageRef struct {
	Language string `json:"language"`
	Version  string `json:"version"`
}

// Runtime is an entry of GET /api/v2/runtimes.
type Runtime struct {
	Language string   `json:"language"`
	Version  string   `json:"version"`
	Aliases  []string `json:"aliases"`
	Runtime  string   `json:"runtime,omitempty"`
}

// Packages lists every package known to the server's repository.
func (c *Client) Packages() ([]Package, error) {
	var out []Package
	return out, c.Do(http.MethodGet, "/api/v2/packages", nil, &out)
}

// Runtimes lists the installed runtimes.
func (c *Client) Runtimes() ([]Runtime, error) {
	var out []Runtime
	return out, c.Do(http.MethodGet, "/api/v2/runtimes", nil, &out)
}

// Install installs a package.
func (c *Client) Install(ref PackageRef) (PackageRef, error) {
	var out PackageRef
	return out, c.Do(http.MethodPost, "/api/v2/packages", ref, &out)
}

// Uninstall removes a package.
func (c *Client) Uninstall(ref PackageRef) (PackageRef, error) {
	var out PackageRef
	return out, c.Do(http.MethodDelete, "/api/v2/packages", ref, &out)
}

// File is a source file of an execute request.
type File struct {
	Name     string `json:"name"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// ExecuteRequest is the body of POST /api/v2/execute.
type ExecuteRequest struct {
	Language       string   `json:"language"`
	Version        string   `json:"version"`
	Files          []File   `json:"files"`
	Args           []string `json:"args"`
	Stdin          string   `json:"stdin"`
	CompileTimeout *int     `json:"compile_timeout,omitempty"`
	RunTimeout     *int     `json:"run_timeout,omitempty"`
}

// Stage is the result of the compile or run stage.
type Stage struct {
	Stdout  string  `json:"stdout"`
	Stderr  string  `json:"stderr"`
	Output  string  `json:"output"`
	Code    *int    `json:"code"`
	Signal  *string `json:"signal"`
	Message *string `json:"message"`
	Status  *string `json:"status"`
}

// ExecuteResponse is the answer of POST /api/v2/execute.
type ExecuteResponse struct {
	Language string `json:"language"`
	Version  string `json:"version"`
	Compile  *Stage `json:"compile"`
	Run      *Stage `json:"run"`
}

// Execute runs code non-interactively.
func (c *Client) Execute(req ExecuteRequest) (*ExecuteResponse, error) {
	var out ExecuteResponse
	if err := c.Do(http.MethodPost, "/api/v2/execute", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
