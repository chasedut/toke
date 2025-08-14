package tui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/v2/key"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/config"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
)

const resetDialogWidth = 50

type ResetConfigDialog struct {
	width     int
	height    int
	wWidth    int // Window width
	wHeight   int // Window height
	confirmed bool
	keyMap    resetKeyMap
}

type resetKeyMap struct {
	Confirm key.Binding
	Cancel  key.Binding
	Toggle  key.Binding
}

func defaultResetKeyMap() resetKeyMap {
	return resetKeyMap{
		Confirm: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc", "n", "N"),
			key.WithHelp("esc/n", "cancel"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("tab", "left", "right"),
			key.WithHelp("tab", "toggle"),
		),
	}
}

func NewResetConfigDialog() dialogs.DialogModel {
	return &ResetConfigDialog{
		width:     resetDialogWidth,
		confirmed: false, // Default to No
		keyMap:    defaultResetKeyMap(),
	}
}

func (d *ResetConfigDialog) Init() tea.Cmd {
	return nil
}

func (d *ResetConfigDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.wWidth = msg.Width
		d.wHeight = msg.Height
		return d, nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, d.keyMap.Cancel):
			return d, util.CmdHandler(dialogs.CloseDialogMsg{})
		case key.Matches(msg, d.keyMap.Toggle):
			d.confirmed = !d.confirmed
			return d, nil
		case key.Matches(msg, d.keyMap.Confirm):
			if d.confirmed {
				// Reset configuration
				if err := d.resetConfiguration(); err != nil {
					return d, util.ReportError(fmt.Errorf("failed to reset configuration: %w", err))
				}
				// Quit and restart
				return d, tea.Sequence(
					util.CmdHandler(dialogs.CloseDialogMsg{}),
					util.ReportInfo("Configuration reset. Please restart toke."),
					tea.Quit,
				)
			}
			// Cancel
			return d, util.CmdHandler(dialogs.CloseDialogMsg{})
		}
	}
	return d, nil
}

func (d *ResetConfigDialog) resetConfiguration() error {
	// Remove configuration files
	paths := []string{
		config.GlobalConfigData(),
		filepath.Join(filepath.Dir(config.GlobalConfigData()), "local_model.json"),
	}
	
	// Also check for .local/share/toke directory
	if homeDir, err := os.UserHomeDir(); err == nil {
		paths = append(paths, 
			filepath.Join(homeDir, ".local", "share", "toke", "toke.json"),
			filepath.Join(homeDir, ".config", "toke", "toke.json"),
		)
	}
	
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			// Log but don't fail if we can't remove a file
			continue
		}
	}
	
	return nil
}

func (d *ResetConfigDialog) View() string {
	t := styles.CurrentTheme()
	
	title := t.S().Base.Bold(true).Foreground(t.Warning).Render("⚠️  Reset Configuration?")
	
	warning := t.S().Base.Foreground(t.FgMuted).Render(
		"This will clear all saved configuration including:\n" +
		"  • API keys\n" +
		"  • Model selections\n" +
		"  • Provider settings\n" +
		"\nYou will need to set up toke again after restart.")
	
	yesStyle := t.S().Base.Padding(0, 2)
	noStyle := t.S().Base.Padding(0, 2)
	
	if d.confirmed {
		yesStyle = yesStyle.Background(t.Error).Foreground(t.FgBase)
	} else {
		noStyle = noStyle.Background(t.Success).Foreground(t.FgBase)
	}
	
	yes := yesStyle.Render("Yes, Reset")
	no := noStyle.Render("No, Keep")
	
	buttons := lipgloss.JoinHorizontal(lipgloss.Center, no, "  ", yes)
	
	help := t.S().Base.Foreground(t.FgSubtle).Render("Tab to toggle • Enter to confirm • Esc to cancel")
	
	content := lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"",
		warning,
		"",
		"",
		buttons,
		"",
		help,
	)
	
	return t.S().Base.
		Width(d.width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocus).
		Padding(1, 2).
		Render(content)
}

func (d *ResetConfigDialog) SetSize(width, height int) tea.Cmd {
	// Dialog has fixed size
	return nil
}

func (d *ResetConfigDialog) ID() dialogs.DialogID {
	return "reset_config"
}

func (d *ResetConfigDialog) Position() (int, int) {
	// Center the dialog
	if d.wHeight == 0 || d.wWidth == 0 {
		// Default position if window size not yet known
		return 5, 10
	}
	
	// Calculate center position
	dialogHeight := 15 // Approximate height of the dialog
	row := (d.wHeight - dialogHeight) / 2
	col := (d.wWidth - d.width) / 2
	
	// Ensure minimum position
	if row < 2 {
		row = 2
	}
	if col < 2 {
		col = 2
	}
	
	return row, col
}