package jira

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/config"
	"github.com/chasedut/toke/internal/integrations/jira"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
)

const (
	JiraAuthDialogID dialogs.DialogID = "jira-auth"
)

type JiraAuthDialog struct {
	width       int
	height      int
	focusIndex  int
	inputs      []textinput.Model
	authMethod  string // "basic" or "oauth"
	err         error
	testing     bool
	testSuccess bool
}

type JiraAuthSuccessMsg struct {
	Config config.JiraConfig
}

func NewJiraAuthDialog() *JiraAuthDialog {
	inputs := make([]textinput.Model, 3)

	// Jira URL input
	inputs[0] = textinput.New()
	inputs[0].Placeholder = "https://yourcompany.atlassian.net"
	inputs[0].Focus()
	inputs[0].CharLimit = 200
	inputs[0].SetWidth(50)

	// Email input (for basic auth)
	inputs[1] = textinput.New()
	inputs[1].Placeholder = "your.email@company.com"
	inputs[1].CharLimit = 100
	inputs[1].SetWidth(50)

	// API Token input
	inputs[2] = textinput.New()
	inputs[2].Placeholder = "Enter your API token"
	inputs[2].CharLimit = 200
	inputs[2].SetWidth(50)
	inputs[2].EchoMode = textinput.EchoPassword

	return &JiraAuthDialog{
		inputs:     inputs,
		authMethod: "basic",
		focusIndex: 0,
	}
}

func (d *JiraAuthDialog) Init() tea.Cmd {
	return textinput.Blink
}

func (d *JiraAuthDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab":
			// Move focus
			if msg.String() == "tab" {
				d.focusIndex = (d.focusIndex + 1) % len(d.inputs)
			} else {
				d.focusIndex = (d.focusIndex - 1 + len(d.inputs)) % len(d.inputs)
			}

			// Update focus
			for i := range d.inputs {
				if i == d.focusIndex {
					d.inputs[i].Focus()
				} else {
					d.inputs[i].Blur()
				}
			}
			return d, nil

		case "enter":
			// Validate inputs
			url := strings.TrimSpace(d.inputs[0].Value())
			email := strings.TrimSpace(d.inputs[1].Value())
			token := strings.TrimSpace(d.inputs[2].Value())

			if url == "" {
				d.err = fmt.Errorf("jira URL is required")
				return d, nil
			}

			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
				d.err = fmt.Errorf("jira URL must start with http:// or https://")
				return d, nil
			}

			if email == "" {
				d.err = fmt.Errorf("email is required")
				return d, nil
			}

			if token == "" {
				d.err = fmt.Errorf("API token is required")
				return d, nil
			}

			// Test connection
			d.testing = true
			d.err = nil
			return d, d.testConnection(url, email, token)

		case "esc":
			return d, util.CmdHandler(dialogs.CloseDialogMsg{})
		}

	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
		return d, nil
	}

	// Handle text input updates
	var cmd tea.Cmd
	d.inputs[d.focusIndex], cmd = d.inputs[d.focusIndex].Update(msg)
	return d, cmd
}

func (d *JiraAuthDialog) testConnection(url, email, token string) tea.Cmd {
	return func() tea.Msg {
		auth := jira.BasicAuth{
			Email:    email,
			APIToken: token,
		}

		client, err := jira.NewClient(url, auth)
		if err != nil {
			d.err = err
			d.testing = false
			return nil
		}

		ctx := context.Background()
		if err := client.TestConnection(ctx); err != nil {
			d.err = fmt.Errorf("connection failed: %w", err)
			d.testing = false
			return nil
		}

		d.testing = false
		d.testSuccess = true

		// Save configuration
		jiraConfig := config.JiraConfig{
			URL:      url,
			Email:    email,
			APIToken: token,
			Enabled:  true,
		}

		// Return success message
		return tea.Batch(
			util.CmdHandler(dialogs.CloseDialogMsg{}),
			util.CmdHandler(JiraAuthSuccessMsg{Config: jiraConfig}),
		)()
	}
}

func (d *JiraAuthDialog) View() string {
	if d.width == 0 || d.height == 0 {
		return ""
	}

	t := styles.CurrentTheme()

	var content strings.Builder

	// Title
	title := t.S().Base.Bold(true).Foreground(t.Primary).Render("🔧 Jira Configuration")
	content.WriteString(lipgloss.NewStyle().Width(d.width).Align(lipgloss.Center).Render(title))
	content.WriteString("\n\n")

	// Instructions
	if d.authMethod == "basic" {
		instructions := []string{
			"Configure Jira with API token authentication.",
			"",
			"To create an API token:",
			"1. Go to https://id.atlassian.com/manage-profile/security/api-tokens",
			"2. Click 'Create API token'",
			"3. Give it a name and copy the token",
			"",
		}

		for _, line := range instructions {
			if line == "" {
				content.WriteString("\n")
			} else if strings.HasPrefix(line, "To create") {
				content.WriteString(t.S().Base.Bold(true).Render(line))
				content.WriteString("\n")
			} else {
				content.WriteString(t.S().Base.Foreground(t.FgMuted).Render(line))
				content.WriteString("\n")
			}
		}
	}

	// Input fields
	labels := []string{"Jira URL:", "Email:", "API Token:"}
	for i, label := range labels {
		labelStyle := t.S().Base.Bold(true)
		if i == d.focusIndex {
			labelStyle = labelStyle.Foreground(t.Primary)
		}
		content.WriteString(labelStyle.Render(label))
		content.WriteString("\n")
		content.WriteString(d.inputs[i].View())
		content.WriteString("\n\n")
	}

	// Status
	if d.testing {
		content.WriteString(t.S().Base.Foreground(t.Primary).Render("🔄 Testing connection..."))
		content.WriteString("\n\n")
	} else if d.testSuccess {
		content.WriteString(t.S().Base.Foreground(t.Green).Render("✅ Connection successful!"))
		content.WriteString("\n\n")
	} else if d.err != nil {
		errMsg := t.S().Base.Foreground(t.Error).Render("❌ " + d.err.Error())
		content.WriteString(errMsg)
		content.WriteString("\n\n")
	}

	// Footer
	footer := t.S().Base.Foreground(t.FgHalfMuted).Render("Tab to navigate • Enter to test • Esc to cancel")
	content.WriteString(footer)

	// Dialog styling
	dialogStyle := t.S().Base.
		Width(70).
		Padding(2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border)

	dialog := dialogStyle.Render(content.String())

	// Center in viewport
	return lipgloss.Place(
		d.width,
		d.height,
		lipgloss.Center,
		lipgloss.Center,
		dialog,
	)
}

func (d *JiraAuthDialog) SetSize(width, height int) tea.Cmd {
	d.width = width
	d.height = height
	return nil
}

func (d *JiraAuthDialog) ID() dialogs.DialogID {
	return JiraAuthDialogID
}

func (d *JiraAuthDialog) Position() (int, int) {
	return 0, 0
}
