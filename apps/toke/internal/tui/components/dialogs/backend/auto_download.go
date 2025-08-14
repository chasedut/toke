package backend

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/v2/progress"
	"github.com/charmbracelet/bubbles/v2/spinner"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/backend"
	"github.com/chasedut/toke/internal/config"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	"github.com/chasedut/toke/internal/tui/util"
)

// AutoDownloadDialog is a full-screen loading experience for auto-downloading models
type AutoDownloadDialog struct {
	width       int
	height      int
	
	// Download state
	selectedModel  *backend.ModelOption
	orchestrator   *backend.Orchestrator
	progress       progress.Model
	downloadMsg    string
	bytesDownloaded int64
	totalBytes     int64
	isComplete     bool
	errorMsg       string
	
	// Animation state
	animFrame      int
	flameFrame     int
	smokeFrame     int
	sparkleFrame   int
	spinner        spinner.Model
	
	// Callbacks
	onComplete     func(*backend.ModelOption)
}

// NewAutoDownloadDialog creates a new auto-download dialog that immediately starts downloading
func NewAutoDownloadDialog() dialogs.DialogModel {
	// Auto-select the smallest compatible model
	model := selectSmallestCompatibleModel()
	if model == nil {
		slog.Error("No compatible model found for auto-download")
		return nil
	}
	
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // Orange for fire
	
	dialog := &AutoDownloadDialog{
		selectedModel: model,
		progress:     progress.New(progress.WithDefaultGradient()),
		downloadMsg:  fmt.Sprintf("Downloading %s...", model.Name),
		spinner:      s,
		onComplete: func(m *backend.ModelOption) {
			// Configure the model after download
			cfg := config.Get()
			if err := cfg.ConfigureLocalModel(m); err != nil {
				slog.Error("Failed to configure local model", "error", err)
			}
		},
	}
	
	return dialog
}

// selectSmallestCompatibleModel auto-selects the best model for the user's hardware
func selectSmallestCompatibleModel() *backend.ModelOption {
	models := backend.AvailableModels()
	
	// Get system memory
	totalRAM := getTotalSystemMemory()
	isAppleSilicon := backend.IsAppleSilicon()
	
	slog.Info("Auto-selecting model", "total_ram", backend.FormatSize(totalRAM), "apple_silicon", isAppleSilicon)
	
	// Filter compatible models based on available memory
	var compatibleModels []backend.ModelOption
	for _, model := range models {
		if model.Available && model.Memory <= totalRAM {
			compatibleModels = append(compatibleModels, model)
		}
	}
	
	if len(compatibleModels) == 0 {
		// If no models fit in memory, just pick the smallest one
		smallestSize := int64(1<<63 - 1)
		var smallest *backend.ModelOption
		for i := range models {
			if models[i].Available && models[i].Size < smallestSize {
				smallestSize = models[i].Size
				smallest = &models[i]
			}
		}
		return smallest
	}
	
	// Prefer models in this order:
	// 1. If Apple Silicon and has 8GB+ RAM: Qwen 2.5 Coder 7B MLX (faster)
	// 2. Otherwise: Qwen 2.5 Coder 7B GGUF (universal)
	// 3. If less RAM: Qwen 2.5 3B
	
	if isAppleSilicon && totalRAM >= 8*1024*1024*1024 {
		// Look for Qwen 2.5 Coder MLX
		for i := range compatibleModels {
			if compatibleModels[i].ID == "qwen2.5-coder-7b-4bit" {
				slog.Info("Selected Qwen 2.5 Coder 7B MLX for Apple Silicon")
				return &compatibleModels[i]
			}
		}
	}
	
	// Look for Qwen 2.5 Coder GGUF
	for i := range compatibleModels {
		if compatibleModels[i].ID == "qwen2.5-coder-7b-q4_k_m" {
			slog.Info("Selected Qwen 2.5 Coder 7B GGUF")
			return &compatibleModels[i]
		}
	}
	
	// Fall back to smallest compatible model
	smallestSize := int64(1<<63 - 1)
	var smallest *backend.ModelOption
	for i := range compatibleModels {
		if compatibleModels[i].Size < smallestSize {
			smallestSize = compatibleModels[i].Size
			smallest = &compatibleModels[i]
		}
	}
	
	if smallest != nil {
		slog.Info("Selected smallest compatible model", "model", smallest.Name, "size", backend.FormatSize(smallest.Size))
	}
	return smallest
}

// getTotalSystemMemory returns the total system memory in bytes
func getTotalSystemMemory() int64 {
	// This is a simplified version - in production you'd use proper system calls
	switch runtime.GOOS {
	case "darwin":
		// macOS typically has at least 8GB on modern machines
		// We'll be conservative and assume 8GB minimum
		return 8 * 1024 * 1024 * 1024
	default:
		// Conservative default
		return 8 * 1024 * 1024 * 1024
	}
}

func (d *AutoDownloadDialog) Init() tea.Cmd {
	if d.selectedModel == nil {
		return func() tea.Msg {
			return DownloadErrorMsg{Error: fmt.Errorf("no compatible model found")}
		}
	}
	
	// Start download immediately
	return tea.Batch(
		d.startDownload(),
		d.spinner.Tick,
		animate(),
	)
}

func (d *AutoDownloadDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
		progressWidth := min(60, msg.Width - 20)
		if progressWidth > 0 {
			d.progress.SetWidth(progressWidth)
		}
		return d, nil
		
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if !d.isComplete {
				// Cancel download
				if downloadProgress.cancel != nil {
					downloadProgress.cancel()
				}
				downloadProgress.err = fmt.Errorf("download cancelled by user")
				downloadProgress.done = true
			}
			return d, util.CmdHandler(dialogs.CloseDialogMsg{})
		case "enter":
			if d.isComplete {
				if d.onComplete != nil && d.selectedModel != nil {
					d.onComplete(d.selectedModel)
				}
				return d, util.CmdHandler(dialogs.CloseDialogMsg{})
			}
		}
		
	case AnimateMsg:
		d.animFrame++
		d.flameFrame = (d.flameFrame + 1) % 8
		d.smokeFrame = (d.smokeFrame + 1) % 12
		d.sparkleFrame = (d.sparkleFrame + 1) % 6
		return d, animate()
		
	case spinner.TickMsg:
		var cmd tea.Cmd
		d.spinner, cmd = d.spinner.Update(msg)
		return d, cmd
		
	case DownloadProgressMsg:
		d.bytesDownloaded = msg.Downloaded
		d.totalBytes = msg.Total
		d.downloadMsg = msg.Message
		
		if msg.Total > 0 {
			percent := float64(msg.Downloaded) / float64(msg.Total)
			cmd := d.progress.SetPercent(percent)
			return d, tea.Batch(cmd, d.tickDownloadProgress())
		}
		return d, d.tickDownloadProgress()
		
	case DownloadCompleteMsg:
		d.isComplete = true
		// Start the backend server
		go func() {
			if d.orchestrator != nil {
				ctx := context.Background()
				if err := d.orchestrator.Start(ctx); err != nil {
					slog.Error("Failed to start backend", "error", err)
				}
			}
		}()
		
		// Auto-close after a delay
		return d, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return AutoCloseMsg{}
		})
		
	case DownloadErrorMsg:
		d.errorMsg = msg.Error.Error()
		return d, nil
		
	case AutoCloseMsg:
		if d.onComplete != nil && d.selectedModel != nil {
			d.onComplete(d.selectedModel)
		}
		return d, util.CmdHandler(dialogs.CloseDialogMsg{})
		
	case progress.FrameMsg:
		progressModel, cmd := d.progress.Update(msg)
		d.progress = progressModel
		return d, cmd
	}
	
	return d, nil
}

func (d *AutoDownloadDialog) startDownload() tea.Cmd {
	if d.selectedModel == nil {
		return func() tea.Msg {
			return DownloadErrorMsg{Error: fmt.Errorf("no model selected")}
		}
	}
	
	// Cancel any existing download
	if downloadProgress.cancel != nil {
		downloadProgress.cancel()
	}
	
	// Reset progress
	downloadProgress.modelID = d.selectedModel.ID
	downloadProgress.downloaded = 0
	downloadProgress.total = d.selectedModel.Size
	downloadProgress.message = "Initializing download..."
	downloadProgress.done = false
	downloadProgress.err = nil
	
	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	downloadProgress.cancel = cancel
	
	// Start download in background
	go func() {
		if d.orchestrator == nil {
			dataDir := ".toke"
			d.orchestrator = backend.NewOrchestrator(dataDir)
		}
		
		err := d.orchestrator.SetupModel(
			ctx,
			d.selectedModel,
			func(downloaded, total int64) {
				downloadProgress.downloaded = downloaded
				downloadProgress.total = total
				downloadProgress.message = fmt.Sprintf("Downloading %s... %d%%", 
					d.selectedModel.Name,
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
	
	return tea.Batch(
		func() tea.Msg {
			return DownloadProgressMsg{
				Downloaded: 0,
				Total:      d.selectedModel.Size,
				Message:    "Starting download...",
			}
		},
		d.tickDownloadProgress(),
	)
}

func (d *AutoDownloadDialog) tickDownloadProgress() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		if downloadProgress.err != nil {
			return DownloadErrorMsg{Error: downloadProgress.err}
		}
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

func (d *AutoDownloadDialog) View() string {
	if d.width == 0 || d.height == 0 {
		return ""
	}
	
	// Create full-screen view with ASCII art
	return d.renderFullScreenLoading()
}

func (d *AutoDownloadDialog) renderFullScreenLoading() string {
	var content strings.Builder
	
	// Calculate spacing
	artHeight := 20 // Height of our ASCII art
	progressHeight := 8 // Height for progress info
	totalContentHeight := artHeight + progressHeight
	topPadding := max(0, (d.height - totalContentHeight) / 2)
	
	// Add top padding
	for i := 0; i < topPadding; i++ {
		content.WriteString("\n")
	}
	
	// Render ASCII art with animations
	art := d.renderAnimatedArt()
	content.WriteString(art)
	content.WriteString("\n\n")
	
	// Render download progress
	progressSection := d.renderProgressSection()
	content.WriteString(progressSection)
	
	// Style the entire view
	style := lipgloss.NewStyle().
		Width(d.width).
		Height(d.height).
		AlignHorizontal(lipgloss.Center)
	
	return style.Render(content.String())
}

func (d *AutoDownloadDialog) renderAnimatedArt() string {
	// Colors for animation
	flameColors := []string{"214", "208", "202", "196"} // Orange to red gradient
	smokeColor := "240" // Gray
	jointColor := "94"   // Brown
	cherryColor := "196" // Bright red
	
	// Flame animation based on frame
	flameChar := []string{"🔥", "🔥", "🔥", "🔥"}[d.flameFrame % 4]
	
	// Smoke particles that rise
	smokeParticles := []string{".", "o", "O", "◯", "○", "◌", "◦", "･", "∘", "∙", "⋅"}
	smoke1 := smokeParticles[d.smokeFrame % len(smokeParticles)]
	smoke2 := smokeParticles[(d.smokeFrame + 3) % len(smokeParticles)]
	smoke3 := smokeParticles[(d.smokeFrame + 6) % len(smokeParticles)]
	
	// Create the art
	var art strings.Builder
	
	// Smoke trail (animated upward movement)
	smokeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(smokeColor))
	offset := d.smokeFrame % 3
	
	// Multiple smoke lines for effect
	for i := 0; i < 3; i++ {
		spaces := strings.Repeat(" ", 35 + offset + i*2)
		if rand.Float32() < 0.7 { // Random smoke particles
			art.WriteString(spaces + smokeStyle.Render(smoke1) + "\n")
		} else {
			art.WriteString("\n")
		}
	}
	
	art.WriteString(strings.Repeat(" ", 34) + smokeStyle.Render(smoke2) + "  " + smokeStyle.Render(smoke3) + "\n")
	art.WriteString(strings.Repeat(" ", 33) + smokeStyle.Render(smoke1) + "    " + smokeStyle.Render(smoke2) + "\n")
	
	// The joint with glowing cherry
	jointStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(jointColor))
	cherryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(cherryColor)).Bold(true)
	
	// Animated cherry glow
	cherryGlow := ""
	if d.animFrame % 2 == 0 {
		cherryGlow = cherryStyle.Render("◉")
	} else {
		cherryGlow = cherryStyle.Render("●")
	}
	
	art.WriteString(strings.Repeat(" ", 30) + cherryGlow + jointStyle.Render("═══════╗") + "\n")
	art.WriteString(strings.Repeat(" ", 30) + jointStyle.Render("        ╚═") + "\n")
	
	// Lighter with flame
	lighterBody := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	flameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(flameColors[d.flameFrame % len(flameColors)]))
	
	// Animated flame
	art.WriteString(strings.Repeat(" ", 28) + flameStyle.Render(flameChar) + "\n")
	art.WriteString(strings.Repeat(" ", 27) + lighterBody.Render("╔═╗") + "\n")
	art.WriteString(strings.Repeat(" ", 27) + lighterBody.Render("║ ║") + "\n")
	art.WriteString(strings.Repeat(" ", 27) + lighterBody.Render("╚═╝") + "\n")
	
	// Sparkles around the scene
	if d.sparkleFrame < 3 {
		sparkleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
		art.WriteString(strings.Repeat(" ", 20) + sparkleStyle.Render("✨") + strings.Repeat(" ", 20) + sparkleStyle.Render("✨") + "\n")
	} else {
		art.WriteString("\n")
	}
	
	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("70")).
		Bold(true)
	
	art.WriteString("\n")
	art.WriteString(titleStyle.Render("🌿 Setting up your local AI...") + "\n")
	
	return art.String()
}

func (d *AutoDownloadDialog) renderProgressSection() string {
	var content strings.Builder
	
	if d.errorMsg != "" {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		content.WriteString(errorStyle.Render("Error: " + d.errorMsg) + "\n")
		content.WriteString("\nPress ESC to close")
		return content.String()
	}
	
	if d.isComplete {
		successStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("70")).
			Bold(true)
		content.WriteString(successStyle.Render("✅ Download complete!") + "\n\n")
		content.WriteString("Your AI is ready to use. Closing in a moment...")
		return content.String()
	}
	
	// Model info
	if d.selectedModel != nil {
		infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
		content.WriteString(infoStyle.Render(fmt.Sprintf("Model: %s", d.selectedModel.Name)) + "\n")
		content.WriteString(infoStyle.Render(fmt.Sprintf("Size: %s", backend.FormatSize(d.selectedModel.Size))) + "\n\n")
	}
	
	// Progress message
	msgStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	content.WriteString(msgStyle.Render(d.downloadMsg) + "\n\n")
	
	// Progress bar
	content.WriteString(d.progress.View() + "\n\n")
	
	// Download stats
	if d.totalBytes > 0 {
		statsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		stats := fmt.Sprintf("%s / %s",
			backend.FormatSize(d.bytesDownloaded),
			backend.FormatSize(d.totalBytes))
		content.WriteString(statsStyle.Render(stats) + "\n")
	}
	
	// Hint
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
	content.WriteString("\n" + hintStyle.Render("Press ESC to cancel"))
	
	return content.String()
}

// Position returns the position of the dialog (full-screen, so 0, 0)
func (d *AutoDownloadDialog) Position() (int, int) {
	return 0, 0
}

// ID returns the dialog ID
func (d *AutoDownloadDialog) ID() dialogs.DialogID {
	return "auto_download"
}