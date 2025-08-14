package models

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/backend"
	"github.com/chasedut/toke/internal/config"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	backendDlg "github.com/chasedut/toke/internal/tui/components/dialogs/backend"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
)

type HFBrowseState int

const (
	HFStateList HFBrowseState = iota
	HFStateSearch
	HFStateFileSelect
	HFStateLoading
)

type HFBrowseCmp struct {
	state         HFBrowseState
	theme         *styles.Theme
	width         int
	height        int
	selectedIdx   int
	fileIdx       int
	
	// HF data
	hfClient      *backend.HuggingFaceClient
	hfModels      []backend.HuggingFaceModel
	hfFiles       []backend.HuggingFaceFile
	searchInput   textinput.Model
	loading       bool
	errorMsg      string
	
	// Selected model for download
	selectedModel *backend.HuggingFaceModel
	selectedFile  string
}

type HFModelsLoadedMsg struct {
	Models []backend.HuggingFaceModel
}

type HFFilesLoadedMsg struct {
	Files []backend.HuggingFaceFile
}

type HFErrorMsg struct {
	Error error
}

func NewHFBrowseCmp() *HFBrowseCmp {
	t := styles.CurrentTheme()
	
	searchInput := textinput.New()
	searchInput.Placeholder = "Search models (e.g., 'llama', 'qwen', 'mistral')..."
	searchInput.CharLimit = 100
	
	m := &HFBrowseCmp{
		state:       HFStateLoading,
		theme:       t,
		hfClient:    backend.NewHuggingFaceClient(),
		searchInput: searchInput,
		loading:     true,
	}
	
	return m
}

func (m *HFBrowseCmp) Init() tea.Cmd {
	// Load recent models on init
	return m.loadRecentModels()
}

func (m *HFBrowseCmp) loadRecentModels() tea.Cmd {
	return func() tea.Msg {
		models, err := m.hfClient.GetRecentModels(context.Background())
		if err != nil {
			return HFErrorMsg{Error: err}
		}
		return HFModelsLoadedMsg{Models: models}
	}
}

func (m *HFBrowseCmp) searchModels(query string) tea.Cmd {
	return func() tea.Msg {
		// First try searching with the query + gguf
		ggufQuery := query + " gguf"
		models, err := m.hfClient.SearchModels(context.Background(), ggufQuery, "")
		if err != nil {
			return HFErrorMsg{Error: err}
		}
		
		// If on Apple Silicon, also search for MLX models
		if backend.IsAppleSilicon() {
			mlxQuery := query + " mlx"
			mlxModels, err := m.hfClient.SearchModels(context.Background(), mlxQuery, "")
			if err == nil && len(mlxModels) > 0 {
				// Append MLX models, avoiding duplicates
				modelMap := make(map[string]bool)
				for _, model := range models {
					modelMap[model.ID] = true
				}
				for _, model := range mlxModels {
					if !modelMap[model.ID] {
						models = append(models, model)
					}
				}
			}
		}
		
		// If no results with format specifiers, try raw query
		if len(models) == 0 {
			models, err = m.hfClient.SearchModels(context.Background(), query, "")
			if err != nil {
				return HFErrorMsg{Error: err}
			}
		}
		
		return HFModelsLoadedMsg{Models: models}
	}
}

func (m *HFBrowseCmp) loadModelFiles(modelID string) tea.Cmd {
	return func() tea.Msg {
		files, err := m.hfClient.GetModelFiles(context.Background(), modelID)
		if err != nil {
			return HFErrorMsg{Error: err}
		}
		return HFFilesLoadedMsg{Files: files}
	}
}

func (m *HFBrowseCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
		
	case HFModelsLoadedMsg:
		m.hfModels = msg.Models
		m.loading = false
		m.state = HFStateList
		m.selectedIdx = 0
		return m, nil
		
	case HFFilesLoadedMsg:
		m.hfFiles = msg.Files
		m.loading = false
		m.state = HFStateFileSelect
		m.fileIdx = 0
		return m, nil
		
	case HFErrorMsg:
		m.errorMsg = msg.Error.Error()
		m.loading = false
		return m, nil
		
	case tea.KeyMsg:
		switch m.state {
		case HFStateList:
			switch msg.String() {
			case "up", "k":
				if m.selectedIdx > 0 {
					m.selectedIdx--
				}
				
			case "down", "j":
				if m.selectedIdx < len(m.hfModels)-1 {
					m.selectedIdx++
				}
				
			case "enter":
				if m.selectedIdx < len(m.hfModels) {
					model := m.hfModels[m.selectedIdx]
					m.selectedModel = &model
					
					// Check if this is an MLX model - if so, download directly
					if isMLXModel(model) {
						// Create a model option for MLX download
						modelOpt := &backend.ModelOption{
							ID:          model.ID,
							Name:        model.ID,
							Description: fmt.Sprintf("MLX Model from HuggingFace: %s", model.ID),
							URL:         fmt.Sprintf("https://huggingface.co/%s", model.ID),
							Provider:    "mlx",
							Available:   backend.IsAppleSilicon(),
						}
						
						// Get data directory
						dataDir := ".toke"
						if cfg := config.Get(); cfg.Options != nil && cfg.Options.DataDirectory != "" {
							dataDir = cfg.Options.DataDirectory
						}
						orchestrator := backend.NewOrchestrator(dataDir)
						
						// Create backend setup dialog in download mode
						backendDialog := backendDlg.NewWithModel(
							orchestrator,
							modelOpt,
							func(downloadedModel *backend.ModelOption) {
								// Model downloaded successfully
								slog.Info("MLX model downloaded", "model", downloadedModel.Name)
							},
						)
						
						// Switch to the download dialog
						return m, util.CmdHandler(dialogs.OpenDialogMsg{Model: backendDialog})
					} else {
						// For non-MLX models, show file selection
						m.loading = true
						return m, m.loadModelFiles(model.ID)
					}
				}
				
			case "s", "/":
				// Switch to search mode
				m.state = HFStateSearch
				m.searchInput.Focus()
				return m, textinput.Blink
				
			case "esc":
				return m, util.CmdHandler(dialogs.CloseDialogMsg{})
			}
			
		case HFStateSearch:
			switch msg.String() {
			case "enter":
				query := m.searchInput.Value()
				if query != "" {
					m.loading = true
					m.state = HFStateLoading
					return m, m.searchModels(query)
				}
				
			case "esc":
				// Go back to list
				m.state = HFStateList
				m.searchInput.Blur()
				m.searchInput.SetValue("")
				return m, nil
				
			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				return m, cmd
			}
			
		case HFStateFileSelect:
			switch msg.String() {
			case "up", "k":
				if m.fileIdx > 0 {
					m.fileIdx--
				}
				
			case "down", "j":
				if m.fileIdx < len(m.hfFiles)-1 {
					m.fileIdx++
				}
				
			case "enter":
				if m.fileIdx < len(m.hfFiles) && m.selectedModel != nil {
					// Download the selected file
					selectedFile := m.hfFiles[m.fileIdx]
					
					// Build download URL
					downloadURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", 
						m.selectedModel.ID, selectedFile.Path)
					
					// Create a model option from the HF model and file
					modelOpt := &backend.ModelOption{
						ID:          m.selectedModel.ID + "/" + selectedFile.Path,
						Name:        m.selectedModel.ID + " - " + selectedFile.Path,
						Description: fmt.Sprintf("HuggingFace: %s", m.selectedModel.ID),
						URL:         downloadURL,
						Size:        selectedFile.Size,
						Memory:      selectedFile.Size * 2, // Estimate
					}
					
					// Determine provider from file extension
					if strings.HasSuffix(strings.ToLower(selectedFile.Path), ".gguf") {
						modelOpt.Provider = "llamacpp"
					} else if strings.Contains(strings.ToLower(selectedFile.Path), "mlx") {
						modelOpt.Provider = "mlx"
					}
					
					// Get data directory
					dataDir := ".toke"
					if cfg := config.Get(); cfg.Options != nil && cfg.Options.DataDirectory != "" {
						dataDir = cfg.Options.DataDirectory
					}
					orchestrator := backend.NewOrchestrator(dataDir)
					
					// Create backend setup dialog in download mode
					backendDialog := backendDlg.NewWithModel(
						orchestrator,
						modelOpt,
						func(downloadedModel *backend.ModelOption) {
							// Model downloaded successfully
						},
					)
					
					// Switch to the download dialog
					return m, util.CmdHandler(dialogs.OpenDialogMsg{Model: backendDialog})
				}
				
			case "b", "esc":
				// Go back to model list
				m.state = HFStateList
				m.hfFiles = nil
				m.selectedModel = nil
				return m, nil
			}
		}
	}
	
	return m, nil
}

func (m *HFBrowseCmp) View() string {
	var content strings.Builder
	
	switch m.state {
	case HFStateLoading:
		content.WriteString(m.renderLoading())
	case HFStateList:
		content.WriteString(m.renderModelList())
	case HFStateSearch:
		content.WriteString(m.renderSearch())
	case HFStateFileSelect:
		content.WriteString(m.renderFileSelect())
	}
	
	if m.errorMsg != "" {
		errorStyle := lipgloss.NewStyle().
			Foreground(m.theme.Error).
			MarginTop(2)
		content.WriteString("\n")
		content.WriteString(errorStyle.Render("Error: " + m.errorMsg))
	}
	
	// Create a nice bordered box
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Primary).
		Padding(1, 2).
		Width(m.width - 8).
		Height(m.height - 6)
	
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		dialogStyle.Render(content.String()),
	)
}

func (m *HFBrowseCmp) renderLoading() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.Primary)
	return titleStyle.Render("Loading Hugging Face models...")
}

func (m *HFBrowseCmp) renderModelList() string {
	var s strings.Builder
	
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.Primary).
		MarginBottom(1)
	s.WriteString(titleStyle.Render("🤗 Browse Hugging Face Models"))
	s.WriteString("\n\n")
	
	if len(m.hfModels) == 0 {
		s.WriteString("No models found.\n")
	} else {
		for i, model := range m.hfModels {
			cursor := "  "
			itemStyle := lipgloss.NewStyle()
			
			if i == m.selectedIdx {
				cursor = "→ "
				itemStyle = itemStyle.Foreground(m.theme.Primary).Bold(true)
			} else {
				itemStyle = itemStyle.Foreground(m.theme.FgBase)
			}
			
			s.WriteString(cursor)
			
			// Format as "Model ID - Author (Downloads: X)" on same line
			displayText := model.ID
			var details []string
			if model.Author != "" {
				details = append(details, model.Author)
			}
			if model.Downloads > 0 {
				details = append(details, fmt.Sprintf("%d downloads", model.Downloads))
			}
			if len(details) > 0 {
				displayText = fmt.Sprintf("%s - %s", model.ID, strings.Join(details, ", "))
			}
			
			s.WriteString(itemStyle.Render(displayText))
			s.WriteString("\n")
		}
	}
	
	// Footer
	footerStyle := lipgloss.NewStyle().
		Foreground(m.theme.FgHalfMuted).
		MarginTop(2)
	s.WriteString(footerStyle.Render("↑/↓: navigate • enter: select • s: search • esc: back"))
	
	return s.String()
}

func (m *HFBrowseCmp) renderSearch() string {
	var s strings.Builder
	
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.Primary).
		MarginBottom(1)
	s.WriteString(titleStyle.Render("🔍 Search Hugging Face"))
	s.WriteString("\n\n")
	
	s.WriteString(m.searchInput.View())
	s.WriteString("\n\n")
	
	// Footer
	footerStyle := lipgloss.NewStyle().
		Foreground(m.theme.FgHalfMuted)
	s.WriteString(footerStyle.Render("enter: search • esc: cancel"))
	
	return s.String()
}

func (m *HFBrowseCmp) renderFileSelect() string {
	var s strings.Builder
	
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.Primary).
		MarginBottom(1)
	
	modelName := "Model"
	if m.selectedModel != nil {
		modelName = m.selectedModel.ID
	}
	s.WriteString(titleStyle.Render(fmt.Sprintf("📦 Select File - %s", modelName)))
	s.WriteString("\n\n")
	
	if len(m.hfFiles) == 0 {
		s.WriteString("No files found.\n")
	} else {
		for i, file := range m.hfFiles {
			cursor := "  "
			itemStyle := lipgloss.NewStyle()
			
			// Highlight GGUF and MLX files
			isRecommended := strings.HasSuffix(strings.ToLower(file.Path), ".gguf") ||
				strings.Contains(strings.ToLower(file.Path), "mlx")
			
			if i == m.fileIdx {
				cursor = "→ "
				itemStyle = itemStyle.Foreground(m.theme.Primary).Bold(true)
			} else if isRecommended {
				itemStyle = itemStyle.Foreground(m.theme.Success)
			} else {
				itemStyle = itemStyle.Foreground(m.theme.FgBase)
			}
			
			s.WriteString(cursor)
			s.WriteString(itemStyle.Render(file.Path))
			
			// Show size
			sizeStyle := lipgloss.NewStyle().
				Foreground(m.theme.FgHalfMuted).
				MarginLeft(2)
			s.WriteString(sizeStyle.Render(fmt.Sprintf(" (%s)", backend.FormatSize(file.Size))))
			
			if isRecommended && i == m.fileIdx {
				s.WriteString(" ✓")
			}
			
			s.WriteString("\n")
		}
	}
	
	// Footer
	footerStyle := lipgloss.NewStyle().
		Foreground(m.theme.FgHalfMuted).
		MarginTop(2)
	s.WriteString(footerStyle.Render("↑/↓: navigate • enter: download • esc: back"))
	
	return s.String()
}

func (m *HFBrowseCmp) ID() dialogs.DialogID {
	return "hf_browse"
}

// isMLXModel checks if a HuggingFace model is an MLX model
func isMLXModel(model backend.HuggingFaceModel) bool {
	modelIDLower := strings.ToLower(model.ID)
	
	// Check if model ID contains mlx
	if strings.Contains(modelIDLower, "mlx") {
		return true
	}
	
	// Check tags for mlx
	for _, tag := range model.Tags {
		if strings.ToLower(tag) == "mlx" {
			return true
		}
	}
	
	// Check library name
	if strings.ToLower(model.LibraryName) == "mlx" {
		return true
	}
	
	// If none of the above, assume it's not MLX (will show file picker)
	return false
}

func (m *HFBrowseCmp) Position() (int, int) {
	return 0, 0
}