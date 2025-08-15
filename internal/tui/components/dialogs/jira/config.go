package jira

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/v2/key"
	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
	"github.com/pelletier/go-toml/v2"
)

const (
	JiraConfigDialogID dialogs.DialogID = "jira-config"
)

type JiraConfigDialog interface {
	dialogs.DialogModel
}

type jiraConfigDialogModel struct {
	wWidth  int
	wHeight int

	inputs       []textinput.Model
	focusIndex   int
	keymap       ConfigKeyMap
	errorMessage string
}

type ConfigKeyMap struct {
	Next   key.Binding
	Prev   key.Binding
	Submit key.Binding
	Cancel key.Binding
}

func DefaultConfigKeymap() ConfigKeyMap {
	return ConfigKeyMap{
		Next: key.NewBinding(
			key.WithKeys("tab", "down"),
			key.WithHelp("tab/↓", "next field"),
		),
		Prev: key.NewBinding(
			key.WithKeys("shift+tab", "up"),
			key.WithHelp("shift+tab/↑", "previous field"),
		),
		Submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "save & continue"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
	}
}

// JiraConfig represents the Jira configuration
type JiraConfig struct {
	Jira struct {
		JiraURL            string `toml:"jira_url"`
		InstallationType   string `toml:"installation_type"`
		JiraToken          string `toml:"jira_token"`
		JiraUsername       string `toml:"jira_username"`
		JQL                string `toml:"jql"`
		FallbackComment    string `toml:"fallback_comment"`
	} `toml:"jira"`
}

// JiraConfigSavedMsg is sent when configuration is saved
type JiraConfigSavedMsg struct {
	BaseURL  string
	Email    string
	APIToken string
	JQL      string
}

func NewJiraConfigDialog() JiraConfigDialog {
	// Create text inputs
	inputs := make([]textinput.Model, 4)
	
	// Jira URL input
	inputs[0] = textinput.New()
	inputs[0].Placeholder = "https://your-domain.atlassian.net/"
	inputs[0].Focus()
	inputs[0].CharLimit = 256
	inputs[0].SetWidth(50)
	inputs[0].Prompt = ""
	
	// Username/Email input
	inputs[1] = textinput.New()
	inputs[1].Placeholder = "your-email@example.com"
	inputs[1].CharLimit = 256
	inputs[1].SetWidth(50)
	inputs[1].Prompt = ""
	
	// API Token input
	inputs[2] = textinput.New()
	inputs[2].Placeholder = "Your Jira API token"
	inputs[2].CharLimit = 512
	inputs[2].SetWidth(50)
	inputs[2].EchoMode = textinput.EchoPassword
	inputs[2].EchoCharacter = '•'
	inputs[2].Prompt = ""
	
	// JQL input
	inputs[3] = textinput.New()
	inputs[3].Placeholder = "assignee = currentUser() ORDER BY updated DESC"
	inputs[3].CharLimit = 512
	inputs[3].SetWidth(50)
	inputs[3].Prompt = ""
	
	// Pre-fill with helpful suggestions based on weedmaps config
	inputs[0].SetValue("https://weedmaps.atlassian.net/")
	inputs[3].SetValue("assignee = currentUser() AND updatedDate >= -30d ORDER BY updatedDate DESC")
	
	return &jiraConfigDialogModel{
		inputs:     inputs,
		focusIndex: 0,
		keymap:     DefaultConfigKeymap(),
	}
}

func (j *jiraConfigDialogModel) Init() tea.Cmd {
	return textinput.Blink
}

func (j *jiraConfigDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		j.wWidth = msg.Width
		j.wHeight = msg.Height
		
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, j.keymap.Cancel):
			return j, util.CmdHandler(dialogs.CloseDialogMsg{})
			
		case key.Matches(msg, j.keymap.Submit):
			// Validate inputs
			if j.inputs[0].Value() == "" {
				j.errorMessage = "🌿 Yo, need that Jira URL to connect to your stash"
				return j, nil
			}
			if j.inputs[1].Value() == "" {
				j.errorMessage = "🌿 Need your email to know who's rolling"
				return j, nil
			}
			if j.inputs[2].Value() == "" {
				j.errorMessage = "🌿 API token required for the good stuff"
				return j, nil
			}
			
			// Set default JQL if empty
			jql := j.inputs[3].Value()
			if jql == "" {
				jql = "assignee = currentUser() ORDER BY updated DESC"
			}
			
			// Save configuration
			if err := j.saveConfig(); err != nil {
				j.errorMessage = fmt.Sprintf("Failed to save config: %v", err)
				return j, nil
			}
			
			// Return success message and close dialog
			return j, tea.Sequence(
				util.CmdHandler(JiraConfigSavedMsg{
					BaseURL:  j.inputs[0].Value(),
					Email:    j.inputs[1].Value(),
					APIToken: j.inputs[2].Value(),
					JQL:      jql,
				}),
				util.CmdHandler(dialogs.CloseDialogMsg{}),
			)
			
		case key.Matches(msg, j.keymap.Next):
			j.focusIndex++
			if j.focusIndex >= len(j.inputs) {
				j.focusIndex = 0
			}
			j.updateFocus()
			return j, nil
			
		case key.Matches(msg, j.keymap.Prev):
			j.focusIndex--
			if j.focusIndex < 0 {
				j.focusIndex = len(j.inputs) - 1
			}
			j.updateFocus()
			return j, nil
		}
	}
	
	// Update the focused input
	cmd := j.updateInputs(msg)
	return j, cmd
}

func (j *jiraConfigDialogModel) updateFocus() {
	for i := range j.inputs {
		if i == j.focusIndex {
			j.inputs[i].Focus()
		} else {
			j.inputs[i].Blur()
		}
	}
}

func (j *jiraConfigDialogModel) updateInputs(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(j.inputs))
	for i := range j.inputs {
		j.inputs[i], cmds[i] = j.inputs[i].Update(msg)
	}
	return tea.Batch(cmds...)
}

func (j *jiraConfigDialogModel) saveConfig() error {
	// Create config directory if it doesn't exist
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "punchout")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	
	// Create config structure
	var config JiraConfig
	config.Jira.JiraURL = j.inputs[0].Value()
	config.Jira.InstallationType = "cloud"
	config.Jira.JiraUsername = j.inputs[1].Value()
	config.Jira.JiraToken = j.inputs[2].Value()
	config.Jira.JQL = j.inputs[3].Value()
	if config.Jira.JQL == "" {
		config.Jira.JQL = "assignee = currentUser() ORDER BY updated DESC"
	}
	config.Jira.FallbackComment = "comment"
	
	// Marshal to TOML
	data, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	// Write to file
	configPath := filepath.Join(configDir, "punchout.toml")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	
	// Also set environment variables for immediate use
	os.Setenv("JIRA_BASE_URL", config.Jira.JiraURL)
	os.Setenv("JIRA_EMAIL", config.Jira.JiraUsername)
	os.Setenv("JIRA_API_TOKEN", config.Jira.JiraToken)
	os.Setenv("JIRA_JQL", config.Jira.JQL)
	
	return nil
}

func (j *jiraConfigDialogModel) View() string {
	t := styles.CurrentTheme()
	
	dialogStyle := t.S().Base.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Accent).
		Padding(1, 2).
		Width(60)
	
	titleStyle := t.S().Title.
		Foreground(t.GreenDark).
		Bold(true).
		MarginBottom(1)
	
	labelStyle := t.S().Text.
		Foreground(t.GreenLight).
		Bold(true)
	
	errorStyle := t.S().Error.
		MarginTop(1)
	
	var content strings.Builder
	
	// Title
	content.WriteString(titleStyle.Render("🌿 Set Up Your Jira Garden"))
	content.WriteString("\n\n")
	
	// URL field
	content.WriteString(labelStyle.Render("Jira URL:"))
	content.WriteString("\n")
	content.WriteString(j.inputs[0].View())
	content.WriteString("\n\n")
	
	// Email field
	content.WriteString(labelStyle.Render("Email (your login):"))
	content.WriteString("\n")
	content.WriteString(j.inputs[1].View())
	content.WriteString("\n\n")
	
	// API Token field
	content.WriteString(labelStyle.Render("API Token:"))
	content.WriteString("\n")
	content.WriteString(j.inputs[2].View())
	content.WriteString("\n")
	content.WriteString(t.S().Subtle.Render("Get your token at: https://id.atlassian.com/manage/api-tokens"))
	content.WriteString("\n\n")
	
	// JQL field
	content.WriteString(labelStyle.Render("JQL Query (optional):"))
	content.WriteString("\n")
	content.WriteString(j.inputs[3].View())
	content.WriteString("\n")
	content.WriteString(t.S().Subtle.Render("Leave empty for default: your assigned issues"))
	
	// Error message if any
	if j.errorMessage != "" {
		content.WriteString("\n")
		content.WriteString(errorStyle.Render(j.errorMessage))
	}
	
	// Help text
	content.WriteString("\n\n")
	helpText := "Tab: Next field | Shift+Tab: Previous | Enter: Light it up | Esc: Pass"
	content.WriteString(t.S().Subtle.Render(helpText))
	
	return dialogStyle.Render(content.String())
}

func (j *jiraConfigDialogModel) dialogDimensions() (int, int) {
	return 60, 20
}

func (j *jiraConfigDialogModel) Position() (int, int) {
	w, h := j.dialogDimensions()
	row := (j.wHeight - h) / 2
	col := (j.wWidth - w) / 2
	return row, col
}

func (j *jiraConfigDialogModel) ID() dialogs.DialogID {
	return JiraConfigDialogID
}