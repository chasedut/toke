package quit

import (
	"github.com/charmbracelet/bubbles/v2/key"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
	"github.com/charmbracelet/lipgloss/v2"
)

const (
	question                      = "What would you like to do?"
	QuitDialogID dialogs.DialogID = "quit"
)

// QuitDialog represents a confirmation dialog for quitting the application.
type QuitDialog interface {
	dialogs.DialogModel
}

type quitDialogCmp struct {
	wWidth  int
	wHeight int

	selectedOption int // 0: Quit, 1: Show Terminal, 2: Cancel
	keymap         KeyMap
}

// NewQuitDialog creates a new quit confirmation dialog.
func NewQuitDialog() QuitDialog {
	return &quitDialogCmp{
		selectedOption: 0, // Default to "Quit"
		keymap:         DefaultKeymap(),
	}
}

func (q *quitDialogCmp) Init() tea.Cmd {
	return nil
}

// Update handles keyboard input for the quit dialog.
func (q *quitDialogCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		q.wWidth = msg.Width
		q.wHeight = msg.Height
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, q.keymap.LeftRight, q.keymap.Tab):
			// Cycle through options: 0 -> 1 -> 2 -> 0
			q.selectedOption = (q.selectedOption + 1) % 3
			return q, nil
		case key.Matches(msg, q.keymap.EnterSpace):
			switch q.selectedOption {
			case 0: // Quit
				return q, tea.Quit
			case 1: // Show Terminal
				// Send a command to exit toke but keep terminal open
				return q, tea.Sequence(
					util.CmdHandler(dialogs.CloseDialogMsg{}),
					tea.Quit,
				)
			case 2: // Cancel
				return q, util.CmdHandler(dialogs.CloseDialogMsg{})
			}
		case key.Matches(msg, q.keymap.Yes):
			return q, tea.Quit
		case key.Matches(msg, q.keymap.No, q.keymap.Close):
			return q, util.CmdHandler(dialogs.CloseDialogMsg{})
		}
	}
	return q, nil
}

// View renders the quit dialog with three option buttons.
func (q *quitDialogCmp) View() string {
	t := styles.CurrentTheme()
	baseStyle := t.S().Base
	buttonStyle := t.S().Text

	// Create styles for each button
	quitStyle := buttonStyle.Background(t.BgSubtle)
	terminalStyle := buttonStyle.Background(t.BgSubtle)
	cancelStyle := buttonStyle.Background(t.BgSubtle)

	// Highlight the selected option
	switch q.selectedOption {
	case 0:
		quitStyle = quitStyle.Foreground(t.White).Background(t.Secondary)
	case 1:
		terminalStyle = terminalStyle.Foreground(t.White).Background(t.Secondary)
	case 2:
		cancelStyle = cancelStyle.Foreground(t.White).Background(t.Secondary)
	}

	const horizontalPadding = 2
	quitButton := quitStyle.Padding(0, horizontalPadding).Render("Quit")
	terminalButton := terminalStyle.Padding(0, horizontalPadding).Render("Show Terminal")
	cancelButton := cancelStyle.Padding(0, horizontalPadding).Render("Cancel")

	buttons := baseStyle.Align(lipgloss.Center).Render(
		lipgloss.JoinHorizontal(lipgloss.Center, quitButton, " ", terminalButton, " ", cancelButton),
	)

	content := baseStyle.Render(
		lipgloss.JoinVertical(
			lipgloss.Center,
			question,
			"",
			buttons,
		),
	)

	quitDialogStyle := baseStyle.
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocus)

	return quitDialogStyle.Render(content)
}

func (q *quitDialogCmp) Position() (int, int) {
	row := q.wHeight / 2
	row -= 7 / 2
	col := q.wWidth / 2
	// Adjust for wider dialog with three buttons
	col -= 35 / 2

	return row, col
}

func (q *quitDialogCmp) ID() dialogs.DialogID {
	return QuitDialogID
}
