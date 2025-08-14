package shell

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/shell"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
)

type SetupState int

const (
	StateOffering SetupState = iota
	StateInstalling
	StateSuccess
	StateError
)

type ShellSetupDialog struct {
	state       SetupState
	shellType   shell.UserShell
	shellConfig *shell.ShellConfig
	width       int
	height      int
	selectedIdx int
	errorMsg    string
	execPath    string
	theme       *styles.Theme
}

func NewShellSetupDialog() (*ShellSetupDialog, error) {
	// Get the executable path
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}
	
	// Detect shell configuration
	config, err := shell.GetShellConfig()
	if err != nil {
		return nil, err
	}
	
	shellType, err := shell.DetectUserShell()
	if err != nil {
		shellType = shell.UserShellBash // Default to bash
	}
	
	return &ShellSetupDialog{
		state:       StateOffering,
		shellType:   shellType,
		shellConfig: config,
		execPath:    execPath,
		theme:       styles.CurrentTheme(),
	}, nil
}

func (d *ShellSetupDialog) Init() tea.Cmd {
	return nil
}

func (d *ShellSetupDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
		return d, nil
		
	case tea.KeyMsg:
		switch d.state {
		case StateOffering:
			switch msg.String() {
			case "up", "k":
				d.selectedIdx = 0
			case "down", "j":
				d.selectedIdx = 1
			case "enter":
				if d.selectedIdx == 0 {
					// Install shortcut
					d.state = StateInstalling
					return d, d.installShortcut()
				} else {
					// Skip
					return d, util.CmdHandler(dialogs.CloseDialogMsg{})
				}
			case "esc":
				return d, util.CmdHandler(dialogs.CloseDialogMsg{})
			}
			
		case StateSuccess:
			switch msg.String() {
			case "enter", "esc":
				return d, util.CmdHandler(dialogs.CloseDialogMsg{})
			}
			
		case StateError:
			switch msg.String() {
			case "enter", "esc":
				return d, util.CmdHandler(dialogs.CloseDialogMsg{})
			}
		}
		
	case ShellInstallSuccessMsg:
		d.state = StateSuccess
		return d, nil
		
	case ShellInstallErrorMsg:
		d.state = StateError
		d.errorMsg = msg.Error.Error()
		return d, nil
	}
	
	return d, nil
}

type ShellInstallSuccessMsg struct{}
type ShellInstallErrorMsg struct {
	Error error
}

func (d *ShellSetupDialog) installShortcut() tea.Cmd {
	return func() tea.Msg {
		if err := shell.InstallShortcut(d.execPath); err != nil {
			return ShellInstallErrorMsg{Error: err}
		}
		return ShellInstallSuccessMsg{}
	}
}

func (d *ShellSetupDialog) View() string {
	var content strings.Builder
	
	switch d.state {
	case StateOffering:
		content.WriteString(d.renderOffer())
	case StateInstalling:
		content.WriteString(d.renderInstalling())
	case StateSuccess:
		content.WriteString(d.renderSuccess())
	case StateError:
		content.WriteString(d.renderError())
	}
	
	// Create dialog box
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(d.theme.Primary).
		Padding(1, 2).
		Width(60).
		MaxWidth(d.width - 4)
	
	return lipgloss.Place(
		d.width,
		d.height,
		lipgloss.Center,
		lipgloss.Center,
		dialogStyle.Render(content.String()),
	)
}

func (d *ShellSetupDialog) renderOffer() string {
	var s strings.Builder
	
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(d.theme.Primary).
		MarginBottom(1)
	s.WriteString(titleStyle.Render("🐚 Setup Shell Shortcut"))
	s.WriteString("\n\n")
	
	s.WriteString(fmt.Sprintf("Would you like to add 'toke' and 'tk' shortcuts to your %s?\n\n", 
		shell.GetShellName(d.shellType)))
	
	s.WriteString(fmt.Sprintf("This will modify: %s\n\n", d.shellConfig.RCFile))
	
	s.WriteString("After installation, you can start Toke from anywhere by typing:\n")
	codeStyle := lipgloss.NewStyle().
		Foreground(d.theme.Success).
		MarginLeft(2)
	s.WriteString(codeStyle.Render("$ toke"))
	s.WriteString(" or ")
	s.WriteString(codeStyle.Render("$ tk"))
	s.WriteString("\n\n")
	
	// Options
	options := []string{"Install shortcuts", "Skip"}
	for i, opt := range options {
		cursor := "  "
		optStyle := lipgloss.NewStyle()
		
		if i == d.selectedIdx {
			cursor = "→ "
			optStyle = optStyle.Foreground(d.theme.Primary).Bold(true)
		} else {
			optStyle = optStyle.Foreground(d.theme.FgBase)
		}
		
		s.WriteString(cursor)
		s.WriteString(optStyle.Render(opt))
		s.WriteString("\n")
	}
	
	s.WriteString("\n")
	footerStyle := lipgloss.NewStyle().
		Foreground(d.theme.FgHalfMuted)
	s.WriteString(footerStyle.Render("↑/↓: navigate • enter: select • esc: skip"))
	
	return s.String()
}

func (d *ShellSetupDialog) renderInstalling() string {
	var s strings.Builder
	
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(d.theme.Primary).
		MarginBottom(1)
	s.WriteString(titleStyle.Render("Installing shortcuts..."))
	s.WriteString("\n\n")
	
	s.WriteString("Please wait...")
	
	return s.String()
}

func (d *ShellSetupDialog) renderSuccess() string {
	var s strings.Builder
	
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(d.theme.Success).
		MarginBottom(1)
	s.WriteString(titleStyle.Render("✓ Shortcuts installed successfully!"))
	s.WriteString("\n\n")
	
	s.WriteString(fmt.Sprintf("The shortcuts have been added to %s\n\n", d.shellConfig.RCFile))
	
	s.WriteString("To use them in your current terminal, run:\n")
	sourceCmd, _ := shell.GetShellSourceCommand()
	codeStyle := lipgloss.NewStyle().
		Foreground(d.theme.Success).
		MarginLeft(2)
	s.WriteString(codeStyle.Render(fmt.Sprintf("$ %s", sourceCmd)))
	s.WriteString("\n\n")
	
	s.WriteString("Or open a new terminal window.\n\n")
	
	footerStyle := lipgloss.NewStyle().
		Foreground(d.theme.FgHalfMuted)
	s.WriteString(footerStyle.Render("Press enter or esc to continue"))
	
	return s.String()
}

func (d *ShellSetupDialog) renderError() string {
	var s strings.Builder
	
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(d.theme.Error).
		MarginBottom(1)
	s.WriteString(titleStyle.Render("✗ Installation failed"))
	s.WriteString("\n\n")
	
	s.WriteString(fmt.Sprintf("Error: %s\n\n", d.errorMsg))
	
	s.WriteString("You can manually add the following to your shell config:\n")
	codeStyle := lipgloss.NewStyle().
		Foreground(d.theme.FgMuted).
		MarginLeft(2)
	s.WriteString(codeStyle.Render(fmt.Sprintf("alias toke='%s'", d.execPath)))
	s.WriteString("\n")
	s.WriteString(codeStyle.Render(fmt.Sprintf("alias tk='%s'", d.execPath)))
	s.WriteString("\n\n")
	
	footerStyle := lipgloss.NewStyle().
		Foreground(d.theme.FgHalfMuted)
	s.WriteString(footerStyle.Render("Press enter or esc to continue"))
	
	return s.String()
}

func (d *ShellSetupDialog) ID() dialogs.DialogID {
	return "shell_setup"
}

func (d *ShellSetupDialog) Position() (int, int) {
	return 0, 0
}