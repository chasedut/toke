package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chasedut/toke/internal/integrations/jira"
	"github.com/chasedut/toke/internal/permission"
)

type JiraSearchParams struct {
	Query      string   `json:"query"`
	MaxResults int      `json:"max_results,omitempty"`
	Fields     []string `json:"fields,omitempty"`
}

type JiraSearchPermissionsParams struct {
	Query      string   `json:"query"`
	MaxResults int      `json:"max_results,omitempty"`
	Fields     []string `json:"fields,omitempty"`
}

type jiraSearchTool struct {
	client      *jira.Client
	permissions permission.Service
	workingDir  string
}

const (
	JiraSearchToolName        = "jira_search"
	jiraSearchToolDescription = `Searches Jira issues using JQL (Jira Query Language).

WHEN TO USE THIS TOOL:
| Use when you need to find Jira issues based on various criteria
| Helpful for understanding current work, finding related tickets, or getting project status
| Can search by assignee, status, project, labels, and more

HOW TO USE:
| Provide a JQL query to search for issues
| Optionally specify max_results (default 10, max 50) and fields to retrieve
| Common JQL examples:
  - "project = PROJ AND status = 'In Progress'"
  - "assignee = currentUser() AND created >= -7d"
  - "labels in (bug, critical) AND resolution is EMPTY"
  - "text ~ 'search term' ORDER BY created DESC"

FEATURES:
| Returns issue key, summary, status, assignee, and other requested fields
| Supports all standard JQL syntax and functions
| Results include links to view issues in Jira

LIMITATIONS:
| Requires Jira authentication to be configured
| Only returns issues the authenticated user has permission to view
| Complex queries may take longer to execute

TIPS:
| Use specific field names in JQL for better performance
| Add ORDER BY clauses to get most relevant results first
| Request only needed fields to reduce response size`
)

func NewJiraSearchTool(client *jira.Client, permissions permission.Service, workingDir string) BaseTool {
	return &jiraSearchTool{
		client:      client,
		permissions: permissions,
		workingDir:  workingDir,
	}
}

func (t *jiraSearchTool) Name() string {
	return JiraSearchToolName
}

func (t *jiraSearchTool) Info() ToolInfo {
	return ToolInfo{
		Name:        JiraSearchToolName,
		Description: jiraSearchToolDescription,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "JQL query to search for issues",
				},
				"max_results": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of results to return (default 10, max 50)",
					"minimum":     1,
					"maximum":     50,
				},
				"fields": map[string]interface{}{
					"type":        "array",
					"description": "List of fields to retrieve for each issue",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
		Required: []string{"query"},
	}
}

func (t *jiraSearchTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params JiraSearchParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse("Failed to parse parameters: " + err.Error()), nil
	}

	if params.Query == "" {
		return NewTextErrorResponse("Query parameter is required"), nil
	}

	// Set defaults
	if params.MaxResults <= 0 {
		params.MaxResults = 10
	} else if params.MaxResults > 50 {
		params.MaxResults = 50
	}

	sessionID, _ := GetContextValues(ctx)
	if sessionID == "" {
		return ToolResponse{}, fmt.Errorf("session ID is required for Jira search")
	}

	p := t.permissions.Request(
		permission.CreatePermissionRequest{
			SessionID:   sessionID,
			Path:        t.workingDir,
			ToolCallID:  call.ID,
			ToolName:    JiraSearchToolName,
			Action:      "search",
			Description: fmt.Sprintf("Search Jira issues with query: %s", params.Query),
			Params:      JiraSearchPermissionsParams(params),
		},
	)

	if !p {
		return ToolResponse{}, permission.ErrorPermissionDenied
	}

	// Search issues
	searchResp, err := t.client.SearchIssues(ctx, params.Query, params.Fields, 0, params.MaxResults)
	if err != nil {
		return ToolResponse{}, fmt.Errorf("failed to search Jira issues: %w", err)
	}

	// Format results
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Found %d issues (showing %d):\n\n", searchResp.Total, len(searchResp.Issues)))

	for _, issue := range searchResp.Issues {
		result.WriteString(fmt.Sprintf("**%s** - %s\n", issue.Key, issue.Fields.Summary))
		result.WriteString(fmt.Sprintf("  Status: %s\n", issue.Fields.Status.Name))
		
		if issue.Fields.Assignee != nil {
			result.WriteString(fmt.Sprintf("  Assignee: %s\n", issue.Fields.Assignee.DisplayName))
		} else {
			result.WriteString("  Assignee: Unassigned\n")
		}
		
		if issue.Fields.Priority != nil {
			result.WriteString(fmt.Sprintf("  Priority: %s\n", issue.Fields.Priority.Name))
		}
		
		result.WriteString(fmt.Sprintf("  Type: %s\n", issue.Fields.IssueType.Name))
		result.WriteString(fmt.Sprintf("  Project: %s\n", issue.Fields.Project.Name))
		
		if len(issue.Fields.Labels) > 0 {
			result.WriteString(fmt.Sprintf("  Labels: %s\n", strings.Join(issue.Fields.Labels, ", ")))
		}
		
		result.WriteString(fmt.Sprintf("  Created: %s\n", issue.Fields.Created.Format("2006-01-02 15:04")))
		result.WriteString(fmt.Sprintf("  Updated: %s\n", issue.Fields.Updated.Format("2006-01-02 15:04")))
		
		// Add description preview if available
		if issue.Fields.Description != nil {
			descStr := fmt.Sprintf("%v", issue.Fields.Description)
			if len(descStr) > 200 {
				descStr = descStr[:197] + "..."
			}
			if descStr != "" && descStr != "<nil>" {
				result.WriteString(fmt.Sprintf("  Description: %s\n", descStr))
			}
		}
		
		result.WriteString(fmt.Sprintf("  Link: %s/browse/%s\n", t.client.BaseURL(), issue.Key))
		result.WriteString("\n")
	}

	if searchResp.Total > len(searchResp.Issues) {
		result.WriteString(fmt.Sprintf("Note: %d more issues match your query. Refine your search or increase max_results.\n", 
			searchResp.Total - len(searchResp.Issues)))
	}

	return NewTextResponse(result.String()), nil
}
