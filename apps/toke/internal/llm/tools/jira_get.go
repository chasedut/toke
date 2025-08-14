package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chasedut/toke/internal/integrations/jira"
	"github.com/chasedut/toke/internal/permission"
)

type JiraGetParams struct {
	IssueKey string   `json:"issue_key"`
	Fields   []string `json:"fields,omitempty"`
}

type JiraGetPermissionsParams struct {
	IssueKey string   `json:"issue_key"`
	Fields   []string `json:"fields,omitempty"`
}

type jiraGetTool struct {
	client      *jira.Client
	permissions permission.Service
	workingDir  string
}

const (
	JiraGetToolName        = "jira_get_ticket"
	jiraGetToolDescription = `Retrieves detailed information about a specific Jira issue.

WHEN TO USE THIS TOOL:
| Use when you need detailed information about a specific Jira ticket
| Helpful for understanding issue context, checking current status, or viewing comments
| Can retrieve standard and custom fields

HOW TO USE:
| Provide the issue key (e.g., "PROJ-123")
| Optionally specify which fields to retrieve (defaults to common fields)

FEATURES:
| Returns comprehensive issue details including description, comments, and metadata
| Supports custom fields specific to your Jira instance
| Includes links to view the issue in Jira

LIMITATIONS:
| Requires Jira authentication to be configured
| Only returns issues the authenticated user has permission to view
| Large issues with many comments may take longer to retrieve

TIPS:
| Request only needed fields to improve performance
| Use this after jira_search to get full details of relevant issues
| Check the description and comments for important context`
)

func NewJiraGetTool(client *jira.Client, permissions permission.Service, workingDir string) BaseTool {
	return &jiraGetTool{
		client:      client,
		permissions: permissions,
		workingDir:  workingDir,
	}
}

func (t *jiraGetTool) Name() string {
	return JiraGetToolName
}

func (t *jiraGetTool) Info() ToolInfo {
	return ToolInfo{
		Name:        JiraGetToolName,
		Description: jiraGetToolDescription,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"issue_key": map[string]interface{}{
					"type":        "string",
					"description": "The issue key (e.g., 'PROJ-123')",
				},
				"fields": map[string]interface{}{
					"type":        "array",
					"description": "List of fields to retrieve (optional)",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
		Required: []string{"issue_key"},
	}
}

func (t *jiraGetTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params JiraGetParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse("Failed to parse parameters: " + err.Error()), nil
	}

	if params.IssueKey == "" {
		return NewTextErrorResponse("issue_key parameter is required"), nil
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
			ToolName:    JiraGetToolName,
			Action:      "read",
			Description: fmt.Sprintf("Get Jira issue: %s", params.IssueKey),
			Params:      JiraGetPermissionsParams(params),
		},
	)

	if !p {
		return ToolResponse{}, permission.ErrorPermissionDenied
	}

	// Get the issue
	issue, err := t.client.GetIssue(ctx, params.IssueKey, params.Fields)
	if err != nil {
		return ToolResponse{}, fmt.Errorf("failed to get Jira issue: %w", err)
	}

	// Get comments
	commentsResp, err := t.client.GetComments(ctx, params.IssueKey)
	if err != nil {
		// Non-fatal: continue without comments
		commentsResp = &jira.CommentResponse{}
	}

	// Format the response
	var result strings.Builder
	result.WriteString(fmt.Sprintf("# %s - %s\n\n", issue.Key, issue.Fields.Summary))
	
	// Basic info
	result.WriteString("## Details\n")
	result.WriteString(fmt.Sprintf("- **Status**: %s\n", issue.Fields.Status.Name))
	result.WriteString(fmt.Sprintf("- **Type**: %s\n", issue.Fields.IssueType.Name))
	result.WriteString(fmt.Sprintf("- **Project**: %s (%s)\n", issue.Fields.Project.Name, issue.Fields.Project.Key))
	
	if issue.Fields.Priority != nil {
		result.WriteString(fmt.Sprintf("- **Priority**: %s\n", issue.Fields.Priority.Name))
	}
	
	if issue.Fields.Assignee != nil {
		result.WriteString(fmt.Sprintf("- **Assignee**: %s\n", issue.Fields.Assignee.DisplayName))
	} else {
		result.WriteString("- **Assignee**: Unassigned\n")
	}
	
	if issue.Fields.Reporter != nil {
		result.WriteString(fmt.Sprintf("- **Reporter**: %s\n", issue.Fields.Reporter.DisplayName))
	}
	
	result.WriteString(fmt.Sprintf("- **Created**: %s\n", issue.Fields.Created.Format("2006-01-02 15:04")))
	result.WriteString(fmt.Sprintf("- **Updated**: %s\n", issue.Fields.Updated.Format("2006-01-02 15:04")))
	
	if len(issue.Fields.Labels) > 0 {
		result.WriteString(fmt.Sprintf("- **Labels**: %s\n", strings.Join(issue.Fields.Labels, ", ")))
	}
	
	if len(issue.Fields.Components) > 0 {
		compNames := make([]string, len(issue.Fields.Components))
		for i, comp := range issue.Fields.Components {
			compNames[i] = comp.Name
		}
		result.WriteString(fmt.Sprintf("- **Components**: %s\n", strings.Join(compNames, ", ")))
	}
	
	if len(issue.Fields.FixVersions) > 0 {
		versionNames := make([]string, len(issue.Fields.FixVersions))
		for i, ver := range issue.Fields.FixVersions {
			versionNames[i] = ver.Name
		}
		result.WriteString(fmt.Sprintf("- **Fix Versions**: %s\n", strings.Join(versionNames, ", ")))
	}
	
	// Description
	result.WriteString("\n## Description\n")
	if issue.Fields.Description != nil {
		descStr := formatJiraContent(issue.Fields.Description)
		if descStr != "" {
			result.WriteString(descStr)
		} else {
			result.WriteString("*No description*")
		}
	} else {
		result.WriteString("*No description*")
	}
	result.WriteString("\n\n")
	
	// Comments
	if len(commentsResp.Comments) > 0 {
		result.WriteString(fmt.Sprintf("## Comments (%d)\n\n", len(commentsResp.Comments)))
		for i, comment := range commentsResp.Comments {
			result.WriteString(fmt.Sprintf("### Comment %d - %s (%s)\n", 
				i+1, 
				comment.Author.DisplayName,
				comment.Created.Format("2006-01-02 15:04")))
			result.WriteString(formatJiraContent(comment.Body))
			result.WriteString("\n\n")
		}
	}
	
	// Link
	result.WriteString(fmt.Sprintf("\n**View in Jira**: %s/browse/%s\n", t.client.BaseURL(), issue.Key))

	return NewTextResponse(result.String()), nil
}

// formatJiraContent converts Jira content (string or ADF) to readable text
func formatJiraContent(content interface{}) string {
	if content == nil {
		return ""
	}
	
	// If it's already a string, return it
	if str, ok := content.(string); ok {
		return str
	}
	
	// If it's ADF (Atlassian Document Format), try to extract text
	// This is a simplified extraction - a full implementation would parse the ADF tree
	if contentMap, ok := content.(map[string]interface{}); ok {
		if contentArray, ok := contentMap["content"].([]interface{}); ok {
			var texts []string
			for _, node := range contentArray {
				if nodeMap, ok := node.(map[string]interface{}); ok {
					texts = append(texts, extractTextFromADFNode(nodeMap))
				}
			}
			return strings.Join(texts, "\n")
		}
	}
	
	// Fallback: convert to string
	return fmt.Sprintf("%v", content)
}

// extractTextFromADFNode extracts text from an ADF node (simplified)
func extractTextFromADFNode(node map[string]interface{}) string {
	nodeType, _ := node["type"].(string)
	
	switch nodeType {
	case "paragraph", "heading":
		if content, ok := node["content"].([]interface{}); ok {
			var texts []string
			for _, child := range content {
				if childMap, ok := child.(map[string]interface{}); ok {
					texts = append(texts, extractTextFromADFNode(childMap))
				}
			}
			return strings.Join(texts, "")
		}
	case "text":
		if text, ok := node["text"].(string); ok {
			return text
		}
	case "hardBreak":
		return "\n"
	}
	
	// Recursively process content
	if content, ok := node["content"].([]interface{}); ok {
		var texts []string
		for _, child := range content {
			if childMap, ok := child.(map[string]interface{}); ok {
				texts = append(texts, extractTextFromADFNode(childMap))
			}
		}
		return strings.Join(texts, "\n")
	}
	
	return ""
}
