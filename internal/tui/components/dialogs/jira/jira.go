package jira

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/v2/key"
	"github.com/charmbracelet/bubbles/v2/list"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/api/jira"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
)

const (
	JiraDialogID dialogs.DialogID = "jira"
)

type JiraIssuesDialog interface {
	dialogs.DialogModel
}

type jiraDialogModel struct {
	wWidth  int
	wHeight int

	list      list.Model
	issues    []jira.Issue
	jiraClient *jira.Client
	keymap    KeyMap
	
	// For returning content to chat
	selectedIssue *jira.Issue
	includeMetadata bool
}

// JiraItem implements list.Item
type JiraItem struct {
	issue jira.Issue
}

func (i JiraItem) FilterValue() string {
	return i.issue.Key + " " + i.issue.Fields.Summary
}

func (i JiraItem) Title() string {
	return fmt.Sprintf("%s - %s", i.issue.Key, i.issue.Fields.Summary)
}

func (i JiraItem) Description() string {
	status := i.issue.Fields.Status.Name
	assignee := "Unassigned"
	if i.issue.Fields.Assignee != nil {
		assignee = i.issue.Fields.Assignee.DisplayName
	}
	return fmt.Sprintf("Status: %s | Assignee: %s", status, assignee)
}

// Message sent when issue is selected
type JiraIssueSelectedMsg struct {
	Issue           jira.Issue
	IncludeMetadata bool
}

func NewJiraDialog(jiraClient *jira.Client, jql string) (JiraIssuesDialog, error) {
	searchResult, err := jiraClient.SearchIssues(jql, 50)
	if err != nil {
		return nil, err
	}

	items := make([]list.Item, len(searchResult.Issues))
	for i, issue := range searchResult.Issues {
		items[i] = JiraItem{issue: issue}
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Jira Issues"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)

	return &jiraDialogModel{
		list:       l,
		issues:     searchResult.Issues,
		jiraClient: jiraClient,
		keymap:     DefaultKeymap(),
	}, nil
}

func (j *jiraDialogModel) Init() tea.Cmd {
	return nil
}

func (j *jiraDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		j.wWidth = msg.Width
		j.wHeight = msg.Height
		
		// Update list dimensions
		w, h := j.dialogDimensions()
		j.list.SetSize(w-4, h-4) // Account for borders and padding
		
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, j.keymap.Close):
			return j, util.CmdHandler(dialogs.CloseDialogMsg{})
			
		case key.Matches(msg, j.keymap.SelectWithMetadata):
			// Shift+Enter - include metadata
			if item, ok := j.list.SelectedItem().(JiraItem); ok {
				j.selectedIssue = &item.issue
				j.includeMetadata = true
				return j, tea.Sequence(
					util.CmdHandler(dialogs.CloseDialogMsg{}),
					util.CmdHandler(JiraIssueSelectedMsg{
						Issue:           item.issue,
						IncludeMetadata: true,
					}),
				)
			}
			
		case key.Matches(msg, j.keymap.Select):
			// Regular Enter - basic info only
			if item, ok := j.list.SelectedItem().(JiraItem); ok {
				j.selectedIssue = &item.issue
				j.includeMetadata = false
				return j, tea.Sequence(
					util.CmdHandler(dialogs.CloseDialogMsg{}),
					util.CmdHandler(JiraIssueSelectedMsg{
						Issue:           item.issue,
						IncludeMetadata: false,
					}),
				)
			}
		}
	}

	// Update the list
	var cmd tea.Cmd
	j.list, cmd = j.list.Update(msg)
	return j, cmd
}

func (j *jiraDialogModel) View() string {
	t := styles.CurrentTheme()
	
	dialogStyle := t.S().Base.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocus).
		Padding(1)

	helpText := "Enter: Add to chat | Shift+Enter: Add with metadata | Esc: Cancel"
	help := t.S().Subtle.Render(helpText)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		j.list.View(),
		"",
		help,
	)

	return dialogStyle.Render(content)
}

func (j *jiraDialogModel) dialogDimensions() (int, int) {
	width := min(80, j.wWidth-10)
	height := min(30, j.wHeight-6)
	return width, height
}

func (j *jiraDialogModel) Position() (int, int) {
	w, h := j.dialogDimensions()
	row := (j.wHeight - h) / 2
	col := (j.wWidth - w) / 2
	return row, col
}

func (j *jiraDialogModel) ID() dialogs.DialogID {
	return JiraDialogID
}

// FormatIssueForChat formats a Jira issue for insertion into chat
func FormatIssueForChat(issue jira.Issue, includeMetadata bool) string {
	var sb strings.Builder
	
	// Always include title and description
	sb.WriteString(fmt.Sprintf("**%s** - %s\n\n", issue.Key, issue.Fields.Summary))
	
	if issue.Fields.Description != nil {
		desc := issue.Fields.Description.ToPlainText()
		if desc != "" {
			sb.WriteString("**Description:**\n")
			sb.WriteString(desc)
			sb.WriteString("\n\n")
		}
	}
	
	// Add images/attachments info if present
	if len(issue.Fields.Attachments) > 0 {
		sb.WriteString("**Attachments:**\n")
		for _, att := range issue.Fields.Attachments {
			sb.WriteString(fmt.Sprintf("- %s (%s)\n", att.Filename, att.MimeType))
		}
		sb.WriteString("\n")
	}
	
	// Add metadata if requested
	if includeMetadata {
		sb.WriteString("**Metadata:**\n")
		sb.WriteString(fmt.Sprintf("- Type: %s\n", issue.Fields.IssueType.Name))
		sb.WriteString(fmt.Sprintf("- Status: %s\n", issue.Fields.Status.Name))
		
		if issue.Fields.Assignee != nil {
			sb.WriteString(fmt.Sprintf("- Assignee: %s\n", issue.Fields.Assignee.DisplayName))
		}
		
		if issue.Fields.Priority != nil {
			sb.WriteString(fmt.Sprintf("- Priority: %s\n", issue.Fields.Priority.Name))
		}
		
		if len(issue.Fields.Labels) > 0 {
			sb.WriteString(fmt.Sprintf("- Labels: %s\n", strings.Join(issue.Fields.Labels, ", ")))
		}
		
		sb.WriteString(fmt.Sprintf("- Created: %s\n", issue.Fields.Created))
		sb.WriteString(fmt.Sprintf("- Updated: %s\n", issue.Fields.Updated))
		sb.WriteString("\n")
		
		// Add comments if present
		if issue.Fields.Comments != nil && len(issue.Fields.Comments.Comments) > 0 {
			sb.WriteString("**Comments:**\n")
			for i, comment := range issue.Fields.Comments.Comments {
				if i >= 5 { // Limit to first 5 comments
					sb.WriteString(fmt.Sprintf("... and %d more comments\n", len(issue.Fields.Comments.Comments)-5))
					break
				}
				sb.WriteString(fmt.Sprintf("- %s (%s):\n", comment.Author.DisplayName, comment.Created))
				if comment.Body != nil {
					commentText := comment.Body.ToPlainText()
					if len(commentText) > 200 {
						commentText = commentText[:200] + "..."
					}
					sb.WriteString(fmt.Sprintf("  %s\n", commentText))
				}
			}
		}
	}
	
	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
