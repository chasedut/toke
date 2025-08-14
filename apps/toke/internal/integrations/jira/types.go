package jira

import (
	"time"
)

// Issue represents a Jira issue
type Issue struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Self   string `json:"self"`
	Fields IssueFields `json:"fields"`
}

// IssueFields contains the fields of a Jira issue
type IssueFields struct {
	Summary     string        `json:"summary"`
	Description interface{}   `json:"description"` // Can be string or ADF format
	IssueType   IssueType     `json:"issuetype"`
	Project     Project       `json:"project"`
	Priority    *Priority     `json:"priority,omitempty"`
	Status      Status        `json:"status"`
	Assignee    *User         `json:"assignee,omitempty"`
	Reporter    *User         `json:"reporter,omitempty"`
	Created     time.Time     `json:"created"`
	Updated     time.Time     `json:"updated"`
	Labels      []string      `json:"labels,omitempty"`
	Components  []Component   `json:"components,omitempty"`
	FixVersions []Version     `json:"fixVersions,omitempty"`
	Epic        string        `json:"epic,omitempty"`
	Sprint      *Sprint       `json:"sprint,omitempty"`
	StoryPoints float64       `json:"storyPoints,omitempty"`
	
	// Custom fields - map to handle dynamic fields
	CustomFields map[string]interface{} `json:"-"`
}

// IssueType represents a Jira issue type
type IssueType struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IconURL     string `json:"iconUrl,omitempty"`
}

// Project represents a Jira project
type Project struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
	Self string `json:"self"`
}

// Priority represents a Jira priority
type Priority struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	IconURL string `json:"iconUrl,omitempty"`
}

// Status represents a Jira status
type Status struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	StatusCategory StatusCategory `json:"statusCategory"`
}

// StatusCategory represents a Jira status category
type StatusCategory struct {
	ID    int    `json:"id"`
	Key   string `json:"key"`
	Name  string `json:"name"`
	Color string `json:"colorName"`
}

// User represents a Jira user
type User struct {
	AccountID    string `json:"accountId"`
	EmailAddress string `json:"emailAddress,omitempty"`
	DisplayName  string `json:"displayName"`
	Active       bool   `json:"active"`
}

// Component represents a Jira component
type Component struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Version represents a Jira version
type Version struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Released    bool   `json:"released"`
	ReleaseDate string `json:"releaseDate,omitempty"`
}

// Sprint represents a Jira sprint
type Sprint struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	State      string    `json:"state"`
	StartDate  time.Time `json:"startDate,omitempty"`
	EndDate    time.Time `json:"endDate,omitempty"`
	CompleteDate time.Time `json:"completeDate,omitempty"`
}

// Comment represents a Jira comment
type Comment struct {
	ID      string    `json:"id"`
	Author  User      `json:"author"`
	Body    interface{} `json:"body"` // Can be string or ADF format
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

// CommentResponse represents the response from the comments endpoint
type CommentResponse struct {
	StartAt    int       `json:"startAt"`
	MaxResults int       `json:"maxResults"`
	Total      int       `json:"total"`
	Comments   []Comment `json:"comments"`
}

// SearchRequest represents a JQL search request
type SearchRequest struct {
	JQL        string   `json:"jql"`
	StartAt    int      `json:"startAt,omitempty"`
	MaxResults int      `json:"maxResults,omitempty"`
	Fields     []string `json:"fields,omitempty"`
}

// SearchResponse represents a JQL search response
type SearchResponse struct {
	StartAt    int     `json:"startAt"`
	MaxResults int     `json:"maxResults"`
	Total      int     `json:"total"`
	Issues     []Issue `json:"issues"`
}

// CreateIssueRequest represents a request to create an issue
type CreateIssueRequest struct {
	Fields CreateIssueFields `json:"fields"`
}

// CreateIssueFields contains fields for creating an issue
type CreateIssueFields struct {
	Summary     string                 `json:"summary"`
	Description interface{}            `json:"description,omitempty"` // Can be string or ADF
	IssueType   IDField                `json:"issuetype"`
	Project     IDField                `json:"project"`
	Priority    *IDField               `json:"priority,omitempty"`
	Assignee    *AccountIDField        `json:"assignee,omitempty"`
	Labels      []string               `json:"labels,omitempty"`
	Components  []IDField              `json:"components,omitempty"`
	FixVersions []IDField              `json:"fixVersions,omitempty"`
	
	// Custom fields
	CustomFields map[string]interface{} `json:"-"`
}

// IDField represents a field with just an ID
type IDField struct {
	ID string `json:"id"`
}

// AccountIDField represents a user field
type AccountIDField struct {
	AccountID string `json:"accountId"`
}

// UpdateIssueRequest represents a request to update an issue
type UpdateIssueRequest struct {
	Fields map[string]interface{} `json:"fields,omitempty"`
	Update map[string][]UpdateOperation `json:"update,omitempty"`
}

// UpdateOperation represents an update operation
type UpdateOperation struct {
	Add    interface{} `json:"add,omitempty"`
	Set    interface{} `json:"set,omitempty"`
	Remove interface{} `json:"remove,omitempty"`
}

// AddCommentRequest represents a request to add a comment
type AddCommentRequest struct {
	Body interface{} `json:"body"` // Can be string or ADF format
}

// Transition represents a workflow transition
type Transition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   Status `json:"to"`
}

// TransitionRequest represents a request to transition an issue
type TransitionRequest struct {
	Transition IDField `json:"transition"`
}
