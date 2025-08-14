package ngrokauth

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
)

const (
	NgrokAuthDialogID dialogs.DialogID = "ngrok-auth"
)

type NgrokAuthDialog struct {
	width      int
	height     int
	textInput  textinput.Model
	ngrokPath  string
	sessionID  string
	err        error
}

type NgrokAuthSuccessMsg struct {
	SessionID string
}

func NewNgrokAuthDialog(ngrokPath, sessionID string) *NgrokAuthDialog {
	ti := textinput.New()
	ti.Placeholder = "Enter your ngrok authtoken"
	ti.Focus()
	ti.CharLimit = 100
	// Width will be set dynamically
	
	return &NgrokAuthDialog{
		textInput: ti,
		ngrokPath: ngrokPath,
		sessionID: sessionID,
	}
}

func (d *NgrokAuthDialog) Init() tea.Cmd {
	return textinput.Blink
}

func (d *NgrokAuthDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			token := strings.TrimSpace(d.textInput.Value())
			if token != "" {
				return d, d.configureNgrok(token)
			}
		case "esc":
			return d, util.CmdHandler(dialogs.CloseDialogMsg{})
		}
	
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
		return d, nil
	}
	
	var cmd tea.Cmd
	d.textInput, cmd = d.textInput.Update(msg)
	cmds = append(cmds, cmd)
	
	return d, tea.Batch(cmds...)
}

func (d *NgrokAuthDialog) View() string {
	if d.width == 0 || d.height == 0 {
		return ""
	}
	
	t := styles.CurrentTheme()
	
	// Build content
	var content strings.Builder
	
	// Title
	title := t.S().Base.Bold(true).Foreground(t.Primary).Render("🔐 Ngrok Authentication Required")
	content.WriteString(lipgloss.NewStyle().Width(d.width).Align(lipgloss.Center).Render(title))
	content.WriteString("\n\n")
	
	// Instructions
	instructions := []string{
		"Ngrok requires authentication to create public tunnels.",
		"",
		"To get your authtoken:",
		"1. Sign up for free at https://ngrok.com",
		"2. Go to https://dashboard.ngrok.com/auth",
		"3. Copy your authtoken",
		"",
	}
	
	for _, line := range instructions {
		if line == "" {
			content.WriteString("\n")
		} else if strings.HasPrefix(line, "To get") {
			content.WriteString(t.S().Base.Bold(true).Render(line))
			content.WriteString("\n")
		} else {
			content.WriteString(t.S().Base.Foreground(t.FgMuted).Render(line))
			content.WriteString("\n")
		}
	}
	
	// Input field
	content.WriteString(d.textInput.View())
	content.WriteString("\n\n")
	
	// Error message if any
	if d.err != nil {
		errMsg := t.S().Base.Foreground(t.Error).Render("❌ " + d.err.Error())
		content.WriteString(errMsg)
		content.WriteString("\n\n")
	}
	
	// Footer
	footer := t.S().Base.Foreground(t.FgHalfMuted).Render("Press Enter to authenticate • Esc to cancel")
	content.WriteString(footer)
	
	// Center the dialog
	dialogStyle := t.S().Base.
		Width(60).
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

func (d *NgrokAuthDialog) configureNgrok(token string) tea.Cmd {
	return func() tea.Msg {
		// Configure ngrok with the auth token
		cmd := exec.Command(d.ngrokPath, "config", "add-authtoken", token)
		if err := cmd.Run(); err != nil {
			d.err = fmt.Errorf("failed to configure ngrok: %w", err)
			return nil
		}
		
		// Token is now configured in ngrok
		// No need to save separately as ngrok manages it
		
		// Close dialog and signal success
		return tea.Batch(
			util.CmdHandler(dialogs.CloseDialogMsg{}),
			util.CmdHandler(NgrokAuthSuccessMsg{SessionID: d.sessionID}),
		)()
	}
}

func (d *NgrokAuthDialog) SetSize(width, height int) tea.Cmd {
	d.width = width
	d.height = height
	return nil
}

func (d *NgrokAuthDialog) ID() dialogs.DialogID {
	return NgrokAuthDialogID
}

func (d *NgrokAuthDialog) Position() (int, int) {
	// Return 0, 0 to center the dialog
	return 0, 0
}