package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chasedut/toke/internal/integrations/jira"
	"github.com/chasedut/toke/internal/permission"
)

type JiraUpdateParams struct {
	IssueKey string                 `json:"issue_key"`
	Fields   map[string]interface{} `json:"fields,omitempty"`
	Comment  string                 `json:"comment,omitempty"`
}

type JiraUpdatePermissionsParams struct {
	IssueKey string                 `json:"issue_key"`
	Fields   map[string]interface{} `json:"fields,omitempty"`
	Comment  string                 `json:"comment,omitempty"`
}

type jiraUpdateTool struct {
	client      *jira.Client
	permissions permission.Service
	workingDir  string
}

const (
	JiraUpdateToolName        = "jira_update_ticket"
	jiraUpdateToolDescription = `Updates an existing Jira issue's fields and/or adds a comment.

WHEN TO USE THIS TOOL:
| Use when you need to update a Jira ticket's status, assignee, or other fields
| Helpful for tracking progress, updating descriptions, or adding comments
| Can update multiple fields in a single operation

HOW TO USE:
| Provide the issue key (e.g., "PROJ-123")
| Specify fields to update as key-value pairs
| Optionally add a comment to explain the changes

COMMON FIELD UPDATES:
| summary: "New title"
| description: "Updated description"
| assignee: {"accountId": "user-id"} or null to unassign
| priority: {"id": "priority-id"}
| labels: ["label1", "label2"]
| status: Use jira_transition_ticket to change status
| customfield_XXXXX: Custom field values

FEATURES:
| Updates multiple fields atomically
| Can add a comment in the same operation
| Validates field values before updating

LIMITATIONS:
| Cannot change issue type or project
| Status changes require using jira_transition_ticket
| Some fields may be read-only based on workflow

TIPS:
| Use jira_get_ticket first to see current field values
| Check field metadata if updates fail
| Add comments to document why changes were made`
)

func NewJiraUpdateTool(client *jira.Client, permissions permission.Service, workingDir string) BaseTool {
	return &jiraUpdateTool{
		client:      client,
		permissions: permissions,
		workingDir:  workingDir,
	}
}

func (t *jiraUpdateTool) Name() string {
	return JiraUpdateToolName
}

func (t *jiraUpdateTool) Info() ToolInfo {
	return ToolInfo{
		Name:        JiraUpdateToolName,
		Description: jiraUpdateToolDescription,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"issue_key": map[string]interface{}{
					"type":        "string",
					"description": "The issue key (e.g., 'PROJ-123')",
				},
				"fields": map[string]interface{}{
					"type":        "object",
					"description": "Fields to update as key-value pairs",
				},
				"comment": map[string]interface{}{
					"type":        "string",
					"description": "Optional comment to add with the update",
				},
			},
		},
		Required: []string{"issue_key"},
	}
}

func (t *jiraUpdateTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params JiraUpdateParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse("Failed to parse parameters: " + err.Error()), nil
	}

	if params.IssueKey == "" {
		return NewTextErrorResponse("issue_key parameter is required"), nil
	}

	if len(params.Fields) == 0 && params.Comment == "" {
		return NewTextErrorResponse("Either fields or comment must be provided"), nil
	}

	sessionID, _ := GetContextValues(ctx)
	if sessionID == "" {
		return ToolResponse{}, fmt.Errorf("session ID is required for Jira operations")
	}

	p := t.permissions.Request(
		permission.CreatePermissionRequest{
			SessionID:   sessionID,
			Path:        t.workingDir,
			ToolCallID:  call.ID,
			ToolName:    JiraUpdateToolName,
			Action:      "update",
			Description: fmt.Sprintf("Update Jira issue: %s", params.IssueKey),
			Params:      JiraUpdatePermissionsParams(params),
		},
	)

	if !p {
		return ToolResponse{}, permission.ErrorPermissionDenied
	}

	// Update fields if provided
	if len(params.Fields) > 0 {
		updateReq := jira.UpdateIssueRequest{
			Fields: params.Fields,
		}
		
		if err := t.client.UpdateIssue(ctx, params.IssueKey, updateReq); err != nil {
			return ToolResponse{}, fmt.Errorf("failed to update issue: %w", err)
		}
	}

	// Add comment if provided
	if params.Comment != "" {
		if _, err := t.client.AddComment(ctx, params.IssueKey, params.Comment); err != nil {
			return ToolResponse{}, fmt.Errorf("failed to add comment: %w", err)
		}
	}

	// Get the updated issue to show results
	issue, err := t.client.GetIssue(ctx, params.IssueKey, nil)
	if err != nil {
		// Update succeeded but couldn't fetch updated issue
		return NewTextResponse(fmt.Sprintf("Successfully updated %s", params.IssueKey)), nil
	}

	// Format response
	result := fmt.Sprintf("Successfully updated **%s** - %s\n\n", issue.Key, issue.Fields.Summary)
	
	if len(params.Fields) > 0 {
		result += "**Updated fields:**\n"
		for field := range params.Fields {
			result += fmt.Sprintf("- %s\n", field)
		}
		result += "\n"
	}
	
	if params.Comment != "" {
		result += fmt.Sprintf("**Added comment:** %s\n\n", params.Comment)
	}
	
	result += fmt.Sprintf("**Current status:** %s\n", issue.Fields.Status.Name)
	if issue.Fields.Assignee != nil {
		result += fmt.Sprintf("**Assignee:** %s\n", issue.Fields.Assignee.DisplayName)
	} else {
		result += "**Assignee:** Unassigned\n"
	}
	
	result += fmt.Sprintf("\n**View in Jira:** %s/browse/%s", t.client.BaseURL(), issue.Key)

	return NewTextResponse(result), nil
}
