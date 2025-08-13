package backend

import (
	"fmt"
	"strings"
	
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/common-nighthawk/go-figure"
	"github.com/chasedut/toke/internal/backend"
)

// Enhanced download view with animated joint
func (m Model) renderDownloadingAnimated() string {
	var s strings.Builder
	
	// Create ASCII art with go-figure
	myFigure := figure.NewFigure("TOKE", "larry3d", true)
	asciiArt := myFigure.String()
	
	// Title style
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true)
	
	// Render ASCII art
	for _, line := range strings.Split(asciiArt, "\n") {
		if line != "" {
			s.WriteString(titleStyle.Render(line))
			s.WriteString("\n")
		}
	}
	
	s.WriteString("\n")
	
	// Add animated joint
	joint := m.getJointFrame()
	jointStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.getFlameColorString())).
		Align(lipgloss.Center)
	
	s.WriteString(jointStyle.Render(joint))
	s.WriteString("\n\n")
	
	// Download title
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("70")).
		MarginBottom(1).
		Render("🌿 Setting up your local AI...")
	
	s.WriteString(title + "\n\n")
	
	// Model info
	if m.selectedModel != nil {
		// Use actual total size if known, otherwise use estimate
		displaySize := m.selectedModel.Size
		if m.totalBytes > 0 {
			displaySize = m.totalBytes
		}
		modelInfo := fmt.Sprintf("Model: %s\nSize: %s\n\n", 
			m.selectedModel.Name,
			backend.FormatSize(displaySize))
		s.WriteString(modelInfo)
	}
	
	// Download message
	s.WriteString(m.downloadMsg + "\n\n")
	
	// Progress bar
	s.WriteString(m.progress.View() + "\n\n")
	
	// Download stats - show detailed progress
	if m.totalBytes > 0 {
		downloaded := backend.FormatSize(m.bytesDownloaded)
		total := backend.FormatSize(m.totalBytes)
		percent := int(float64(m.bytesDownloaded) / float64(m.totalBytes) * 100)
		
		// Show both size and percentage for clarity
		// Even at 0%, show "0.00 MB / X.X GB (0%)" so user knows total size
		stats := fmt.Sprintf("%s / %s (%d%%)",
			downloaded, total, percent)
		
		// Use different styling for the stats to make them more prominent
		statsStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)
		s.WriteString(statsStyle.Render(stats) + "\n\n")
	} else if m.bytesDownloaded > 0 {
		// Show downloaded amount even if total is unknown
		downloaded := backend.FormatSize(m.bytesDownloaded)
		stats := fmt.Sprintf("%s downloaded", downloaded)
		statsStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)
		s.WriteString(statsStyle.Render(stats) + "\n\n")
	} else {
		// No progress yet but we might know the total size
		if m.selectedModel != nil && m.selectedModel.Size > 0 {
			total := backend.FormatSize(m.selectedModel.Size)
			stats := fmt.Sprintf("0.00 MB / %s (0%%)", total)
			statsStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("86")).
				Bold(true)
			s.WriteString(statsStyle.Render(stats) + "\n\n")
		}
	}
	
	// Fun message
	messages := m.getLoadingMessages()
	messageIdx := (m.animFrame / 30) % len(messages)
	msgStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("213")).
		Italic(true)
	s.WriteString(msgStyle.Render(messages[messageIdx]))
	
	// Add minimize hint
	s.WriteString("\n\n")
	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true)
	s.WriteString(hintStyle.Render("Press 'b' to continue in background • Press 'esc' to cancel"))
	
	return s.String()
}

// getJointFrame returns the current frame of the joint animation
func (m Model) getJointFrame() string {
	frames := []string{
		`     )
    (
   _)(_
  |    |
  |____|`,
		`    ( )
   (   )
   _)(_
  |    |
  |____|`,
		`   ( ) )
  (     )
   _)(_
  |    |
  |____|`,
		`  ( ) ( )
 (       )
   _)(_
  |    |
  |____|`,
	}
	
	// Add smoke effect
	smoke := m.getSmokeFrame()
	return smoke + "\n" + frames[m.flameFrame%len(frames)]
}

// getSmokeFrame returns animated smoke
func (m Model) getSmokeFrame() string {
	smokeFrames := []string{
		`    ° ·`,
		`   · ° `,
		`  ° · °`,
		` · ° · `,
	}
	return smokeFrames[m.animFrame%len(smokeFrames)]
}

// getFlameColorString returns a color string for the flame effect
func (m Model) getFlameColorString() string {
	colors := []string{
		"#ff6b35", // Orange
		"#f7931e", // Amber
		"#fbb040", // Yellow-orange
		"#ffcd3c", // Yellow
	}
	return colors[m.flameFrame%len(colors)]
}

// getLoadingMessages returns fun loading messages
func (m Model) getLoadingMessages() []string {
	return []string{
		"🔥 Lighting up the neural networks...",
		"💨 Taking a hit of knowledge...",
		"🌿 Growing the context window...",
		"✨ Sparking creativity...",
		"🎯 Focusing the attention heads...",
		"🌊 Flowing through transformer layers...",
		"🎨 Mixing the perfect blend...",
		"🚀 Getting lifted to higher dimensions...",
		"💭 Expanding consciousness...",
		"🧠 Activating neurons...",
	}
}