package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chasedut/toke/internal/integrations/jira"
	"github.com/chasedut/toke/internal/permission"
)

type JiraCreateParams struct {
	Project     string                 `json:"project"`
	IssueType   string                 `json:"issue_type"`
	Summary     string                 `json:"summary"`
	Description string                 `json:"description,omitempty"`
	Assignee    string                 `json:"assignee,omitempty"`
	Priority    string                 `json:"priority,omitempty"`
	Labels      []string               `json:"labels,omitempty"`
	Components  []string               `json:"components,omitempty"`
	CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
}

type JiraCreatePermissionsParams struct {
	Project     string                 `json:"project"`
	IssueType   string                 `json:"issue_type"`
	Summary     string                 `json:"summary"`
	Description string                 `json:"description,omitempty"`
}

type jiraCreateTool struct {
	client      *jira.Client
	permissions permission.Service
	workingDir  string
}

const (
	JiraCreateToolName        = "jira_create_ticket"
	jiraCreateToolDescription = `Creates a new Jira issue with specified fields.

WHEN TO USE THIS TOOL:
| Use when you need to create a new Jira ticket for tracking work
| Helpful for creating bugs, features, tasks, or other issue types
| Can set initial field values during creation

HOW TO USE:
| Provide required fields: project (key or ID), issue_type (name or ID), and summary
| Optionally specify description, assignee, priority, labels, and other fields
| Project key example: "PROJ", Issue type examples: "Bug", "Task", "Story"

FIELD FORMATS:
| project: Project key (e.g., "PROJ") or ID
| issue_type: Type name (e.g., "Bug", "Task") or ID
| summary: Issue title (required)
| description: Detailed description (optional)
| assignee: User account ID (use jira_search to find users)
| priority: Priority name (e.g., "High", "Medium") or ID
| labels: Array of label strings
| components: Array of component names or IDs
| custom_fields: Object with custom field IDs and values

FEATURES:
| Creates issue with all specified fields in one operation
| Returns the created issue key and details
| Validates required fields before creation

LIMITATIONS:
| Requires appropriate permissions in the project
| Some fields may be required based on project configuration
| Custom fields must use their field IDs (e.g., "customfield_10001")

TIPS:
| Use jira_search to find valid values for assignee, priority, etc.
| Check project configuration for required fields
| Keep summaries concise but descriptive`
)

func NewJiraCreateTool(client *jira.Client, permissions permission.Service, workingDir string) BaseTool {
	return &jiraCreateTool{
		client:      client,
		permissions: permissions,
		workingDir:  workingDir,
	}
}

func (t *jiraCreateTool) Name() string {
	return JiraCreateToolName
}

func (t *jiraCreateTool) Info() ToolInfo {
	return ToolInfo{
		Name:        JiraCreateToolName,
		Description: jiraCreateToolDescription,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project": map[string]interface{}{
					"type":        "string",
					"description": "Project key (e.g., 'PROJ') or ID",
				},
				"issue_type": map[string]interface{}{
					"type":        "string",
					"description": "Issue type name (e.g., 'Bug', 'Task') or ID",
				},
				"summary": map[string]interface{}{
					"type":        "string",
					"description": "Issue summary/title",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Issue description (optional)",
				},
				"assignee": map[string]interface{}{
					"type":        "string",
					"description": "Assignee account ID (optional)",
				},
				"priority": map[string]interface{}{
					"type":        "string",
					"description": "Priority name or ID (optional)",
				},
				"labels": map[string]interface{}{
					"type":        "array",
					"description": "Labels to add (optional)",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
				"components": map[string]interface{}{
					"type":        "array",
					"description": "Component names or IDs (optional)",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
				"custom_fields": map[string]interface{}{
					"type":        "object",
					"description": "Custom field values (optional)",
				},
			},
		},
		Required: []string{"project", "issue_type", "summary"},
	}
}

func (t *jiraCreateTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params JiraCreateParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse("Failed to parse parameters: " + err.Error()), nil
	}

	if params.Project == "" {
		return NewTextErrorResponse("project parameter is required"), nil
	}
	if params.IssueType == "" {
		return NewTextErrorResponse("issue_type parameter is required"), nil
	}
	if params.Summary == "" {
		return NewTextErrorResponse("summary parameter is required"), nil
	}

	sessionID, _ := GetContextValues(ctx)
	if sessionID == "" {
		return ToolResponse{}, fmt.Errorf("session ID is required for Jira operations")
	}

	permParams := JiraCreatePermissionsParams{
		Project:     params.Project,
		IssueType:   params.IssueType,
		Summary:     params.Summary,
		Description: params.Description,
	}

	p := t.permissions.Request(
		permission.CreatePermissionRequest{
			SessionID:   sessionID,
			Path:        t.workingDir,
			ToolCallID:  call.ID,
			ToolName:    JiraCreateToolName,
			Action:      "create",
			Description: fmt.Sprintf("Create Jira issue in project %s: %s", params.Project, params.Summary),
			Params:      permParams,
		},
	)

	if !p {
		return ToolResponse{}, permission.ErrorPermissionDenied
	}

	// Build create request
	createReq := jira.CreateIssueRequest{
		Fields: jira.CreateIssueFields{
			Summary:     params.Summary,
			Description: params.Description,
			Project:     jira.IDField{ID: params.Project},
			IssueType:   jira.IDField{ID: params.IssueType},
			Labels:      params.Labels,
			CustomFields: params.CustomFields,
		},
	}

	// Handle optional fields
	if params.Assignee != "" {
		createReq.Fields.Assignee = &jira.AccountIDField{AccountID: params.Assignee}
	}
	
	if params.Priority != "" {
		createReq.Fields.Priority = &jira.IDField{ID: params.Priority}
	}
	
	if len(params.Components) > 0 {
		createReq.Fields.Components = make([]jira.IDField, len(params.Components))
		for i, comp := range params.Components {
			createReq.Fields.Components[i] = jira.IDField{ID: comp}
		}
	}

	// Create the issue
	issue, err := t.client.CreateIssue(ctx, createReq)
	if err != nil {
		return ToolResponse{}, fmt.Errorf("failed to create issue: %w", err)
	}

	// Format response
	result := fmt.Sprintf("Successfully created **%s** - %s\n\n", issue.Key, issue.Fields.Summary)
	
	result += "**Details:**\n"
	result += fmt.Sprintf("- Project: %s\n", issue.Fields.Project.Name)
	result += fmt.Sprintf("- Type: %s\n", issue.Fields.IssueType.Name)
	result += fmt.Sprintf("- Status: %s\n", issue.Fields.Status.Name)
	
	if issue.Fields.Assignee != nil {
		result += fmt.Sprintf("- Assignee: %s\n", issue.Fields.Assignee.DisplayName)
	} else {
		result += "- Assignee: Unassigned\n"
	}
	
	if issue.Fields.Priority != nil {
		result += fmt.Sprintf("- Priority: %s\n", issue.Fields.Priority.Name)
	}
	
	if len(issue.Fields.Labels) > 0 {
		result += fmt.Sprintf("- Labels: %v\n", issue.Fields.Labels)
	}
	
	if params.Description != "" {
		result += fmt.Sprintf("\n**Description:**\n%s\n", params.Description)
	}
	
	result += fmt.Sprintf("\n**View in Jira:** %s/browse/%s", t.client.BaseURL(), issue.Key)

	return NewTextResponse(result), nil
}
