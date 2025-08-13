package backend

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/v2/progress"
	"github.com/charmbracelet/bubbles/v2/spinner"
	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/backend"
	"github.com/chasedut/toke/internal/config"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	"github.com/chasedut/toke/internal/tui/util"
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
	
	// Animation state
	animFrame    int
	flameFrame   int
	spinner      spinner.Model
	
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

type AnimateMsg struct{}

type MinimizeToBackgroundMsg struct{
	Model      *backend.ModelOption
	Progress   float64
	Downloaded int64
	Total      int64
}

type BackgroundDownloadProgressMsg struct{
	Model      *backend.ModelOption
	Downloaded int64
	Total      int64
	Message    string
}

type ShowDownloadModalMsg struct{
	Model *backend.ModelOption
}

func New(orchestrator *backend.Orchestrator, onComplete func(*backend.ModelOption), onSkip func(), onAPIKey func()) Model {
	searchInput := textinput.New()
	searchInput.Placeholder = "Search for models (e.g., 'llama', 'mistral', 'qwen')..."
	searchInput.CharLimit = 100
	
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	
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
		spinner:      s,
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

// NewWithModel creates a new Model dialog that immediately starts downloading a specific model
func NewWithModel(orchestrator *backend.Orchestrator, model *backend.ModelOption, onComplete func(*backend.ModelOption)) Model {
	m := Model{
		state:         StateDownloading,
		progress:      progress.New(progress.WithDefaultGradient()),
		orchestrator:  orchestrator,
		onComplete:    onComplete,
		isLocalOnly:   true,
		selectedModel: model,
		downloadMsg:   fmt.Sprintf("Downloading %s...", model.Name),
		hfClient:      backend.NewHuggingFaceClient(),
	}
	
	// If this model's download is already in progress, restore the state
	if downloadProgress.modelID == model.ID && 
	   !downloadProgress.done && 
	   downloadProgress.err == nil {
		m.bytesDownloaded = downloadProgress.downloaded
		m.totalBytes = downloadProgress.total
		if downloadProgress.message != "" {
			m.downloadMsg = downloadProgress.message
		}
		// Note: We'll set the progress percentage in Init() where we can return the command
	}
	
	return m
}

func (m Model) Init() tea.Cmd {
	// If we're starting in download state
	if m.state == StateDownloading && m.selectedModel != nil {
		// Check if this exact model's download is already in progress (resuming)
		// The key check is whether the modelID matches - if it does, the download
		// was already started and we should just resume monitoring it
		if downloadProgress.modelID == m.selectedModel.ID && 
		   !downloadProgress.done && 
		   downloadProgress.err == nil {
			// Download is already in progress for this model, just start the ticker
			// Also restore the current progress to the model
			m.bytesDownloaded = downloadProgress.downloaded
			m.totalBytes = downloadProgress.total
			if m.totalBytes == 0 {
				// If total is not set yet, use the model's size
				m.totalBytes = m.selectedModel.Size
				downloadProgress.total = m.selectedModel.Size
			}
			m.downloadMsg = downloadProgress.message
			if m.downloadMsg == "" {
				m.downloadMsg = fmt.Sprintf("Downloading %s...", m.selectedModel.Name)
			}
			
			// Set the progress bar percentage and get the command
			var progressCmd tea.Cmd
			if m.totalBytes > 0 {
				percent := float64(m.bytesDownloaded) / float64(m.totalBytes)
				progressCmd = m.progress.SetPercent(percent)
			}
			
			// Send an immediate progress update to trigger the UI refresh
			immediateProgressCmd := func() tea.Msg {
				return DownloadProgressMsg{
					Downloaded: m.bytesDownloaded,
					Total:      m.totalBytes,
					Message:    m.downloadMsg,
				}
			}
			
			return tea.Batch(
				progressCmd,
				immediateProgressCmd, // Trigger immediate update
				m.tickDownloadProgress(),
				m.spinner.Tick,
				animate(),
			)
		}
		// Start a new download (either a different model or no download in progress)
		return tea.Batch(
			m.startDownload(),
			m.spinner.Tick,
			animate(),
		)
	}
	return nil
}

// animate returns a command that triggers animation
func animate() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
		return AnimateMsg{}
	})
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
		case StateDownloading:
			return m.handleDownloadingKeys(msg)
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
		
	case AnimateMsg:
		m.animFrame++
		m.flameFrame = (m.flameFrame + 1) % 4
		return m, animate()
		
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
		
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
		// Close the dialog
		return m, util.CmdHandler(dialogs.CloseDialogMsg{})
		
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel
		return m, cmd
		
	case HFModelsLoadedMsg:
		m.hfModels = msg.Models
		m.hfLoading = false
		m.hfSelectedIdx = 0
		// After search completes, switch to browse mode to show results
		if m.state == StateHFSearch {
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

func (m Model) handleDownloadingKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "b":
		// Minimize to background - close dialog but continue download
		if m.selectedModel != nil {
			var progress float64
			if m.totalBytes > 0 {
				progress = float64(m.bytesDownloaded) / float64(m.totalBytes)
			}
			// Send both the minimize message and close the dialog
			return m, tea.Batch(
				util.CmdHandler(MinimizeToBackgroundMsg{
					Model:      m.selectedModel,
					Progress:   progress,
					Downloaded: m.bytesDownloaded,
					Total:      m.totalBytes,
				}),
				util.CmdHandler(dialogs.CloseDialogMsg{}),
				// Continue ticking for background updates
				m.tickBackgroundDownload(),
			)
		}
	case "esc":
		// Cancel download - actually cancel the context to stop the download
		if downloadProgress.cancel != nil {
			downloadProgress.cancel() // This will cancel the actual download operation
		}
		// Set an error to indicate cancellation rather than completion
		downloadProgress.err = fmt.Errorf("download cancelled by user")
		downloadProgress.done = true // Mark as done with error
		// Clear the download from background manager if present
		if m.selectedModel != nil {
			// Note: we'll need to handle this in the main TUI
		}
		return m, util.CmdHandler(dialogs.CloseDialogMsg{})
	}
	return m, nil
}

func (m Model) handleSelectionKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.selectedOption--
		if m.selectedOption < 0 {
			// Wrap to bottom based on platform
			if backend.IsAppleSilicon() {
				m.selectedOption = 11 // 3 main options + Recommended + HF options + 6 MLX/GGUF models
			} else {
				m.selectedOption = 7 // 3 main options + Recommended + HF options + 3 GGUF models
			}
		}
		
	case "down", "j":
		m.selectedOption++
		// Count all options
		var maxOption int
		if backend.IsAppleSilicon() {
			maxOption = 11 // 3 main + Recommended + HF options + 6 MLX/GGUF models
		} else {
			maxOption = 7 // 3 main + Recommended + HF options + 3 GGUF models
		}
		if m.selectedOption > maxOption {
			m.selectedOption = 0
		}
		
		
	case "enter":
		switch m.selectedOption {
		case 0: // Download Local Model (section header - do nothing)
			return m, nil
			
		case 1: // Setup API Provider
			// Open the models dialog to setup API
			if m.onAPIKey != nil {
				m.onAPIKey()
			}
			
		case 2: // Cancel
			if m.onSkip != nil {
				m.onSkip()
			}
			// Close the dialog
			return m, util.CmdHandler(dialogs.CloseDialogMsg{})
			
		default:
			// Handle model selection options
			if m.selectedOption >= 3 {
				advancedIdx := m.selectedOption - 3
				
				// First option is recommended model
				if advancedIdx == 0 {
					// Recommended Qwen model
					m.selectedModel = backend.GetRecommendedModel()
					m.state = StateDownloading
					return m, m.startDownload()
				} else if advancedIdx == 1 {
					// Browse recent HF models
					m.state = StateHFBrowse
					m.hfLoading = true
					return m, m.loadRecentModels()
				} else if advancedIdx == 2 {
					// Search HF models
					m.state = StateHFSearch
					m.hfSearchInput.Focus()
					return m, textinput.Blink
				}
				
				// MLX models are now always shown at indices 3-6
				// Check if user selected an MLX model
				if advancedIdx >= 3 && advancedIdx <= 6 {
					// MLX model selected
					if !backend.IsAppleSilicon() {
						// Show error for non-Apple Silicon
						m.state = StateError
						m.errorMsg = "This model requires Apple Silicon (M1/M2/M3/M4) Mac"
						return m, nil
					}
					
					// Handle MLX model selection
					switch advancedIdx {
					case 3:
						// Qwen 2.5 MLX
						m.selectedModel = backend.GetModelByID("qwen2.5-coder-7b-4bit")
					case 4:
						// GLM 4.5 Air 3-bit MLX
						m.selectedModel = backend.GetModelByID("glm-4.5-air-3bit")
					case 5:
						// GLM 4.5 Air 4-bit MLX  
						m.selectedModel = backend.GetModelByID("glm-4.5-air-4bit")
					case 6:
						// GLM 4.5 Air 8-bit MLX
						m.selectedModel = backend.GetModelByID("glm-4.5-air-8bit")
					}
					
					if m.selectedModel != nil {
						m.state = StateDownloading
						return m, m.startDownload()
					}
				} else if advancedIdx == 7 {
					// Qwen 2.5 14B Balanced GGUF
					m.selectedModel = backend.GetRecommendedByTier(backend.TierBalanced)
					m.state = StateDownloading
					return m, m.startDownload()
				} else if advancedIdx == 8 {
					// Qwen 2.5 3B Light GGUF
					m.selectedModel = backend.GetModelByID("qwen2.5-3b-q4_k_m")
					m.state = StateDownloading
					return m, m.startDownload()
				} else if advancedIdx == 9 {
					// GLM 4.5 Air GGUF
					m.selectedModel = backend.GetModelByID("glm-4.5-air-q2_k")
					m.state = StateDownloading
					return m, m.startDownload()
				}
			}
		}
		
	case "esc":
		if m.onSkip != nil {
			m.onSkip()
		}
		// Close the dialog
		return m, util.CmdHandler(dialogs.CloseDialogMsg{})
	}
	
	return m, nil
}

// Global variables to track download progress
var (
	downloadProgress = struct {
		modelID    string // Track which model is downloading
		downloaded int64
		total      int64
		message    string
		done       bool
		err        error
		cancel     context.CancelFunc // Function to cancel the download
	}{}
)

func (m Model) startDownload() tea.Cmd {
	// Check if model is selected
	if m.selectedModel == nil {
		return func() tea.Msg {
			return DownloadErrorMsg{Error: fmt.Errorf("no model selected")}
		}
	}
	
	// Cancel any existing download
	if downloadProgress.cancel != nil {
		downloadProgress.cancel()
	}
	
	// Reset progress
	downloadProgress.modelID = m.selectedModel.ID
	downloadProgress.downloaded = 0
	downloadProgress.total = m.selectedModel.Size
	downloadProgress.message = "Starting download..."
	downloadProgress.done = false
	downloadProgress.err = nil
	
	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	downloadProgress.cancel = cancel
	
	// Start download in background
	go func() {
		if m.orchestrator == nil {
			// Create orchestrator if not provided
			dataDir := ".toke"
			m.orchestrator = backend.NewOrchestrator(dataDir)
		}
		
		// Setup the model with progress callback
		err := m.orchestrator.SetupModel(
			ctx,
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
	
	// Send an immediate progress message to ensure UI updates
	immediateProgress := func() tea.Msg {
		return DownloadProgressMsg{
			Downloaded: 0,
			Total:      m.selectedModel.Size,
			Message:    "Starting download...",
		}
	}
	
	// Return both immediate update and ticker
	return tea.Batch(
		immediateProgress,
		m.tickDownloadProgress(),
	)
}

func (m Model) tickDownloadProgress() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		// Check for error first (including cancellation)
		if downloadProgress.err != nil {
			return DownloadErrorMsg{Error: downloadProgress.err}
		}
		// Only return complete if done without error
		if downloadProgress.done {
			return DownloadCompleteMsg{}
		}
		return DownloadProgressMsg{
			Downloaded: downloadProgress.downloaded,
			Total:      downloadProgress.total,
			Message:    downloadProgress.message,
		}
	})
}

// tickBackgroundDownload continues to monitor download progress after minimizing
func (m Model) tickBackgroundDownload() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		// Check for error first (including cancellation)
		if downloadProgress.err != nil {
			return DownloadErrorMsg{Error: downloadProgress.err}
		}
		// Only return complete if done without error
		if downloadProgress.done {
			// Send completion notification to main UI
			return DownloadCompleteMsg{}
		}
		// Send progress updates to main UI for background status
		return BackgroundDownloadProgressMsg{
			Model:      m.selectedModel,
			Downloaded: downloadProgress.downloaded,
			Total:      downloadProgress.total,
			Message:    downloadProgress.message,
		}
	})
}

// ContinueBackgroundTicker continues the background download ticker
func ContinueBackgroundTicker(model *backend.ModelOption) tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		// Check for error first (including cancellation)
		if downloadProgress.err != nil {
			return DownloadErrorMsg{Error: downloadProgress.err}
		}
		// Only return complete if done without error
		if downloadProgress.done {
			// Send completion notification to main UI
			return DownloadCompleteMsg{}
		}
		// Send progress updates to main UI for background status
		return BackgroundDownloadProgressMsg{
			Model:      model,
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
		return m.renderDownloadingAnimated()
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
		Render("🌿 New Model")
	
	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		MarginBottom(2).
		Render("Choose how to add a new model:")
	
	s.WriteString(title + "\n")
	s.WriteString(subtitle + "\n\n")
	
	// All options in a single list
	var options []struct {
		label string
		desc  string
		isSection bool
	}
	
	// Section header for local models
	options = append(options, struct {
		label string
		desc  string
		isSection bool
	}{
		"── Download Local Models ──",
		"",
		true,
	})
	
	// Add action options
	options = append(options, struct {
		label string
		desc  string
		isSection bool
	}{
		"🔑 Setup API Provider",
		"Use OpenAI, Anthropic, or other cloud services",
		false,
	})
	
	options = append(options, struct {
		label string
		desc  string
		isSection bool
	}{
		"❌ Cancel",
		"Return to chat",
		false,
	})
	
	// Add local model options
	options = append(options, struct {
		label string
		desc  string
		isSection bool
	}{
		"Qwen 2.5 Coder 7B (4GB) - Recommended",
		"Best coding model, runs locally with complete privacy",
		false,
	})
	
	options = append(options, struct {
		label string
		desc  string
		isSection bool
	}{
		"Browse recent Hugging Face models 🆕",
		"See the latest models uploaded to Hugging Face",
		false,
	})
	
	options = append(options, struct {
		label string
		desc  string
		isSection bool
	}{
		"Search Hugging Face models 🔍",
		"Search for specific models by name or type",
		false,
	})
	
	// Add MLX models - show all but mark unavailable if not on Apple Silicon
	isAppleSilicon := backend.IsAppleSilicon()
	
	mlxSuffix := " 🍎"
	if !isAppleSilicon {
		mlxSuffix = " 🍎 [Requires Apple Silicon]"
	}
	
	options = append(options, struct {
		label string
		desc  string
		isSection bool
	}{
		"Qwen 2.5 Coder 7B (MLX)" + mlxSuffix,
		"5GB MLX model - 20-30% faster than GGUF on Apple Silicon",
		false,
	})
	options = append(options, struct {
		label string
		desc  string
		isSection bool
	}{
		"GLM 4.5 Air 3-bit (MLX)" + mlxSuffix,
		"13GB model - Most efficient 106B for 16GB RAM Macs",
		false,
	})
	options = append(options, struct {
		label string
		desc  string
		isSection bool
	}{
		"GLM 4.5 Air 4-bit (MLX)" + mlxSuffix,
		"56GB model - Balanced 106B for 24GB+ RAM Macs",
		false,
	})
	options = append(options, struct {
		label string
		desc  string
		isSection bool
	}{
		"GLM 4.5 Air 8-bit (MLX)" + mlxSuffix,
		"34GB model - Highest quality 106B for 48GB+ RAM Macs",
		false,
	})
	
	// Add standard GGUF models
	options = append(options, struct {
		label string
		desc  string
		isSection bool
	}{
		"Qwen 2.5 14B (8GB) - Balanced",
		"Larger model with better reasoning",
		false,
	})
	options = append(options, struct {
		label string
		desc  string
		isSection bool
	}{
		"Qwen 2.5 3B (2GB) - Light",
		"Smaller, faster model for simple tasks",
		false,
	})
	options = append(options, struct {
		label string
		desc  string
		isSection bool
	}{
		"GLM 4.5 Air Q2_K (44GB) - Power User",
		"Massive 107B parameter model (64GB RAM required)",
		false,
	})
	
	// Render options
	for i, opt := range options {
		if opt.isSection {
			// Render section header
			sectionStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Bold(true)
			s.WriteString("\n" + sectionStyle.Render(opt.label) + "\n\n")
		} else {
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
	}
	
	// Footer
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		MarginTop(2).
		Render("Press ENTER to select • ESC to cancel")
	s.WriteString(footer)
	
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