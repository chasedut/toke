package models

import (
	"fmt"
	"log/slog"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/backend"
	"github.com/chasedut/toke/internal/config"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	backendDlg "github.com/chasedut/toke/internal/tui/components/dialogs/backend"
	"github.com/chasedut/toke/internal/tui/exp/list"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
)

// AddOption represents a model add option
type AddOption struct {
	Type        string // "download_local", "setup_api", "download_mlx", "download_gguf"
	Title       string
	Description string
	ModelID     string // For specific models
	Provider    string // For API setup
}

type AddModelsCmp struct {
	theme        *styles.Theme
	width        int
	height       int
	selectedIdx  int
	items        []AddOption
	groups       []list.Group[list.CompletionItem[AddOption]]
}

func NewAddModelsCmp() *AddModelsCmp {
	t := styles.CurrentTheme()

	m := &AddModelsCmp{
		theme:       t,
		selectedIdx: 0,
	}

	m.groups = m.buildOptions()
	
	// Build flat items list for easier navigation
	for _, group := range m.groups {
		for _, item := range group.Items {
			m.items = append(m.items, item.Value())
		}
	}

	return m
}

func (m *AddModelsCmp) buildOptions() []list.Group[list.CompletionItem[AddOption]] {
	var groups []list.Group[list.CompletionItem[AddOption]]

	// Quick Setup section
	quickSection := list.NewItemSection("⚡ Quick Setup")
	quickSection.SetInfo("Popular choices")
	quickGroup := list.Group[list.CompletionItem[AddOption]]{
		Section: quickSection,
	}

	// Add recommended model
	recommendedItem := list.NewCompletionItem(
		"🎯 Download Qwen 2.5 Coder 7B (Recommended)",
		AddOption{
			Type:        "download_gguf",
			Title:       "Qwen 2.5 Coder 7B",
			Description: "Best coding model - 4GB, runs locally",
			ModelID:     "qwen2.5-coder-7b-q4_k_m",
		},
		list.WithCompletionID("quick:recommended"),
	)
	quickGroup.Items = append(quickGroup.Items, recommendedItem)

	// Add API setup
	apiItem := list.NewCompletionItem(
		"🔑 Setup API Provider",
		AddOption{
			Type:        "setup_api",
			Title:       "Setup API Provider",
			Description: "Configure OpenAI, Anthropic, or other cloud providers",
		},
		list.WithCompletionID("quick:api"),
	)
	quickGroup.Items = append(quickGroup.Items, apiItem)

	groups = append(groups, quickGroup)

	// Local Models section (GGUF)
	localSection := list.NewItemSection("💻 Local Models (GGUF)")
	localSection.SetInfo("Run on CPU/GPU")
	localGroup := list.Group[list.CompletionItem[AddOption]]{
		Section: localSection,
	}

	localModels := []struct {
		id   string
		name string
		desc string
	}{
		{"qwen2.5-3b-q4_k_m", "Qwen 2.5 3B", "Smaller, faster - 2GB"},
		{"qwen2.5-14b-q4_k_m", "Qwen 2.5 14B", "Larger, smarter - 8GB"},
		{"glm-4.5-air-q2_k", "GLM 4.5 Air", "Massive 107B model - 44GB"},
	}

	for _, model := range localModels {
		item := list.NewCompletionItem(
			model.name,
			AddOption{
				Type:        "download_gguf",
				Title:       model.name,
				Description: model.desc,
				ModelID:     model.id,
			},
			list.WithCompletionID(fmt.Sprintf("gguf:%s", model.id)),
		)
		localGroup.Items = append(localGroup.Items, item)
	}

	groups = append(groups, localGroup)

	// MLX Models section (Apple Silicon only)
	if backend.IsAppleSilicon() {
		mlxSection := list.NewItemSection("🍎 MLX Models (Apple Silicon)")
		mlxSection.SetInfo("Optimized for M-series")
		mlxGroup := list.Group[list.CompletionItem[AddOption]]{
			Section: mlxSection,
		}

		mlxModels := []struct {
			id   string
			name string
			desc string
		}{
			{"qwen2.5-coder-7b-4bit", "Qwen 2.5 Coder MLX", "5GB - Faster than GGUF on Apple Silicon"},
			{"glm-4.5-air-3bit", "GLM 4.5 Air 3-bit MLX", "13GB - For 16GB RAM Macs"},
			{"glm-4.5-air-4bit", "GLM 4.5 Air 4-bit MLX", "17GB - For 24GB+ RAM Macs"},
		}

		for _, model := range mlxModels {
			item := list.NewCompletionItem(
				model.name,
				AddOption{
					Type:        "download_mlx",
					Title:       model.name,
					Description: model.desc,
					ModelID:     model.id,
				},
				list.WithCompletionID(fmt.Sprintf("mlx:%s", model.id)),
			)
			mlxGroup.Items = append(mlxGroup.Items, item)
		}

		groups = append(groups, mlxGroup)
	}

	// Advanced section
	advancedSection := list.NewItemSection("🔧 Advanced")
	advancedSection.SetInfo("More options")
	advancedGroup := list.Group[list.CompletionItem[AddOption]]{
		Section: advancedSection,
	}

	browseItem := list.NewCompletionItem(
		"🔍 Browse Hugging Face Models",
		AddOption{
			Type:        "browse_hf",
			Title:       "Browse Hugging Face",
			Description: "Explore more models on Hugging Face",
		},
		list.WithCompletionID("advanced:browse"),
	)
	advancedGroup.Items = append(advancedGroup.Items, browseItem)

	groups = append(groups, advancedGroup)

	return groups
}

func (m *AddModelsCmp) Init() tea.Cmd {
	return nil
}

func (m *AddModelsCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Handle navigation and selection
		switch msg.String() {
		case "up", "k":
			if m.selectedIdx > 0 {
				m.selectedIdx--
			} else {
				// Wrap to bottom
				m.selectedIdx = len(m.items) - 1
			}
			return m, nil
			
		case "down", "j":
			if m.selectedIdx < len(m.items)-1 {
				m.selectedIdx++
			} else {
				// Wrap to top
				m.selectedIdx = 0
			}
			return m, nil
			
		case "esc":
			return m, util.CmdHandler(dialogs.CloseDialogMsg{})
			
		case "enter":
			if m.selectedIdx >= 0 && m.selectedIdx < len(m.items) {
				option := m.items[m.selectedIdx]
				return m.handleSelection(option)
			}
		}
	}

	return m, nil
}

func (m *AddModelsCmp) handleSelection(option AddOption) (tea.Model, tea.Cmd) {
	switch option.Type {
	case "download_gguf", "download_mlx":
		// Start download directly
		model := backend.GetModelByID(option.ModelID)
		if model == nil {
			slog.Error("Model not found", "id", option.ModelID)
			return m, util.CmdHandler(dialogs.CloseDialogMsg{})
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
			model,
			func(downloadedModel *backend.ModelOption) {
				// Model downloaded successfully
				slog.Info("Model downloaded", "model", downloadedModel.Name)
			},
		)
		
		// Switch to the download dialog
		return m, util.CmdHandler(dialogs.OpenDialogMsg{Model: backendDialog})

	case "setup_api":
		// Open the regular model switcher which has API setup
		return m, util.CmdHandler(dialogs.OpenDialogMsg{Model: NewModelDialogCmp()})

	case "browse_hf":
		// Open the new HF Browse dialog as a modal
		hfDialog := NewHFBrowseCmp()
		return m, util.CmdHandler(dialogs.OpenDialogMsg{Model: hfDialog})

	default:
		return m, util.CmdHandler(dialogs.CloseDialogMsg{})
	}
}

func (m *AddModelsCmp) View() string {
	var content strings.Builder
	
	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.Primary).
		MarginBottom(1)
	content.WriteString(titleStyle.Render("New Model"))
	content.WriteString("\n\n")
	
	// Render each group and item with selection highlight
	itemIdx := 0
	for _, group := range m.groups {
		// Section header - get the view from the section
		sectionView := group.Section.View()
		content.WriteString(sectionView)
		content.WriteString("\n")
		
		// Items
		for _, item := range group.Items {
			cursor := "  "
			itemStyle := lipgloss.NewStyle()
			
			if itemIdx == m.selectedIdx {
				cursor = "→ "
				itemStyle = itemStyle.Foreground(m.theme.Primary).Bold(true)
			} else {
				itemStyle = itemStyle.Foreground(m.theme.FgBase)
			}
			
			content.WriteString(cursor)
			content.WriteString(itemStyle.Render(item.Text()))
			
			// Show description for selected item
			if itemIdx == m.selectedIdx && item.Value().Description != "" {
				descStyle := lipgloss.NewStyle().
					Foreground(m.theme.FgHalfMuted).
					MarginLeft(4)
				content.WriteString("\n")
				content.WriteString(descStyle.Render(item.Value().Description))
			}
			
			content.WriteString("\n")
			itemIdx++
		}
		content.WriteString("\n")
	}
	
	// Footer
	footerStyle := lipgloss.NewStyle().
		Foreground(m.theme.FgHalfMuted).
		MarginTop(1)
	content.WriteString(footerStyle.Render("↑/↓: navigate • enter: select • esc: cancel"))
	
	// Create a nice bordered box like the switch model dialog
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Primary).
		Padding(1, 2).
		Width(m.width - 4).
		Height(m.height - 2)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		dialogStyle.Render(content.String()),
	)
}

func (m *AddModelsCmp) ShortHelp() []string {
	return []string{"↑/↓: navigate", "enter: select", "esc: cancel"}
}

func (m *AddModelsCmp) FullHelp() []string {
	return m.ShortHelp()
}

func (m *AddModelsCmp) ID() dialogs.DialogID {
	return "add_models"
}

func (m *AddModelsCmp) Position() (int, int) {
	return 0, 0
}