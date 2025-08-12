package backend

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/v2/progress"
	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/backend"
	"github.com/chasedut/toke/internal/config"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
)

type State int

const (
	StateSelection State = iota
	StateHFBrowse      // Browse Hugging Face models
	StateHFSearch      // Search Hugging Face models
	StateHFFileSelect  // Select specific file from HF model
	StateDownloading
	StateComplete
	StateError
)

type Model struct {
	state       State
	width       int
	height      int
	
	// Selection state
	selectedOption int
	selectedModel  *backend.ModelOption
	showAdvanced   bool
	isLocalOnly    bool  // If true, only show local model options
	
	// Hugging Face state
	hfClient       *backend.HuggingFaceClient
	hfModels       []backend.HuggingFaceModel
	hfSelectedIdx  int
	hfSearchInput  textinput.Model
	hfSelectedFile string
	hfFiles        []backend.HuggingFaceFile
	hfFileIdx      int
	hfLoading      bool
	
	// Download state
	progress     progress.Model
	downloadMsg  string
	bytesDownloaded int64
	totalBytes     int64
	
	// Error state
	errorMsg     string
	
	// Backend orchestrator
	orchestrator *backend.Orchestrator
	
	// Callbacks
	onComplete   func(model *backend.ModelOption)
	onSkip       func()
	onAPIKey     func()
}

type Msg struct{}

type DownloadProgressMsg struct {
	Downloaded int64
	Total      int64
	Message    string
}

type DownloadCompleteMsg struct{}

type DownloadErrorMsg struct {
	Error error
}

type AutoCloseMsg struct{}

type HFModelsLoadedMsg struct {
	Models []backend.HuggingFaceModel
}

type HFFilesLoadedMsg struct {
	Files []backend.HuggingFaceFile
}

type HFErrorMsg struct {
	Error error
}

func New(orchestrator *backend.Orchestrator, onComplete func(*backend.ModelOption), onSkip func(), onAPIKey func()) Model {
	searchInput := textinput.New()
	searchInput.Placeholder = "Search for models (e.g., 'llama', 'mistral', 'qwen')..."
	searchInput.CharLimit = 100
	
	return Model{
		state:        StateSelection,
		progress:     progress.New(progress.WithDefaultGradient()),
		orchestrator: orchestrator,
		onComplete:   onComplete,
		onSkip:       onSkip,
		onAPIKey:     onAPIKey,
		isLocalOnly:  false,  // Show all options for initial setup
		hfClient:     backend.NewHuggingFaceClient(),
		hfSearchInput: searchInput,
	}
}

// NewLocalOnly creates a model setup dialog for local models only
func NewLocalOnly(orchestrator *backend.Orchestrator, onComplete func(*backend.ModelOption), onSkip func()) Model {
	searchInput := textinput.New()
	searchInput.Placeholder = "Search for models (e.g., 'llama', 'mistral', 'qwen')..."
	searchInput.CharLimit = 100
	
	return Model{
		state:        StateSelection,
		progress:     progress.New(progress.WithDefaultGradient()),
		orchestrator: orchestrator,
		onComplete:   onComplete,
		onSkip:       onSkip,
		isLocalOnly:  true,  // Only show local model options
		hfClient:     backend.NewHuggingFaceClient(),
		hfSearchInput: searchInput,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.SetWidth(msg.Width - 20)
		return m, nil
		
	case tea.KeyMsg:
		switch m.state {
		case StateSelection:
			return m.handleSelectionKeys(msg)
		case StateHFBrowse:
			return m.handleHFBrowseKeys(msg)
		case StateHFSearch:
			return m.handleHFSearchKeys(msg)
		case StateHFFileSelect:
			return m.handleHFFileSelectKeys(msg)
		case StateError:
			switch msg.String() {
			case "enter", "esc":
				m.state = StateSelection
				m.errorMsg = ""
			}
		case StateComplete:
			switch msg.String() {
			case "enter":
				if m.onComplete != nil && m.selectedModel != nil {
					m.onComplete(m.selectedModel)
				}
			}
		}
		
	case DownloadProgressMsg:
		m.bytesDownloaded = msg.Downloaded
		m.totalBytes = msg.Total
		m.downloadMsg = msg.Message
		
		if msg.Total > 0 {
			percent := float64(msg.Downloaded) / float64(msg.Total)
			cmd := m.progress.SetPercent(percent)
			// Continue ticking for more progress updates
			return m, tea.Batch(cmd, m.tickDownloadProgress())
		}
		return m, m.tickDownloadProgress()
		
	case DownloadCompleteMsg:
		m.state = StateComplete
		// Start the backend server
		go func() {
			if m.orchestrator != nil {
				ctx := context.Background()
				if err := m.orchestrator.Start(ctx); err != nil {
					slog.Error("Failed to start backend", "error", err)
				} else {
					slog.Info("Backend server started successfully")
				}
			}
		}()
		
		// Configure Toke to use the local model
		cfg := config.Get()
		if err := cfg.ConfigureLocalModel(m.selectedModel); err != nil {
			slog.Error("Failed to configure local model", "error", err)
		}
		
		// Close dialog after a short delay or when ENTER is pressed
		return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return AutoCloseMsg{}
		})
		
	case DownloadErrorMsg:
		m.state = StateError
		m.errorMsg = msg.Error.Error()
		return m, nil
		
	case AutoCloseMsg:
		// Auto-close the dialog
		if m.onComplete != nil && m.selectedModel != nil {
			m.onComplete(m.selectedModel)
		}
		return m, nil
		
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel
		return m, cmd
		
	case HFModelsLoadedMsg:
		m.hfModels = msg.Models
		m.hfLoading = false
		m.hfSelectedIdx = 0
		if m.state == StateHFSearch {
			// After search, switch to browse mode
			m.state = StateHFBrowse
			m.hfSearchInput.Blur()
		}
		return m, nil
		
	case HFFilesLoadedMsg:
		m.hfFiles = msg.Files
		m.hfLoading = false
		m.hfFileIdx = 0
		return m, nil
		
	case HFErrorMsg:
		m.state = StateError
		m.errorMsg = fmt.Sprintf("Hugging Face error: %v", msg.Error)
		m.hfLoading = false
		return m, nil
		
	}
	
	return m, nil
}

func (m Model) handleSelectionKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.selectedOption--
		if m.selectedOption < 0 {
			if m.showAdvanced {
				m.selectedOption = 8 // Wrap to bottom (GLM option)
			} else {
				m.selectedOption = 3 // Wrap to "Skip"
			}
		}
		
	case "down", "j":
		m.selectedOption++
		maxOption := 3
		if m.showAdvanced {
			maxOption = 8
		}
		if m.selectedOption > maxOption {
			m.selectedOption = 0
		}
		
	case "tab":
		// Toggle advanced options
		m.showAdvanced = !m.showAdvanced
		if m.selectedOption > 3 && !m.showAdvanced {
			m.selectedOption = 0
		}
		
	case "enter":
		// Adjust indices based on whether API key option is shown
		actualOption := m.selectedOption
		if m.isLocalOnly && m.selectedOption > 0 {
			// In local-only mode, option 1 (API key) doesn't exist
			// So option 1 becomes "Browse models", option 2 becomes "Skip", etc.
			actualOption++
		}
		
		switch actualOption {
		case 0: // Recommended model
			m.selectedModel = backend.GetRecommendedModel()
			m.state = StateDownloading
			return m, m.startDownload()
			
		case 1: // API key (only in non-local mode)
			if !m.isLocalOnly && m.onAPIKey != nil {
				m.onAPIKey()
			}
			
		case 2: // Browse models
			m.showAdvanced = true
			
		case 3: // Skip
			if m.onSkip != nil {
				m.onSkip()
			}
			
		case 4: // Browse recent HF models (in advanced)
			m.state = StateHFBrowse
			m.hfLoading = true
			return m, m.loadRecentModels()
			
		case 5: // Search HF models (in advanced)
			m.state = StateHFSearch
			m.hfSearchInput.Focus()
			return m, textinput.Blink
			
		case 6: // Balanced model (in advanced)
			m.selectedModel = backend.GetRecommendedByTier(backend.TierBalanced)
			m.state = StateDownloading
			return m, m.startDownload()
			
		case 7: // Light model (in advanced)
			m.selectedModel = backend.GetModelByID("qwen2.5-3b-q4_k_m")
			m.state = StateDownloading
			return m, m.startDownload()
			
		case 8: // GLM-4.5-Air (in advanced)
			m.selectedModel = backend.GetModelByID("glm-4.5-air-iq2_m")
			m.state = StateDownloading
			return m, m.startDownload()
		}
		
	case "esc":
		if m.onSkip != nil {
			m.onSkip()
		}
	}
	
	return m, nil
}

// Global variables to track download progress
var (
	downloadProgress = struct {
		downloaded int64
		total      int64
		message    string
		done       bool
		err        error
	}{}
)

func (m Model) startDownload() tea.Cmd {
	// Reset progress
	downloadProgress.downloaded = 0
	downloadProgress.total = m.selectedModel.Size
	downloadProgress.message = "Starting download..."
	downloadProgress.done = false
	downloadProgress.err = nil
	
	// Start download in background
	go func() {
		if m.orchestrator == nil {
			// Create orchestrator if not provided
			dataDir := ".toke"
			m.orchestrator = backend.NewOrchestrator(dataDir)
		}
		
		// Setup the model with progress callback
		err := m.orchestrator.SetupModel(
			context.Background(),
			m.selectedModel,
			func(downloaded, total int64) {
				// Update global progress
				downloadProgress.downloaded = downloaded
				downloadProgress.total = total
				downloadProgress.message = fmt.Sprintf("Downloading %s... %d%%", 
					m.selectedModel.Name,
					int(100*downloaded/total))
			},
		)
		
		if err != nil {
			downloadProgress.err = err
		} else {
			downloadProgress.done = true
			downloadProgress.message = "Download complete!"
		}
	}()
	
	// Return a ticker to poll progress
	return m.tickDownloadProgress()
}

func (m Model) tickDownloadProgress() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		if downloadProgress.done {
			return DownloadCompleteMsg{}
		}
		if downloadProgress.err != nil {
			return DownloadErrorMsg{Error: downloadProgress.err}
		}
		return DownloadProgressMsg{
			Downloaded: downloadProgress.downloaded,
			Total:      downloadProgress.total,
			Message:    downloadProgress.message,
		}
	})
}

func (m Model) View() string {
	switch m.state {
	case StateSelection:
		return m.renderSelection()
	case StateHFBrowse:
		return m.renderHFBrowse()
	case StateHFSearch:
		return m.renderHFSearch()
	case StateHFFileSelect:
		return m.renderHFFileSelect()
	case StateDownloading:
		return m.renderDownloading()
	case StateComplete:
		return m.renderComplete()
	case StateError:
		return m.renderError()
	default:
		return ""
	}
}

// Position returns the position of the dialog
func (m Model) Position() (int, int) {
	return 0, 0
}

// ID returns the dialog ID
func (m Model) ID() dialogs.DialogID {
	return "backend_setup"
}

func (m Model) renderSelection() string {
	var s strings.Builder
	
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("70")).
		MarginBottom(1).
		Render("🌿 Welcome to Toke!")
	
	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		MarginBottom(2).
		Render("Choose a local model to download:")
	
	if !m.isLocalOnly {
		subtitle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginBottom(2).
			Render("No AI providers configured. Choose an option to get started:")
	}
	
	s.WriteString(title + "\n")
	s.WriteString(subtitle + "\n\n")
	
	// Main options
	var options []struct {
		label string
		desc  string
	}
	
	// Always show recommended model first
	options = append(options, struct {
		label string
		desc  string
	}{
		"Download Qwen 2.5 Coder 7B (4GB) - Recommended",
		"Best coding model, runs locally with complete privacy",
	})
	
	// Only show API key option if not in local-only mode
	if !m.isLocalOnly {
		options = append(options, struct {
			label string
			desc  string
		}{
			"Enter API key for cloud provider",
			"Use OpenAI, Anthropic, or other cloud services",
		})
	}
	
	// Browse models option
	options = append(options, struct {
		label string
		desc  string
	}{
		"Browse other models",
		"See more local model options",
	})
	
	// Skip option
	options = append(options, struct {
		label string
		desc  string
	}{
		"Skip for now",
		"Configure later in settings",
	})
	
	// Add advanced options if showing
	if m.showAdvanced {
		s.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1).
			Render("\nAdvanced Options:") + "\n\n")
			
		options = append(options, []struct {
			label string
			desc  string
		}{
			{
				"Browse recent Hugging Face models 🆕",
				"See the latest models uploaded to Hugging Face",
			},
			{
				"Search Hugging Face models 🔍",
				"Search for specific models by name or type",
			},
			{
				"Qwen 2.5 14B (8GB) - Balanced",
				"Larger model with better reasoning",
			},
			{
				"Qwen 2.5 3B (2GB) - Light",
				"Smaller, faster model for simple tasks",
			},
			{
				"GLM 4.5 Air (44GB) - Power User",
				"Massive 107B parameter model (64GB RAM required)",
			},
		}...)
	}
	
	// Render options
	for i, opt := range options {
		cursor := "  "
		if i == m.selectedOption {
			cursor = "→ "
		}
		
		label := opt.label
		if i == m.selectedOption {
			label = lipgloss.NewStyle().
				Foreground(lipgloss.Color("70")).
				Bold(true).
				Render(label)
		}
		
		desc := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			PaddingLeft(2).
			Render(opt.desc)
		
		s.WriteString(cursor + label + "\n")
		if i == m.selectedOption && opt.desc != "" {
			s.WriteString("  " + desc + "\n")
		}
		s.WriteString("\n")
	}
	
	// Footer
	if !m.showAdvanced {
		footer := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(2).
			Render("Press TAB to show advanced options • ESC to skip")
		s.WriteString(footer)
	} else {
		footer := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(2).
			Render("Press TAB to hide advanced options • ESC to skip")
		s.WriteString(footer)
	}
	
	return s.String()
}

func (m Model) renderDownloading() string {
	var s strings.Builder
	
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("70")).
		MarginBottom(2).
		Render("📦 Setting up your local AI...")
	
	s.WriteString(title + "\n\n")
	
	if m.selectedModel != nil {
		modelInfo := fmt.Sprintf("Model: %s\nSize: %s\n\n", 
			m.selectedModel.Name,
			backend.FormatSize(m.selectedModel.Size))
		s.WriteString(modelInfo)
	}
	
	s.WriteString(m.downloadMsg + "\n\n")
	s.WriteString(m.progress.View() + "\n\n")
	
	if m.totalBytes > 0 {
		stats := fmt.Sprintf("%s / %s",
			backend.FormatSize(m.bytesDownloaded),
			backend.FormatSize(m.totalBytes))
		s.WriteString(stats)
	}
	
	return s.String()
}

func (m Model) renderComplete() string {
	var s strings.Builder
	
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("70")).
		MarginBottom(2).
		Render("🎉 Success! You now have AI running locally!")
	
	s.WriteString(title + "\n\n")
	
	if m.selectedModel != nil {
		info := fmt.Sprintf(
			"Model: %s\n"+
			"• No internet required\n"+
			"• No API keys needed\n"+
			"• Complete privacy\n"+
			"• Ready to use!\n",
			m.selectedModel.Name)
		s.WriteString(info)
	}
	
	s.WriteString("\n\nClosing in a moment... (or press ENTER)")
	
	return s.String()
}

func (m Model) renderError() string {
	var s strings.Builder
	
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")).
		MarginBottom(2).
		Render("❌ Setup Error")
	
	s.WriteString(title + "\n\n")
	s.WriteString("An error occurred during setup:\n\n")
	s.WriteString(m.errorMsg + "\n\n")
	s.WriteString("Press ENTER to try again or ESC to skip...")
	
	return s.String()
}