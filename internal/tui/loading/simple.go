package loading

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/tui/styles"
)

// SimpleLoadingScreen shows a simple loading screen for initial startup
type SimpleLoadingScreen struct {
	width     int
	height    int
	frame     int
	startTime time.Time
	theme     *styles.Theme
}

// NewSimple creates a new simple loading screen
func NewSimple() *SimpleLoadingScreen {
	return &SimpleLoadingScreen{
		startTime: time.Now(),
		theme:     styles.CurrentTheme(),
	}
}

// Init initializes the loading screen
func (l *SimpleLoadingScreen) Init() tea.Cmd {
	return animateSimple()
}

// Update handles messages for the loading screen
func (l *SimpleLoadingScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		l.width = msg.Width
		l.height = msg.Height
		return l, nil
		
	case simpleAnimateMsg:
		l.frame++
		return l, animateSimple()
	}
	
	return l, nil
}

// View renders the simple loading screen
func (l *SimpleLoadingScreen) View() string {
	if l.width == 0 || l.height == 0 {
		return ""
	}
	
	// ASCII art for TOKE
	asciiArt := []string{
		`  ████████╗ ██████╗ ██╗  ██╗███████╗`,
		`  ╚══██╔══╝██╔═══██╗██║ ██╔╝██╔════╝`,
		`     ██║   ██║   ██║█████╔╝ █████╗  `,
		`     ██║   ██║   ██║██╔═██╗ ██╔══╝  `,
		`     ██║   ╚██████╔╝██║  ██╗███████╗`,
		`     ╚═╝    ╚═════╝ ╚═╝  ╚═╝╚══════╝`,
	}
	
	// Loading text with animation
	loadingFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinner := loadingFrames[l.frame%len(loadingFrames)]
	
	// Loading messages
	messages := []string{
		"Initializing AI assistant",
		"Loading language models",
		"Preparing workspace",
		"Starting Toke",
	}
	messageIdx := (l.frame / 10) % len(messages)
	loadingText := spinner + "  " + messages[messageIdx] + "..."
	
	// Style the components
	artStyle := lipgloss.NewStyle().
		Foreground(l.theme.Primary).
		Bold(true)
	
	loadingStyle := lipgloss.NewStyle().
		Foreground(l.theme.FgMuted).
		MarginTop(2)
	
	versionStyle := lipgloss.NewStyle().
		Foreground(l.theme.FgHalfMuted).
		MarginTop(1)
	
	// Build the content
	var content strings.Builder
	
	// Add ASCII art
	for _, line := range asciiArt {
		content.WriteString(artStyle.Render(line))
		content.WriteString("\n")
	}
	
	// Add loading text
	content.WriteString(loadingStyle.Render(loadingText))
	content.WriteString("\n")
	
	// Add version/tagline
	content.WriteString(versionStyle.Render("AI-powered coding assistant"))
	
	// Center everything
	finalContent := content.String()
	
	return lipgloss.Place(
		l.width,
		l.height,
		lipgloss.Center,
		lipgloss.Center,
		finalContent,
	)
}

// simpleAnimateMsg is sent to animate the loading screen
type simpleAnimateMsg struct{}

// animateSimple returns a command that triggers animation
func animateSimple() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return simpleAnimateMsg{}
	})
}