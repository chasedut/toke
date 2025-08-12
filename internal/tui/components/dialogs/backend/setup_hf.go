package backend

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/backend"
)

// Commands for loading HF data
func (m Model) loadRecentModels() tea.Cmd {
	return func() tea.Msg {
		models, err := m.hfClient.GetRecentModels(context.Background())
		if err != nil {
			return HFErrorMsg{Error: err}
		}
		return HFModelsLoadedMsg{Models: models}
	}
}

func (m Model) searchModels(query string) tea.Cmd {
	return func() tea.Msg {
		models, err := m.hfClient.SearchModels(context.Background(), query, "")
		if err != nil {
			return HFErrorMsg{Error: err}
		}
		return HFModelsLoadedMsg{Models: models}
	}
}

func (m Model) loadModelFiles(modelID string) tea.Cmd {
	return func() tea.Msg {
		files, err := m.hfClient.GetModelFiles(context.Background(), modelID)
		if err != nil {
			return HFErrorMsg{Error: err}
		}
		return HFFilesLoadedMsg{Files: files}
	}
}

// Handle HF browse state
func (m Model) handleHFBrowseKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.hfSelectedIdx > 0 {
			m.hfSelectedIdx--
		}
		
	case "down", "j":
		if m.hfSelectedIdx < len(m.hfModels)-1 {
			m.hfSelectedIdx++
		}
		
	case "enter":
		if m.hfSelectedIdx < len(m.hfModels) {
			// Load files for selected model
			m.state = StateHFFileSelect
			m.hfLoading = true
			return m, m.loadModelFiles(m.hfModels[m.hfSelectedIdx].ID)
		}
		
	case "s":
		// Switch to search mode
		m.state = StateHFSearch
		m.hfSearchInput.Focus()
		return m, textinput.Blink
		
	case "esc":
		m.state = StateSelection
		m.hfModels = nil
		m.hfSelectedIdx = 0
	}
	
	return m, nil
}

// Handle HF search state
func (m Model) handleHFSearchKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		query := m.hfSearchInput.Value()
		if query != "" {
			m.hfLoading = true
			return m, m.searchModels(query)
		}
		
	case "esc":
		m.state = StateHFBrowse
		m.hfSearchInput.Blur()
		m.hfSearchInput.SetValue("")
		if len(m.hfModels) == 0 {
			// Load recent models if we don't have any
			m.hfLoading = true
			return m, m.loadRecentModels()
		}
	}
	
	// Pass other keys to text input
	var cmd tea.Cmd
	m.hfSearchInput, cmd = m.hfSearchInput.Update(msg)
	return m, cmd
}

// Handle HF file selection state
func (m Model) handleHFFileSelectKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.hfFileIdx > 0 {
			m.hfFileIdx--
		}
		
	case "down", "j":
		if m.hfFileIdx < len(m.hfFiles)-1 {
			m.hfFileIdx++
		}
		
	case "enter":
		if m.hfFileIdx < len(m.hfFiles) && m.hfSelectedIdx < len(m.hfModels) {
			// Convert HF model to our ModelOption format
			selectedFile := m.hfFiles[m.hfFileIdx]
			hfModel := m.hfModels[m.hfSelectedIdx]
			
			// Get file size
			fileSize := selectedFile.Size
			if selectedFile.LFS.Size > 0 {
				fileSize = selectedFile.LFS.Size
			}
			
			// Create ModelOption from HF model
			modelOption := m.hfClient.ConvertToModelOption(hfModel, selectedFile.Path, fileSize)
			m.selectedModel = &modelOption
			
			// Start download
			m.state = StateDownloading
			return m, m.startDownload()
		}
		
	case "esc":
		m.state = StateHFBrowse
		m.hfFiles = nil
		m.hfFileIdx = 0
	}
	
	return m, nil
}

// Render functions for HF states
func (m Model) renderHFBrowse() string {
	var s strings.Builder
	
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("70")).
		MarginBottom(1).
		Render("🌐 Browse Hugging Face Models")
	
	s.WriteString(title + "\n\n")
	
	if m.hfLoading {
		s.WriteString("Loading models...\n")
		return s.String()
	}
	
	if len(m.hfModels) == 0 {
		s.WriteString("No models found.\n\nPress 's' to search • ESC to go back")
		return s.String()
	}
	
	// Show models
	for i, model := range m.hfModels {
		if i > 15 { // Limit display to 15 models
			break
		}
		
		cursor := "  "
		if i == m.hfSelectedIdx {
			cursor = "→ "
		}
		
		// Format model info
		name := model.ID
		if len(name) > 50 {
			name = name[:47] + "..."
		}
		
		info := fmt.Sprintf("⬇ %d  ❤ %d", model.Downloads, model.Likes)
		
		line := fmt.Sprintf("%s%-52s %s", cursor, name, info)
		
		if i == m.hfSelectedIdx {
			line = lipgloss.NewStyle().
				Foreground(lipgloss.Color("70")).
				Bold(true).
				Render(line)
		}
		
		s.WriteString(line + "\n")
	}
	
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		MarginTop(2).
		Render("\nPress 's' to search • ENTER to select • ESC to go back")
	s.WriteString(footer)
	
	return s.String()
}

func (m Model) renderHFSearch() string {
	var s strings.Builder
	
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("70")).
		MarginBottom(1).
		Render("🔍 Search Hugging Face Models")
	
	s.WriteString(title + "\n\n")
	
	s.WriteString("Enter search query:\n\n")
	s.WriteString(m.hfSearchInput.View() + "\n\n")
	
	if m.hfLoading {
		s.WriteString("Searching...\n")
	} else if len(m.hfModels) > 0 {
		s.WriteString(fmt.Sprintf("Found %d models. Press ESC to browse results.\n", len(m.hfModels)))
	}
	
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		MarginTop(2).
		Render("Press ENTER to search • ESC to cancel")
	s.WriteString(footer)
	
	return s.String()
}

func (m Model) renderHFFileSelect() string {
	var s strings.Builder
	
	if m.hfSelectedIdx >= len(m.hfModels) {
		return "Error: Invalid model selection"
	}
	
	model := m.hfModels[m.hfSelectedIdx]
	
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("70")).
		MarginBottom(1).
		Render(fmt.Sprintf("📦 Select File from %s", model.ID))
	
	s.WriteString(title + "\n\n")
	
	if m.hfLoading {
		s.WriteString("Loading files...\n")
		return s.String()
	}
	
	if len(m.hfFiles) == 0 {
		s.WriteString("No compatible files found.\n\nPress ESC to go back")
		return s.String()
	}
	
	// Filter and show only relevant files (GGUF, safetensors)
	relevantFiles := []backend.HuggingFaceFile{}
	for _, file := range m.hfFiles {
		lower := strings.ToLower(file.Path)
		if strings.HasSuffix(lower, ".gguf") || 
		   strings.HasSuffix(lower, ".safetensors") ||
		   strings.Contains(lower, "model") {
			relevantFiles = append(relevantFiles, file)
		}
	}
	
	if len(relevantFiles) == 0 {
		s.WriteString("No compatible model files found (looking for .gguf or .safetensors)\n\nPress ESC to go back")
		return s.String()
	}
	
	// Update files list to only show relevant ones
	m.hfFiles = relevantFiles
	
	for i, file := range relevantFiles {
		if i > 10 { // Limit display
			break
		}
		
		cursor := "  "
		if i == m.hfFileIdx {
			cursor = "→ "
		}
		
		// Format file info
		size := file.Size
		if file.LFS.Size > 0 {
			size = file.LFS.Size
		}
		
		sizeStr := backend.FormatSize(size)
		fileName := file.Path
		if len(fileName) > 45 {
			fileName = "..." + fileName[len(fileName)-42:]
		}
		
		line := fmt.Sprintf("%s%-47s %8s", cursor, fileName, sizeStr)
		
		if i == m.hfFileIdx {
			line = lipgloss.NewStyle().
				Foreground(lipgloss.Color("70")).
				Bold(true).
				Render(line)
		}
		
		s.WriteString(line + "\n")
	}
	
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		MarginTop(2).
		Render("\nPress ENTER to download • ESC to go back")
	s.WriteString(footer)
	
	return s.String()
}