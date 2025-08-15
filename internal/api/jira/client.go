package jira

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	baseURL   string
	email     string
	apiToken  string
	httpClient *http.Client
}

type Issue struct {
	Key    string      `json:"key"`
	Fields IssueFields `json:"fields"`
}

type IssueFields struct {
	Summary     string              `json:"summary"`
	Description *Description        `json:"description,omitempty"`
	IssueType   IssueType          `json:"issuetype"`
	Status      Status             `json:"status"`
	Assignee    *User              `json:"assignee,omitempty"`
	Comments    *CommentsContainer `json:"comment,omitempty"`
	Attachments []Attachment       `json:"attachment,omitempty"`
	Created     string             `json:"created"`
	Updated     string             `json:"updated"`
	Priority    *Priority          `json:"priority,omitempty"`
	Labels      []string           `json:"labels,omitempty"`
}

type Description struct {
	Type    string         `json:"type"`
	Version int            `json:"version"`
	Content []ContentBlock `json:"content"`
}

type ContentBlock struct {
	Type    string        `json:"type"`
	Content []ContentItem `json:"content,omitempty"`
	Text    string        `json:"text,omitempty"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type IssueType struct {
	Name string `json:"name"`
}

type Status struct {
	Name string `json:"name"`
}

type User struct {
	DisplayName string `json:"displayName"`
	EmailAddress string `json:"emailAddress,omitempty"`
}

type CommentsContainer struct {
	Comments []Comment `json:"comments"`
	Total    int       `json:"total"`
}

type Comment struct {
	Author  User         `json:"author"`
	Body    *Description `json:"body"`
	Created string       `json:"created"`
}

type Attachment struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
	Size     int    `json:"size"`
	MimeType string `json:"mimeType"`
}

type Priority struct {
	Name string `json:"name"`
}

type SearchResult struct {
	Issues []Issue `json:"issues"`
	Total  int     `json:"total"`
}

func NewClient(baseURL, email, apiToken string) *Client {
	return &Client{
		baseURL:  baseURL,
		email:    email,
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) doRequest(method, path string, params url.Values) ([]byte, error) {
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, err
	}

	if params != nil {
		u.RawQuery = params.Encode()
	}

	req, err := http.NewRequest(method, u.String(), nil) //nolint:noctx
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(c.email, c.apiToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *Client) SearchIssues(jql string, maxResults int) (*SearchResult, error) {
	params := url.Values{}
	params.Set("jql", jql)
	params.Set("maxResults", fmt.Sprintf("%d", maxResults))
	params.Set("fields", "key,summary,description,issuetype,status,assignee,comment,attachment,created,updated,priority,labels")

	body, err := c.doRequest("GET", "/rest/api/3/search", params)
	if err != nil {
		return nil, err
	}

	var result SearchResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) GetIssue(issueKey string) (*Issue, error) {
	params := url.Values{}
	params.Set("fields", "key,summary,description,issuetype,status,assignee,comment,attachment,created,updated,priority,labels")

	body, err := c.doRequest("GET", fmt.Sprintf("/rest/api/3/issue/%s", issueKey), params)
	if err != nil {
		return nil, err
	}

	var issue Issue
	if err := json.Unmarshal(body, &issue); err != nil {
		return nil, err
	}

	return &issue, nil
}

// Helper to convert Jira Description to plain text
func (d *Description) ToPlainText() string {
	if d == nil {
		return ""
	}

	result := ""
	for _, block := range d.Content {
		if block.Type == "paragraph" {
			for _, item := range block.Content {
				if item.Type == "text" {
					result += item.Text
				}
			}
			result += "\n"
		} else if block.Type == "text" {
			result += block.Text + "\n"
		}
	}
	return result
}
