package loading

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/v2/spinner"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/common-nighthawk/go-figure"
	"github.com/chasedut/toke/internal/tui/styles"
)

// LoadingScreen shows an animated ASCII art loading screen with a joint theme
type LoadingScreen struct {
	width     int
	height    int
	frame     int
	startTime time.Time
	theme     *styles.Theme
	message   string
	spinner   spinner.Model
	
	// Animation state
	smokeOffset float64
	flameFrame  int
}

// New creates a new loading screen
func New() *LoadingScreen {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	
	return &LoadingScreen{
		startTime: time.Now(),
		theme:     styles.CurrentTheme(),
		spinner:   s,
		message:   "Setting up your local AI...",
	}
}

// SetMessage sets a custom loading message
func (l *LoadingScreen) SetMessage(msg string) {
	l.message = msg
}

// Init initializes the loading screen
func (l *LoadingScreen) Init() tea.Cmd {
	return tea.Batch(
		l.spinner.Tick,
		animate(),
	)
}

// Update handles messages for the loading screen
func (l *LoadingScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		l.width = msg.Width
		l.height = msg.Height
		return l, nil
		
	case animateMsg:
		l.frame++
		l.flameFrame = (l.flameFrame + 1) % 4
		l.smokeOffset += 0.2
		return l, animate()
		
	case spinner.TickMsg:
		var cmd tea.Cmd
		l.spinner, cmd = l.spinner.Update(msg)
		return l, cmd
	}
	
	return l, nil
}

// View renders the loading screen as a modal overlay
func (l *LoadingScreen) View() string {
	if l.width == 0 || l.height == 0 {
		return ""
	}
	
	// Create ASCII art with go-figure
	myFigure := figure.NewFigure("TOKE", "larry3d", true)
	asciiArt := myFigure.String()
	
	// Create the joint animation
	joint := l.getJointAnimation()
	
	// Build the modal content
	modalWidth := 60
	modalHeight := 20
	
	// Create the modal box
	modalStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Height(modalHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(l.theme.Primary).
		Background(lipgloss.Color("#1a1a1a")).
		Padding(1, 2)
	
	// Title style
	titleStyle := lipgloss.NewStyle().
		Foreground(l.theme.Primary).
		Bold(true).
		Width(modalWidth - 4).
		Align(lipgloss.Center)
	
	// Message style
	messageStyle := lipgloss.NewStyle().
		Foreground(l.theme.FgMuted).
		Width(modalWidth - 4).
		Align(lipgloss.Center)
	
	// Joint style with gradient effect
	jointStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(l.getFlameColorString())).
		Width(modalWidth - 4).
		Align(lipgloss.Center)
	
	// Loading progress
	progressStyle := lipgloss.NewStyle().
		Foreground(l.theme.Success).
		Width(modalWidth - 4).
		Align(lipgloss.Center)
	
	// Build content
	var content strings.Builder
	
	// ASCII Title
	for _, line := range strings.Split(asciiArt, "\n") {
		if line != "" {
			content.WriteString(titleStyle.Render(line))
			content.WriteString("\n")
		}
	}
	
	content.WriteString("\n")
	
	// Animated joint
	content.WriteString(jointStyle.Render(joint))
	content.WriteString("\n\n")
	
	// Loading message with spinner
	loadingText := fmt.Sprintf("%s %s", l.spinner.View(), l.message)
	content.WriteString(messageStyle.Render(loadingText))
	content.WriteString("\n\n")
	
	// Fun loading messages
	messages := l.getLoadingMessages()
	messageIdx := (l.frame / 30) % len(messages)
	content.WriteString(progressStyle.Render(messages[messageIdx]))
	
	// Create the modal
	modal := modalStyle.Render(content.String())
	
	// Place the modal in the center
	return lipgloss.Place(
		l.width,
		l.height,
		lipgloss.Center,
		lipgloss.Center,
		modal,
	)
}

// getJointAnimation returns animated ASCII joint art
func (l *LoadingScreen) getJointAnimation() string {
	frames := []string{
		`     )
    (
   __)__
  |     |
  |_____|`,
		`    ( )
   (   )
  __)__
  |     |
  |_____|`,
		`   ( ) )
  (     )
  __)__
  |     |
  |_____|`,
		`  ( ) ( )
 (       )
  __)__
  |     |
  |_____|`,
	}
	
	// Add smoke effect
	smoke := l.getSmokeAnimation()
	return smoke + "\n" + frames[l.flameFrame%len(frames)]
}

// getSmokeAnimation returns animated smoke
func (l *LoadingScreen) getSmokeAnimation() string {
	smokeFrames := []string{
		`    ° ·`,
		`   · ° `,
		`  ° · °`,
		` · ° · `,
	}
	return smokeFrames[l.frame%len(smokeFrames)]
}

// getFlameColorString returns a color string for the flame effect
func (l *LoadingScreen) getFlameColorString() string {
	colors := []string{
		"#ff6b35", // Orange
		"#f7931e", // Amber
		"#fbb040", // Yellow-orange
		"#ffcd3c", // Yellow
	}
	return colors[l.flameFrame%len(colors)]
}

// getLoadingMessages returns fun loading messages
func (l *LoadingScreen) getLoadingMessages() []string {
	return []string{
		"🔥 Lighting up the neural networks...",
		"💨 Inhaling some knowledge...",
		"🌿 Growing the context window...",
		"✨ Sparking creativity...",
		"🎯 Focusing the attention heads...",
		"🌊 Flowing through the transformer layers...",
		"🎨 Mixing the perfect blend...",
		"🚀 Getting lifted to higher dimensions...",
	}
}

// IsReady returns true when loading should complete
func (l *LoadingScreen) IsReady() bool {
	// Show for at least 1 second
	return time.Since(l.startTime) > 1*time.Second
}

// animateMsg is sent to animate the loading screen
type animateMsg struct{}

// animate returns a command that triggers animation
func animate() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
		return animateMsg{}
	})
}