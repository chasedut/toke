package sessions

import (
	"github.com/charmbracelet/bubbles/v2/help"
	"github.com/charmbracelet/bubbles/v2/key"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/chasedut/toke/internal/session"
	"github.com/chasedut/toke/internal/tui/components/chat"
	"github.com/chasedut/toke/internal/tui/components/core"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	"github.com/chasedut/toke/internal/tui/exp/list"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
	"github.com/charmbracelet/lipgloss/v2"
)

const SessionsDialogID dialogs.DialogID = "sessions"

// SessionDialog interface for the session switching dialog
type SessionDialog interface {
	dialogs.DialogModel
}

type SessionsList = list.FilterableList[list.CompletionItem[session.Session]]

type sessionDialogCmp struct {
	selectedInx       int
	wWidth            int
	wHeight           int
	width             int
	selectedSessionID string
	keyMap            KeyMap
	sessionsList      SessionsList
	help              help.Model
}

// NewSessionDialogCmp creates a new session switching dialog
func NewSessionDialogCmp(sessions []session.Session, selectedID string) SessionDialog {
	t := styles.CurrentTheme()
	listKeyMap := list.DefaultKeyMap()
	keyMap := DefaultKeyMap()
	listKeyMap.Down.SetEnabled(false)
	listKeyMap.Up.SetEnabled(false)
	listKeyMap.DownOneItem = keyMap.Next
	listKeyMap.UpOneItem = keyMap.Previous

	items := make([]list.CompletionItem[session.Session], len(sessions))
	if len(sessions) > 0 {
		for i, session := range sessions {
			items[i] = list.NewCompletionItem(session.Title, session, list.WithCompletionID(session.ID))
		}
	}

	inputStyle := t.S().Base.PaddingLeft(1).PaddingBottom(1)
	sessionsList := list.NewFilterableList(
		items,
		list.WithFilterPlaceholder("Enter a session name"),
		list.WithFilterInputStyle(inputStyle),
		list.WithFilterListOptions(
			list.WithKeyMap(listKeyMap),
			list.WithWrapNavigation(),
		),
	)
	help := help.New()
	help.Styles = t.S().Help
	s := &sessionDialogCmp{
		selectedSessionID: selectedID,
		keyMap:            DefaultKeyMap(),
		sessionsList:      sessionsList,
		help:              help,
	}

	return s
}

func (s *sessionDialogCmp) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, s.sessionsList.Init())
	cmds = append(cmds, s.sessionsList.Focus())
	return tea.Sequence(cmds...)
}

func (s *sessionDialogCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		var cmds []tea.Cmd
		s.wWidth = msg.Width
		s.wHeight = msg.Height
		s.width = min(120, s.wWidth-8)
		s.sessionsList.SetInputWidth(s.listWidth() - 2)
		cmds = append(cmds, s.sessionsList.SetSize(s.listWidth(), s.listHeight()))
		if s.selectedSessionID != "" {
			cmds = append(cmds, s.sessionsList.SetSelected(s.selectedSessionID))
		}
		return s, tea.Batch(cmds...)
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, s.keyMap.Select):
			selectedItem := s.sessionsList.SelectedItem()
			if selectedItem != nil {
				selected := *selectedItem
				return s, tea.Sequence(
					util.CmdHandler(dialogs.CloseDialogMsg{}),
					util.CmdHandler(
						chat.SessionSelectedMsg(selected.Value()),
					),
				)
			}
		case key.Matches(msg, s.keyMap.Close):
			return s, util.CmdHandler(dialogs.CloseDialogMsg{})
		default:
			u, cmd := s.sessionsList.Update(msg)
			s.sessionsList = u.(SessionsList)
			return s, cmd
		}
	}
	return s, nil
}

func (s *sessionDialogCmp) View() string {
	t := styles.CurrentTheme()
	listView := s.sessionsList.View()
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		t.S().Base.Padding(0, 1, 1, 1).Render(core.Title("Switch Session", s.width-4)),
		listView,
		"",
		t.S().Base.Width(s.width-2).PaddingLeft(1).AlignHorizontal(lipgloss.Left).Render(s.help.View(s.keyMap)),
	)

	return s.style().Render(content)
}

func (s *sessionDialogCmp) Cursor() *tea.Cursor {
	if cursor, ok := s.sessionsList.(util.Cursor); ok {
		cursor := cursor.Cursor()
		if cursor != nil {
			cursor = s.moveCursor(cursor)
		}
		return cursor
	}
	return nil
}

func (s *sessionDialogCmp) style() lipgloss.Style {
	t := styles.CurrentTheme()
	return t.S().Base.
		Width(s.width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocus)
}

func (s *sessionDialogCmp) listHeight() int {
	if s.wHeight == 0 {
		return 10 // Default height
	}
	height := s.wHeight/2 - 6 // 5 for the border, title and help
	return max(5, height) // Ensure minimum height of 5
}

func (s *sessionDialogCmp) listWidth() int {
	if s.width <= 2 {
		return 10 // Default width
	}
	return max(10, s.width - 2) // 2 for the border, ensure minimum width
}

func (s *sessionDialogCmp) Position() (int, int) {
	// Use reasonable defaults if window size not set yet
	width := s.wWidth
	height := s.wHeight
	if width == 0 {
		width = 80  // Conservative default terminal width
	}
	if height == 0 {
		height = 24  // Conservative default terminal height
	}
	
	// Center vertically but ensure dialog fits
	// Estimate height based on typical session list size
	dialogHeight := min(20, height/2) + 6 // list height + borders/padding
	row := (height - dialogHeight) / 2
	
	// Ensure dialog doesn't go off bottom
	maxRow := height - dialogHeight - 3
	if row > maxRow {
		row = maxRow
	}
	if row < 2 {
		row = 2
	}
	
	col := width / 2
	col -= s.width / 2
	if col < 2 {
		col = 2
	}
	return row, col
}

func (s *sessionDialogCmp) moveCursor(cursor *tea.Cursor) *tea.Cursor {
	row, col := s.Position()
	offset := row + 3 // Border + title
	cursor.Y += offset
	cursor.X = cursor.X + col + 2
	return cursor
}

// ID implements SessionDialog.
func (s *sessionDialogCmp) ID() dialogs.DialogID {
	return SessionsDialogID
}
