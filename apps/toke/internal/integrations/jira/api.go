package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// SearchIssues searches for issues using JQL
func (c *Client) SearchIssues(ctx context.Context, jql string, fields []string, startAt, maxResults int) (*SearchResponse, error) {
	// Default fields if none specified
	if len(fields) == 0 {
		fields = []string{
			"summary", "description", "status", "issuetype", "priority",
			"assignee", "reporter", "created", "updated", "project",
			"labels", "components", "fixVersions",
		}
	}
	
	req := SearchRequest{
		JQL:        jql,
		StartAt:    startAt,
		MaxResults: maxResults,
		Fields:     fields,
	}
	
	resp, err := c.do(ctx, "POST", "/rest/api/3/search", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}
	
	return &searchResp, nil
}

// GetIssue retrieves a single issue by key
func (c *Client) GetIssue(ctx context.Context, issueKey string, fields []string) (*Issue, error) {
	path := fmt.Sprintf("/rest/api/3/issue/%s", url.PathEscape(issueKey))
	
	if len(fields) > 0 {
		path += "?fields=" + strings.Join(fields, ",")
	}
	
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var issue Issue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, fmt.Errorf("failed to decode issue: %w", err)
	}
	
	return &issue, nil
}

// CreateIssue creates a new issue
func (c *Client) CreateIssue(ctx context.Context, req CreateIssueRequest) (*Issue, error) {
	// Handle custom fields
	if req.Fields.CustomFields != nil {
		// Marshal to map first
		data, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		
		var reqMap map[string]interface{}
		if err := json.Unmarshal(data, &reqMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal to map: %w", err)
		}
		
		// Add custom fields to the fields map
		fieldsMap := reqMap["fields"].(map[string]interface{})
		for k, v := range req.Fields.CustomFields {
			fieldsMap[k] = v
		}
		
		resp, err := c.do(ctx, "POST", "/rest/api/3/issue", reqMap)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		
		var createResp struct {
			ID   string `json:"id"`
			Key  string `json:"key"`
			Self string `json:"self"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
			return nil, fmt.Errorf("failed to decode create response: %w", err)
		}
		
		// Fetch the created issue
		return c.GetIssue(ctx, createResp.Key, nil)
	}
	
	resp, err := c.do(ctx, "POST", "/rest/api/3/issue", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var createResp struct {
		ID   string `json:"id"`
		Key  string `json:"key"`
		Self string `json:"self"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return nil, fmt.Errorf("failed to decode create response: %w", err)
	}
	
	// Fetch the created issue
	return c.GetIssue(ctx, createResp.Key, nil)
}

// UpdateIssue updates an existing issue
func (c *Client) UpdateIssue(ctx context.Context, issueKey string, req UpdateIssueRequest) error {
	path := fmt.Sprintf("/rest/api/3/issue/%s", url.PathEscape(issueKey))
	
	resp, err := c.do(ctx, "PUT", path, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	return nil
}

// GetComments gets all comments for an issue
func (c *Client) GetComments(ctx context.Context, issueKey string) (*CommentResponse, error) {
	path := fmt.Sprintf("/rest/api/3/issue/%s/comment", url.PathEscape(issueKey))
	
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var commentsResp CommentResponse
	if err := json.NewDecoder(resp.Body).Decode(&commentsResp); err != nil {
		return nil, fmt.Errorf("failed to decode comments response: %w", err)
	}
	
	return &commentsResp, nil
}

// AddComment adds a comment to an issue
func (c *Client) AddComment(ctx context.Context, issueKey string, comment string) (*Comment, error) {
	path := fmt.Sprintf("/rest/api/3/issue/%s/comment", url.PathEscape(issueKey))
	
	req := AddCommentRequest{
		Body: comment,
	}
	
	resp, err := c.do(ctx, "POST", path, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var commentResp Comment
	if err := json.NewDecoder(resp.Body).Decode(&commentResp); err != nil {
		return nil, fmt.Errorf("failed to decode comment response: %w", err)
	}
	
	return &commentResp, nil
}

// GetTransitions gets available transitions for an issue
func (c *Client) GetTransitions(ctx context.Context, issueKey string) ([]Transition, error) {
	path := fmt.Sprintf("/rest/api/3/issue/%s/transitions", url.PathEscape(issueKey))
	
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var transResp struct {
		Transitions []Transition `json:"transitions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&transResp); err != nil {
		return nil, fmt.Errorf("failed to decode transitions: %w", err)
	}
	
	return transResp.Transitions, nil
}

// TransitionIssue transitions an issue to a new status
func (c *Client) TransitionIssue(ctx context.Context, issueKey string, transitionID string) error {
	path := fmt.Sprintf("/rest/api/3/issue/%s/transitions", url.PathEscape(issueKey))
	
	req := TransitionRequest{
		Transition: IDField{ID: transitionID},
	}
	
	resp, err := c.do(ctx, "POST", path, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	return nil
}

// GetProjects gets all projects accessible to the user
func (c *Client) GetProjects(ctx context.Context) ([]Project, error) {
	resp, err := c.do(ctx, "GET", "/rest/api/3/project", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var projects []Project
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return nil, fmt.Errorf("failed to decode projects: %w", err)
	}
	
	return projects, nil
}

// GetIssueTypes gets issue types for a project
func (c *Client) GetIssueTypes(ctx context.Context, projectKey string) ([]IssueType, error) {
	path := fmt.Sprintf("/rest/api/3/project/%s", url.PathEscape(projectKey))
	
	resp, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var project struct {
		IssueTypes []IssueType `json:"issueTypes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		return nil, fmt.Errorf("failed to decode project: %w", err)
	}
	
	return project.IssueTypes, nil
}

// GetPriorities gets all priorities
func (c *Client) GetPriorities(ctx context.Context) ([]Priority, error) {
	resp, err := c.do(ctx, "GET", "/rest/api/3/priority", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var priorities []Priority
	if err := json.NewDecoder(resp.Body).Decode(&priorities); err != nil {
		return nil, fmt.Errorf("failed to decode priorities: %w", err)
	}
	
	return priorities, nil
}

// GetFieldsMetadata gets metadata for all fields
func (c *Client) GetFieldsMetadata(ctx context.Context) ([]FieldMetadata, error) {
	resp, err := c.do(ctx, "GET", "/rest/api/3/field", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var fields []FieldMetadata
	if err := json.NewDecoder(resp.Body).Decode(&fields); err != nil {
		return nil, fmt.Errorf("failed to decode fields: %w", err)
	}
	
	return fields, nil
}

// FieldMetadata represents metadata for a Jira field
type FieldMetadata struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Custom      bool     `json:"custom"`
	Orderable   bool     `json:"orderable"`
	Navigable   bool     `json:"navigable"`
	Searchable  bool     `json:"searchable"`
	Schema      Schema   `json:"schema,omitempty"`
	Key         string   `json:"key,omitempty"`
}

// Schema represents the schema of a Jira field
type Schema struct {
	Type     string `json:"type"`
	Items    string `json:"items,omitempty"`
	System   string `json:"system,omitempty"`
	Custom   string `json:"custom,omitempty"`
	CustomID int    `json:"customId,omitempty"`
}
