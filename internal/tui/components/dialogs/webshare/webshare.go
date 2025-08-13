package webshare

import (
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/v2/key"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
	"github.com/chasedut/toke/internal/webshare"
)

type ShareDialog struct {
	width     int
	height    int
	localURL  string
	ngrokURL  string
	hasNgrok  bool
	sessionID string
	keymap    KeyMap
}

type KeyMap struct {
	Close     key.Binding
	Enter     key.Binding
	CopyLocal key.Binding
	CopyPublic key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Close: key.NewBinding(
			key.WithKeys("esc", "q"),
			key.WithHelp("esc/q", "close"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "action"),
		),
		CopyLocal: key.NewBinding(
			key.WithKeys("c", "l"),
			key.WithHelp("c/l", "copy local URL"),
		),
		CopyPublic: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "copy public URL"),
		),
	}
}

func NewShareDialog(sessionID string, urls *webshare.ShareURLs) *ShareDialog {
	hasNgrok := false
	ngrokURL := ""
	localURL := ""
	
	if urls != nil {
		localURL = urls.LocalURL
		if urls.NgrokURL != "" {
			hasNgrok = true
			ngrokURL = urls.NgrokURL
		}
	}
	
	return &ShareDialog{
		width:     60,
		height:    20,
		localURL:  localURL,
		ngrokURL:  ngrokURL,
		hasNgrok:  hasNgrok,
		sessionID: sessionID,
		keymap:    DefaultKeyMap(),
	}
}

func (d *ShareDialog) Init() tea.Cmd {
	return nil
}

func (d *ShareDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, d.keymap.Close):
			return d, util.CmdHandler(dialogs.CloseDialogMsg{})
		case key.Matches(msg, d.keymap.Enter):
			if !d.hasNgrok {
				// Open ngrok installation instructions
				return d, d.openNgrokDocs()
			} else {
				// Open the local URL
				return d, d.openLocalURL()
			}
		case key.Matches(msg, d.keymap.CopyLocal):
			// Copy local URL to clipboard
			return d, d.copyToClipboard(d.localURL)
		case key.Matches(msg, d.keymap.CopyPublic):
			// Copy public URL to clipboard
			if d.hasNgrok && d.ngrokURL != "" {
				return d, d.copyToClipboard(d.ngrokURL)
			}
			return d, nil
		}
	// TODO: Add mouse click handling for URLs
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
	}
	return d, nil
}

func (d *ShareDialog) View() string {
	t := styles.CurrentTheme()
	
	// Title
	titleStyle := t.S().Base.Bold(true).Foreground(t.Secondary)
	title := titleStyle.Render("🌐 Session Share Active")
	
	// Content
	var content strings.Builder
	
	content.WriteString(t.S().Base.Bold(true).Render("Your session is being shared!"))
	content.WriteString("\n\n")
	
	// Local URL
	content.WriteString(t.S().Base.Foreground(t.FgMuted).Render("Local URL:"))
	content.WriteString("\n")
	content.WriteString(t.S().Base.Foreground(t.Secondary).Underline(true).Render(d.localURL))
	content.WriteString("\n")
	content.WriteString(t.S().Base.Foreground(t.FgHalfMuted).Italic(true).Render("(⌘+click to open in browser)"))
	content.WriteString("\n\n")
	
	// Ngrok URL or setup instructions
	if d.hasNgrok {
		content.WriteString(t.S().Base.Foreground(t.FgMuted).Render("Public URL (via ngrok):"))
		content.WriteString("\n")
		content.WriteString(t.S().Base.Foreground(t.Success).Bold(true).Underline(true).Render(d.ngrokURL))
		content.WriteString("\n")
		content.WriteString(t.S().Base.Foreground(t.FgHalfMuted).Italic(true).Render("(⌘+click to open, or share with anyone)"))
	} else {
		content.WriteString(t.S().Base.Foreground(t.Warning).Render("⚠ Ngrok not found"))
		content.WriteString("\n\n")
		content.WriteString(t.S().Base.Foreground(t.FgMuted).Render("To share publicly, install ngrok:"))
		content.WriteString("\n")
		content.WriteString(t.S().Base.Foreground(t.FgHalfMuted).Render("1. Download from https://ngrok.com/download"))
		content.WriteString("\n")
		content.WriteString(t.S().Base.Foreground(t.FgHalfMuted).Render("2. Or install via: brew install ngrok"))
		content.WriteString("\n")
		content.WriteString(t.S().Base.Foreground(t.FgHalfMuted).Render("3. Sign up for free account at ngrok.com"))
		content.WriteString("\n")
		content.WriteString(t.S().Base.Foreground(t.FgHalfMuted).Render("4. Run: ngrok config add-authtoken <token>"))
		content.WriteString("\n\n")
		content.WriteString(t.S().Base.Foreground(t.FgMuted).Italic(true).Render("Press Enter to open ngrok docs"))
	}
	
	content.WriteString("\n\n")
	
	// Instructions
	content.WriteString(t.S().Base.Foreground(t.FgHalfMuted).Render("The shared view updates in real-time as you chat."))
	content.WriteString("\n\n")
	content.WriteString(t.S().Base.Foreground(t.FgHalfMuted).Render("Keyboard shortcuts:"))
	content.WriteString("\n")
	content.WriteString(t.S().Base.Foreground(t.FgHalfMuted).Render("• Enter: Open local URL"))
	content.WriteString("\n")
	content.WriteString(t.S().Base.Foreground(t.FgHalfMuted).Render("• C/L: Copy local URL to clipboard"))
	if d.hasNgrok {
		content.WriteString("\n")
		content.WriteString(t.S().Base.Foreground(t.FgHalfMuted).Render("• P: Copy public URL to clipboard"))
	}
	content.WriteString("\n")
	content.WriteString(t.S().Base.Foreground(t.FgHalfMuted).Render("• ESC: Close dialog (sharing continues)"))
	
	// Render in a box
	contentStr := content.String()
	
	dialogStyle := t.S().Base.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Width(d.width - 4).
		Padding(1, 2)
	
	return lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		dialogStyle.Render(contentStr),
	)
}

func (d *ShareDialog) openNgrokDocs() tea.Cmd {
	return func() tea.Msg {
		// Try to open the ngrok download page
		url := "https://ngrok.com/download"
		
		switch runtime.GOOS {
		case "darwin":
			exec.Command("open", url).Run()
		case "linux":
			exec.Command("xdg-open", url).Run()
		case "windows":
			exec.Command("cmd", "/c", "start", url).Run()
		}
		
		return dialogs.CloseDialogMsg{}
	}
}

func (d *ShareDialog) openLocalURL() tea.Cmd {
	return func() tea.Msg {
		// Try to open the local URL
		url := d.localURL
		if url == "" {
			return nil
		}
		
		switch runtime.GOOS {
		case "darwin":
			exec.Command("open", url).Run()
		case "linux":
			exec.Command("xdg-open", url).Run()
		case "windows":
			exec.Command("cmd", "/c", "start", url).Run()
		}
		
		return nil
	}
}

func (d *ShareDialog) copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		if text == "" {
			return nil
		}
		
		// Try to copy to clipboard using pbcopy/xclip/clip
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("pbcopy")
		case "linux":
			cmd = exec.Command("xclip", "-selection", "clipboard")
		case "windows":
			cmd = exec.Command("clip")
		default:
			return util.InfoMsg{
				Type: util.InfoTypeError,
				Msg:  "Clipboard copy not supported on this platform",
				TTL:  3 * time.Second,
			}
		}
		
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err != nil {
			return util.InfoMsg{
				Type: util.InfoTypeError,
				Msg:  "Failed to copy to clipboard",
				TTL:  3 * time.Second,
			}
		}
		
		return util.InfoMsg{
			Type: util.InfoTypeSuccess,
			Msg:  "URL copied to clipboard",
			TTL:  2 * time.Second,
		}
	}
}

func (d *ShareDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}

func (d *ShareDialog) IsFocusable() bool {
	return true
}

// Implement DialogModel interface
func (d *ShareDialog) ID() dialogs.DialogID {
	return "webshare"
}

func (d *ShareDialog) Position() (int, int) {
	// Center the dialog
	return 0, 0
}

func (d *ShareDialog) GetDialogID() dialogs.DialogID {
	return "webshare"
}

func (d *ShareDialog) IsClosable() bool {
	return true
}

func (d *ShareDialog) GetSize() (int, int) {
	return d.width, d.height
}