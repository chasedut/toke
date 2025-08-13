package buddychat

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/webshare"
)

// BuddyChatComponent handles buddy chat functionality
type BuddyChatComponent struct {
	width    int
	height   int
	visible  bool
	focused  bool
	input    textinput.Model
	messages []webshare.BuddyMessage
	buddies  []*webshare.Buddy
	share    *webshare.SessionShare
}

// BuddyJoinedMsg is sent when a buddy joins
type BuddyJoinedMsg struct {
	Buddy *webshare.Buddy
}

// BuddyMessageMsg is sent when a buddy sends a message
type BuddyMessageMsg struct {
	Message webshare.BuddyMessage
}

// ToggleBuddyChatMsg toggles the buddy chat visibility
type ToggleBuddyChatMsg struct{}

// SendBuddyMessageMsg sends a message to buddies
type SendBuddyMessageMsg struct {
	Message string
}

func New() *BuddyChatComponent {
	ti := textinput.New()
	ti.Placeholder = "Message your buddy..."
	ti.CharLimit = 500

	return &BuddyChatComponent{
		input:    ti,
		messages: make([]webshare.BuddyMessage, 0),
		buddies:  make([]*webshare.Buddy, 0),
		visible:  false,
	}
}

func (b *BuddyChatComponent) Init() tea.Cmd {
	return nil
}

func (b *BuddyChatComponent) SetShare(share *webshare.SessionShare) {
	b.share = share
	if share != nil {
		b.buddies = share.GetBuddies()
	}
}

func (b *BuddyChatComponent) Update(msg tea.Msg) (*BuddyChatComponent, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		b.width = msg.Width / 3  // Take up 1/3 of screen width
		b.height = msg.Height / 3 // Take up 1/3 of screen height
		return b, nil

	case ToggleBuddyChatMsg:
		b.visible = !b.visible
		if b.visible {
			b.focused = true
			return b, b.input.Focus()
		}
		b.focused = false
		b.input.Blur()
		return b, nil

	case BuddyJoinedMsg:
		b.buddies = append(b.buddies, msg.Buddy)
		// Show chat when buddy joins
		b.visible = true
		return b, nil

	case BuddyMessageMsg:
		b.messages = append(b.messages, msg.Message)
		// Keep only last 50 messages
		if len(b.messages) > 50 {
			b.messages = b.messages[len(b.messages)-50:]
		}
		return b, nil

	case tea.KeyMsg:
		if b.focused {
			switch msg.String() {
			case "esc":
				b.focused = false
				b.input.Blur()
				return b, nil
			case "enter":
				if b.input.Value() != "" && b.share != nil {
					// Send message to all buddies
					for _, buddy := range b.buddies {
						b.share.SendMessageToBuddy(buddy.ID, b.input.Value())
					}
					// Don't add to local messages - it will come back via broadcast
					b.input.SetValue("")
				}
				return b, nil
			}
			
			var cmd tea.Cmd
			b.input, cmd = b.input.Update(msg)
			return b, cmd
		}
	}

	return b, nil
}

func (b *BuddyChatComponent) View() string {
	if !b.visible {
		return ""
	}

	t := styles.CurrentTheme()
	
	var content strings.Builder
	
	// Title with buddy count
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Primary).
		MarginBottom(1)
	
	buddyCount := len(b.buddies)
	title := fmt.Sprintf("💬 Buddy Chat (%d connected)", buddyCount)
	content.WriteString(titleStyle.Render(title))
	content.WriteString("\n")
	
	// Messages area
	messagesHeight := b.height - 6 // Leave room for title, input, borders
	messagesStyle := lipgloss.NewStyle().
		Height(messagesHeight).
		Width(b.width - 4).
		Border(lipgloss.NormalBorder()).
		BorderForeground(t.Border).
		Padding(0, 1)
	
	var messagesContent strings.Builder
	
	// If no messages and no buddies, show help text
	if len(b.messages) == 0 && len(b.buddies) == 0 {
		helpStyle := lipgloss.NewStyle().Foreground(t.FgMuted).Italic(true)
		messagesContent.WriteString(helpStyle.Render("Buddy chat allows web visitors to chat with you.\n\n"))
		messagesContent.WriteString(helpStyle.Render("Note: Real-time buddy events are temporarily\n"))
		messagesContent.WriteString(helpStyle.Render("disabled to prevent UI freezing.\n\n"))
		messagesContent.WriteString(helpStyle.Render("Web visitors can still view your session\n"))
		messagesContent.WriteString(helpStyle.Render("at the URLs shown in the sidebar."))
	} else {
		startIdx := 0
		if len(b.messages) > messagesHeight-2 {
			startIdx = len(b.messages) - (messagesHeight - 2)
		}
		
		for i := startIdx; i < len(b.messages); i++ {
			msg := b.messages[i]
			timeStr := msg.Time.Format("15:04")
			
			msgStyle := lipgloss.NewStyle()
			if msg.FromID == "host" {
				msgStyle = msgStyle.Foreground(t.Primary)
			} else {
				msgStyle = msgStyle.Foreground(t.Secondary)
			}
			
			messagesContent.WriteString(fmt.Sprintf("[%s] %s: %s\n", 
				timeStr, msg.FromName, msg.Message))
		}
	}
	
	content.WriteString(messagesStyle.Render(messagesContent.String()))
	content.WriteString("\n")
	
	// Input area
	inputLabel := ">"
	if b.focused {
		inputLabel = "▶"
	}
	content.WriteString(inputLabel + " " + b.input.View())
	
	// Wrap in a box
	boxStyle := lipgloss.NewStyle().
		Width(b.width).
		Height(b.height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Primary).
		Padding(1)
	
	return boxStyle.Render(content.String())
}

func (b *BuddyChatComponent) Focus() tea.Cmd {
	b.focused = true
	return b.input.Focus()
}

func (b *BuddyChatComponent) Blur() {
	b.focused = false
	b.input.Blur()
}

func (b *BuddyChatComponent) IsFocused() bool {
	return b.focused
}

func (b *BuddyChatComponent) IsVisible() bool {
	return b.visible
}

func (b *BuddyChatComponent) SetVisible(visible bool) {
	b.visible = visible
}

func (b *BuddyChatComponent) GetBuddyCount() int {
	return len(b.buddies)
}

func (b *BuddyChatComponent) GetShare() *webshare.SessionShare {
	return b.share
}