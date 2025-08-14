package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client represents a Jira API client
type Client struct {
	baseURL    string
	httpClient *http.Client
	auth       AuthMethod
}

// AuthMethod represents the authentication method for Jira
type AuthMethod interface {
	// Apply adds authentication to the HTTP request
	Apply(req *http.Request) error
}

// BasicAuth implements basic authentication for Jira
type BasicAuth struct {
	Email    string
	APIToken string
}

func (b BasicAuth) Apply(req *http.Request) error {
	req.SetBasicAuth(b.Email, b.APIToken)
	return nil
}

// OAuth2Auth implements OAuth2 authentication for Jira
type OAuth2Auth struct {
	AccessToken  string
	RefreshToken string
	TokenExpiry  time.Time
	ClientID     string
	ClientSecret string
}

func (o OAuth2Auth) Apply(req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+o.AccessToken)
	return nil
}

// NewClient creates a new Jira API client
func NewClient(baseURL string, auth AuthMethod) (*Client, error) {
	// Ensure baseURL ends without trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")
	
	// Validate URL
	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		auth: auth,
	}, nil
}

// SetHTTPClient allows setting a custom HTTP client (useful for debugging)
func (c *Client) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// do performs an HTTP request with authentication
func (c *Client) do(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	// Build full URL
	fullURL := c.baseURL + path
	
	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}
	
	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Set headers
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	
	// Apply authentication
	if err := c.auth.Apply(req); err != nil {
		return nil, fmt.Errorf("failed to apply authentication: %w", err)
	}
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	
	// Check for errors
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		errorBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Jira API error (status %d): %s", resp.StatusCode, string(errorBody))
	}
	
	return resp, nil
}

// TestConnection verifies the connection to Jira
func (c *Client) TestConnection(ctx context.Context) error {
	resp, err := c.do(ctx, "GET", "/rest/api/3/myself", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	var user struct {
		AccountID   string `json:"accountId"`
		DisplayName string `json:"displayName"`
		EmailAddress string `json:"emailAddress"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return fmt.Errorf("failed to decode user info: %w", err)
	}
	
	return nil
}

// BaseURL returns the base URL of the Jira instance
func (c *Client) BaseURL() string {
	return c.baseURL
}
