package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/v2/progress"
	"github.com/charmbracelet/bubbles/v2/spinner"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170")).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginBottom(2)

	itemStyle = lipgloss.NewStyle().
			PaddingLeft(2)

	doneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("70"))

	progressStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			PaddingRight(2)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)
)

type ProgressMsg struct {
	Name         string
	CurrentBytes int64
	TotalBytes   int64
	Status       string
	Error        error
}

type DoneMsg struct{}

type LoadingModel struct {
	width    int
	height   int
	spinner  spinner.Model
	progress progress.Model
	items    map[string]*ItemProgress
	order    []string
	done     bool
	err      error
}

type ItemProgress struct {
	Name         string
	Status       string
	CurrentBytes int64
	TotalBytes   int64
	Progress     float64
	Done         bool
	Error        error
}

func NewLoadingModel() LoadingModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("170"))

	p := progress.New(progress.WithDefaultGradient())

	return LoadingModel{
		spinner:  s,
		progress: p,
		items:    make(map[string]*ItemProgress),
		order:    []string{},
	}
}

func (m LoadingModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.waitForProgress(),
	)
}

func (m LoadingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.SetWidth(msg.Width - 4)
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		var cmd tea.Cmd
		m.progress, cmd = m.progress.Update(msg)
		return m, cmd

	case ProgressMsg:
		if _, exists := m.items[msg.Name]; !exists {
			m.order = append(m.order, msg.Name)
		}

		item := &ItemProgress{
			Name:         msg.Name,
			Status:       msg.Status,
			CurrentBytes: msg.CurrentBytes,
			TotalBytes:   msg.TotalBytes,
			Error:        msg.Error,
		}

		if msg.TotalBytes > 0 {
			item.Progress = float64(msg.CurrentBytes) / float64(msg.TotalBytes)
		}

		if msg.Status == "Installed" || msg.Status == "Already installed" || 
		   strings.HasPrefix(msg.Status, "Using local") || strings.HasPrefix(msg.Status, "Found local") {
			item.Done = true
			item.Progress = 1.0
		}

		m.items[msg.Name] = item

		return m, tea.Batch(
			m.progress.SetPercent(item.Progress),
			m.waitForProgress(),
		)

	case DoneMsg:
		m.done = true
		return m, tea.Quit

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m LoadingModel) View() string {
	if m.done {
		return doneStyle.Render("✓ All dependencies ready!\n")
	}

	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("🚀 Toke - Initializing"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Setting up required dependencies..."))
	b.WriteString("\n\n")

	// Show progress for each item
	for _, name := range m.order {
		item := m.items[name]
		
		var icon string
		if item.Done {
			icon = "✓"
		} else if item.Error != nil {
			icon = "✗"
		} else {
			icon = m.spinner.View()
		}

		line := fmt.Sprintf("%s %s", icon, item.Name)
		
		if item.Status != "" && !item.Done {
			line += fmt.Sprintf(" - %s", item.Status)
		}

		if item.TotalBytes > 0 && !item.Done {
			line += fmt.Sprintf(" (%s/%s)", 
				formatBytes(item.CurrentBytes), 
				formatBytes(item.TotalBytes))
		}

		if item.Done {
			b.WriteString(doneStyle.Render(itemStyle.Render(line)))
		} else {
			b.WriteString(itemStyle.Render(line))
		}
		b.WriteString("\n")

		// Show progress bar for active download
		if !item.Done && item.TotalBytes > 0 && item.Status == "Downloading" {
			b.WriteString(progressStyle.Render(m.progress.ViewAs(item.Progress)))
			b.WriteString("\n")
		}
	}

	// Help text
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Press Ctrl+C to cancel"))

	return b.String()
}

func (m LoadingModel) waitForProgress() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(100 * time.Millisecond)
		return nil
	}
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// RunLoadingScreen starts the loading screen UI
func RunLoadingScreen(progressChan <-chan ProgressMsg) error {
	p := tea.NewProgram(NewLoadingModel(), tea.WithAltScreen())
	
	// Start a goroutine to send progress updates to the model
	go func() {
		for msg := range progressChan {
			p.Send(msg)
		}
		p.Send(DoneMsg{})
	}()

	_, err := p.Run()
	return err
}