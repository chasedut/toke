package github

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/v2/key"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
)

const (
	GitHubAuthDialogID dialogs.DialogID = "github-auth"
)

type GitHubAuthDialog interface {
	dialogs.DialogModel
}

type githubAuthDialogModel struct {
	wWidth  int
	wHeight int
	
	// State
	ghInstalled      bool
	ghAuthenticated  bool
	copilotAuthed    bool
	currentStep      int
	errorMessage     string
	
	keymap AuthKeyMap
}

type AuthKeyMap struct {
	Proceed key.Binding
	Cancel  key.Binding
}

func DefaultAuthKeymap() AuthKeyMap {
	return AuthKeyMap{
		Proceed: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "proceed"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
	}
}

// GitHubAuthCompleteMsg is sent when authentication is complete
type GitHubAuthCompleteMsg struct {
	CopilotEnabled bool
}

// CheckGitHubAuthMsg triggers a check of GitHub auth status
type CheckGitHubAuthMsg struct{}

func NewGitHubAuthDialog() GitHubAuthDialog {
	m := &githubAuthDialogModel{
		keymap:      DefaultAuthKeymap(),
		currentStep: 0,
	}
	
	// Check initial state
	m.checkGHStatus()
	
	return m
}

func (g *githubAuthDialogModel) checkGHStatus() {
	// Check if gh is installed
	if _, err := exec.LookPath("gh"); err == nil {
		g.ghInstalled = true
		
		// Check if authenticated
		cmd := exec.Command("gh", "auth", "status")
		if err := cmd.Run(); err == nil {
			g.ghAuthenticated = true
		}
		
		// Check if Copilot is authenticated
		cmd = exec.Command("gh", "auth", "status", "--hostname", "github.com")
		output, _ := cmd.CombinedOutput()
		if strings.Contains(string(output), "✓ Logged in to github.com") {
			// Check Copilot extension
			cmd = exec.Command("gh", "extension", "list")
			output, _ = cmd.CombinedOutput()
			if strings.Contains(string(output), "copilot") {
				g.copilotAuthed = true
			}
		}
	}
}

func (g *githubAuthDialogModel) Init() tea.Cmd {
	return nil
}

func (g *githubAuthDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		g.wWidth = msg.Width
		g.wHeight = msg.Height
		
	case CheckGitHubAuthMsg:
		g.checkGHStatus()
		return g, nil
		
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, g.keymap.Cancel):
			return g, util.CmdHandler(dialogs.CloseDialogMsg{})
			
		case key.Matches(msg, g.keymap.Proceed):
			return g.handleProceed()
		}
	}
	
	return g, nil
}

func (g *githubAuthDialogModel) handleProceed() (tea.Model, tea.Cmd) {
	if !g.ghInstalled {
		// Open browser to install gh
		return g, tea.Sequence(
			g.runCommand("open", "https://cli.github.com/"),
			util.CmdHandler(util.InfoMsg{
				Type: util.InfoTypeInfo,
				Msg:  "🌿 Opening browser to install GitHub CLI... Come back when you're lifted!",
			}),
		)
	}
	
	if !g.ghAuthenticated {
		// Run gh auth login
		return g, tea.Sequence(
			g.runCommand("gh", "auth", "login"),
			func() tea.Msg {
				g.checkGHStatus()
				return CheckGitHubAuthMsg{}
			},
		)
	}
	
	if !g.copilotAuthed {
		// Install and auth Copilot
		return g, tea.Sequence(
			g.runCommand("gh", "extension", "install", "github/gh-copilot"),
			g.runCommand("gh", "copilot", "auth"),
			func() tea.Msg {
				g.checkGHStatus()
				return GitHubAuthCompleteMsg{CopilotEnabled: true}
			},
			util.CmdHandler(dialogs.CloseDialogMsg{}),
		)
	}
	
	// Everything is set up
	return g, tea.Sequence(
		util.CmdHandler(GitHubAuthCompleteMsg{CopilotEnabled: g.copilotAuthed}),
		util.CmdHandler(dialogs.CloseDialogMsg{}),
	)
}

func (g *githubAuthDialogModel) runCommand(name string, args ...string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command(name, args...)
		if err := cmd.Run(); err != nil {
			g.errorMessage = fmt.Sprintf("Failed to run %s: %v", name, err)
		}
		return CheckGitHubAuthMsg{}
	}
}

func (g *githubAuthDialogModel) View() string {
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
	
	statusStyle := t.S().Text.
		Foreground(t.GreenLight)
	
	errorStyle := t.S().Error.
		MarginTop(1)
	
	var content strings.Builder
	
	// Title
	content.WriteString(titleStyle.Render("🌿 GitHub & Copilot Setup"))
	content.WriteString("\n\n")
	
	// Status checks
	ghStatus := "❌ Not installed"
	if g.ghInstalled {
		ghStatus = "✅ Installed"
	}
	content.WriteString(statusStyle.Render(fmt.Sprintf("GitHub CLI (gh): %s", ghStatus)))
	content.WriteString("\n")
	
	authStatus := "❌ Not authenticated"
	if g.ghAuthenticated {
		authStatus = "✅ Authenticated"
	}
	content.WriteString(statusStyle.Render(fmt.Sprintf("GitHub Auth: %s", authStatus)))
	content.WriteString("\n")
	
	copilotStatus := "❌ Not set up"
	if g.copilotAuthed {
		copilotStatus = "✅ Ready to blaze"
	}
	content.WriteString(statusStyle.Render(fmt.Sprintf("GitHub Copilot: %s", copilotStatus)))
	content.WriteString("\n\n")
	
	// Instructions based on state
	if !g.ghInstalled {
		content.WriteString(t.S().Text.Render("🚀 First, we need to install GitHub CLI."))
		content.WriteString("\n")
		content.WriteString(t.S().Subtle.Render("Press Enter to open the download page"))
	} else if !g.ghAuthenticated {
		content.WriteString(t.S().Text.Render("🔑 Now let's authenticate with GitHub."))
		content.WriteString("\n")
		content.WriteString(t.S().Subtle.Render("Press Enter to run: gh auth login"))
	} else if !g.copilotAuthed {
		content.WriteString(t.S().Text.Render("🤖 Finally, let's set up GitHub Copilot."))
		content.WriteString("\n")
		content.WriteString(t.S().Subtle.Render("Press Enter to install and authenticate Copilot"))
	} else {
		content.WriteString(t.S().Text.Foreground(t.GreenDark).Bold(true).Render("✨ You're all set! GitHub and Copilot are ready!"))
		content.WriteString("\n")
		content.WriteString(t.S().Subtle.Render("Press Enter to continue"))
	}
	
	// Error message if any
	if g.errorMessage != "" {
		content.WriteString("\n")
		content.WriteString(errorStyle.Render(g.errorMessage))
	}
	
	// Help text
	content.WriteString("\n\n")
	helpText := "Enter: Proceed | Esc: Skip for now"
	content.WriteString(t.S().Subtle.Render(helpText))
	
	return dialogStyle.Render(content.String())
}

func (g *githubAuthDialogModel) dialogDimensions() (int, int) {
	return 60, 15
}

func (g *githubAuthDialogModel) Position() (int, int) {
	w, h := g.dialogDimensions()
	row := (g.wHeight - h) / 2
	col := (g.wWidth - w) / 2
	return row, col
}

func (g *githubAuthDialogModel) ID() dialogs.DialogID {
	return GitHubAuthDialogID
}