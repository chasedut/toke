package models

import (
	"fmt"
	"runtime"

	"github.com/charmbracelet/bubbles/v2/help"
	"github.com/charmbracelet/bubbles/v2/key"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/catwalk/pkg/catwalk"
	"github.com/chasedut/toke/internal/backend"
	"github.com/chasedut/toke/internal/config"
	"github.com/chasedut/toke/internal/tui/components/core"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	backendDlg "github.com/chasedut/toke/internal/tui/components/dialogs/backend"
	"github.com/chasedut/toke/internal/tui/exp/list"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
	"github.com/charmbracelet/lipgloss/v2"
)

const (
	LocalModelsDialogID dialogs.DialogID = "local_models"
	localModelsWidth    int              = 60
)

// LocalModelDialog interface for the local model selection dialog
type LocalModelDialog interface {
	dialogs.DialogModel
}

type localModelDialogCmp struct {
	width   int
	wWidth  int
	wHeight int

	modelList    *ModelListComponent
	keyMap       KeyMap
	help         help.Model
	selectedIdx  int
	
	orchestrator *backend.Orchestrator
}

func NewLocalModelDialogCmp() LocalModelDialog {
	keyMap := DefaultKeyMap()

	listKeyMap := list.DefaultKeyMap()
	listKeyMap.Down.SetEnabled(false)
	listKeyMap.Up.SetEnabled(false)
	listKeyMap.DownOneItem = keyMap.Next
	listKeyMap.UpOneItem = keyMap.Previous

	t := styles.CurrentTheme()
	modelList := NewModelListComponent(listKeyMap, "Search for local models to download", true)
	
	help := help.New()
	help.Styles = t.S().Help

	// Create orchestrator for local model downloads
	dataDir := ".toke"
	if cfg := config.Get(); cfg.Options != nil && cfg.Options.DataDirectory != "" {
		dataDir = cfg.Options.DataDirectory
	}
	orchestrator := backend.NewOrchestrator(dataDir)

	return &localModelDialogCmp{
		modelList:    modelList,
		width:        localModelsWidth,
		keyMap:       DefaultKeyMap(),
		help:         help,
		orchestrator: orchestrator,
	}
}

func (m *localModelDialogCmp) Init() tea.Cmd {
	// Initialize with local models
	return tea.Batch(
		m.modelList.Init(),
		m.setupLocalModels(),
	)
}

func (m *localModelDialogCmp) setupLocalModels() tea.Cmd {
	return func() tea.Msg {
		var groups []list.Group[list.CompletionItem[ModelOption]]
		
		// Get system info to recommend appropriate models
		dataDir := ".toke"
		if cfg := config.Get(); cfg.Options != nil && cfg.Options.DataDirectory != "" {
			dataDir = cfg.Options.DataDirectory
		}
		
		sysInfo, err := backend.GetSystemInfo(dataDir)
		if err != nil {
			// If we can't get system info, fall back to default recommendations
			sysInfo = &backend.SystemInfo{
				TotalRAM:      8 * 1024 * 1024 * 1024,  // 8GB default
				AvailableRAM:  4 * 1024 * 1024 * 1024,  // 4GB default
				FreeDiskSpace: 50 * 1024 * 1024 * 1024, // 50GB default
				CPUCores:      runtime.NumCPU(),
				IsAppleSilicon: backend.IsAppleSilicon(),
			}
		}
		
		// Get dynamically recommended models based on system specs
		recommendations := backend.RecommendModelsForSystem(sysInfo)
		if len(recommendations) == 0 {
			// Fallback to default if no recommendations
			if model := backend.GetRecommendedModel(); model != nil {
				recommendations = []*backend.ModelOption{model}
			}
		}
		
		// Create a map for quick lookup of recommended models
		recommendedMap := make(map[string]bool)
		for _, model := range recommendations {
			if model != nil {
				recommendedMap[model.ID] = true
			}
		}
		
		// Single section for all models
		allModelsSection := list.NewItemSection("📦 Available Models")
		allModelsGroup := list.Group[list.CompletionItem[ModelOption]]{
			Section: allModelsSection,
		}
		
		// Add Hugging Face options at the top
		hfBrowseItem := list.NewCompletionItem(
			"🤗 Browse Recent Models",
			ModelOption{
				Provider: catwalk.Provider{ID: "hf_browse", Name: "Browse Hugging Face"},
				Model:    catwalk.Model{ID: "hf_browse", Name: "Browse Recent Models"},
			},
			list.WithCompletionID("hf:browse"),
		)
		allModelsGroup.Items = append(allModelsGroup.Items, hfBrowseItem)
		
		hfSearchItem := list.NewCompletionItem(
			"🔍 Search Models",
			ModelOption{
				Provider: catwalk.Provider{ID: "hf_search", Name: "Search Hugging Face"},
				Model:    catwalk.Model{ID: "hf_search", Name: "Search Models"},
			},
			list.WithCompletionID("hf:search"),
		)
		allModelsGroup.Items = append(allModelsGroup.Items, hfSearchItem)
		
		// Add MLX models for Apple Silicon
		if backend.IsAppleSilicon() {
			mlxModels := []struct {
				id   string
				name string
				desc string
			}{
				{"qwen2.5-coder-7b-4bit", "Qwen 2.5 Coder 7B", "5GB - Faster than GGUF on Apple Silicon"},
				{"glm-4.5-air-3bit", "GLM 4.5 Air 3-bit", "13GB - Efficient for 16GB RAM"},
				{"glm-4.5-air-4bit", "GLM 4.5 Air 4-bit", "56GB - Balanced for 24GB+ RAM"},
				{"glm-4.5-air-8bit", "GLM 4.5 Air 8-bit", "34GB - Highest quality for 48GB+ RAM"},
			}
			
			for _, model := range mlxModels {
				if m := backend.GetModelByID(model.id); m != nil {
					// Add star if recommended
					prefix := ""
					if recommendedMap[m.ID] {
						prefix = "⭐ "
					}
					
					item := list.NewCompletionItem(
						fmt.Sprintf("%s%s - %s [MLX]", prefix, model.name, model.desc),
						ModelOption{
							Provider: catwalk.Provider{ID: "local_download", Name: "Download"},
							Model:    catwalk.Model{ID: m.ID, Name: m.Name},
						},
						list.WithCompletionID("download:"+m.ID),
					)
					allModelsGroup.Items = append(allModelsGroup.Items, item)
				}
			}
		}
		
		// Add GGUF models
		ggufModels := []struct {
			id   string
			name string
			desc string
		}{
			{"qwen2.5-14b-q4_k_m", "Qwen 2.5 14B", "8GB - Balanced, better reasoning"},
			{"qwen2.5-3b-q4_k_m", "Qwen 2.5 3B", "2GB - Light, faster"},
			{"glm-4.5-air-q2_k", "GLM 4.5 Air Q2_K", "44GB - Power user (64GB RAM required)"},
		}
		
		for _, model := range ggufModels {
			if m := backend.GetModelByID(model.id); m != nil {
				// Add star if recommended
				prefix := ""
				if recommendedMap[m.ID] {
					prefix = "⭐ "
				}
				
				item := list.NewCompletionItem(
					fmt.Sprintf("%s%s - %s [GGUF]", prefix, model.name, model.desc),
					ModelOption{
						Provider: catwalk.Provider{ID: "local_download", Name: "Download"},
						Model:    catwalk.Model{ID: m.ID, Name: m.Name},
					},
					list.WithCompletionID("download:"+m.ID),
				)
				allModelsGroup.Items = append(allModelsGroup.Items, item)
			}
		}
		
		// Add legend at the bottom
		allModelsSection.SetInfo("⭐ = Recommended for your system")
		
		groups = append(groups, allModelsGroup)
		
		// Check if a local model is already configured
		cfg := config.Get()
		if localConfig, _ := cfg.GetLocalModelConfig(); localConfig != nil && localConfig.Enabled {
			activeSection := list.NewItemSection("✅ Active Local Model")
			activeGroup := list.Group[list.CompletionItem[ModelOption]]{
				Section: activeSection,
			}
			
			activeItem := list.NewCompletionItem(
				localConfig.ModelID,
				ModelOption{
					Provider: catwalk.Provider{ID: "local", Name: "Local Model"},
					Model:    catwalk.Model{ID: localConfig.ModelID, Name: localConfig.ModelID},
				},
				list.WithCompletionID("active:local"),
			)
			activeGroup.Items = append(activeGroup.Items, activeItem)
			
			// Insert at the beginning
			groups = append([]list.Group[list.CompletionItem[ModelOption]]{activeGroup}, groups...)
		}
		
		return m.modelList.list.SetGroups(groups)
	}
}

func (m *localModelDialogCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.wWidth = msg.Width
		m.wHeight = msg.Height
		m.help.Width = m.width - 2
		return m, m.modelList.SetSize(m.listWidth(), m.listHeight())
		
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keyMap.Select):
			selectedModel := m.modelList.SelectedModel()
			if selectedModel == nil {
				return m, nil
			}
			
			switch selectedModel.Provider.ID {
			case "hf_browse":
				// Launch Hugging Face browse dialog with modern UI
				hfDialog := NewHFBrowseCmp()
				return m, tea.Sequence(
					util.CmdHandler(dialogs.CloseDialogMsg{}),
					util.CmdHandler(dialogs.OpenDialogMsg{Model: hfDialog}),
				)
				
			case "hf_search":
				// Launch Hugging Face search dialog - use browse with search mode
				hfDialog := NewHFBrowseCmp()
				// Start in search mode
				return m, tea.Sequence(
					util.CmdHandler(dialogs.CloseDialogMsg{}),
					util.CmdHandler(dialogs.OpenDialogMsg{Model: hfDialog}),
				)
				
			case "local_download":
				// Download a specific local model
				modelOption := backend.GetModelByID(selectedModel.Model.ID)
				if modelOption == nil {
					return m, util.ReportError(fmt.Errorf("Model %s not found", selectedModel.Model.ID))
				}
				
				// Check for Apple Silicon requirement
				if !backend.IsAppleSilicon() && modelOption.Provider == "mlx" {
					return m, util.ReportError(fmt.Errorf("This model requires Apple Silicon (M1/M2/M3/M4) Mac"))
				}
				
				// Launch download dialog
				onComplete := func(model *backend.ModelOption) {
					// Model downloaded, configure it
					if err := config.Get().ConfigureLocalModel(model); err != nil {
						// Return error message
						return
					}
				}
				
				downloadDialog := backendDlg.NewWithModel(m.orchestrator, modelOption, onComplete)
				return m, tea.Sequence(
					util.CmdHandler(dialogs.CloseDialogMsg{}),
					util.CmdHandler(dialogs.OpenDialogMsg{Model: downloadDialog}),
				)
				
			case "local":
				// Local model already active, just close
				return m, util.CmdHandler(dialogs.CloseDialogMsg{})
				
			default:
				return m, nil
			}
			
		case key.Matches(msg, m.keyMap.Close):
			return m, util.CmdHandler(dialogs.CloseDialogMsg{})
			
		default:
			var cmd tea.Cmd
			m.modelList, cmd = m.modelList.Update(msg)
			return m, cmd
		}
		
	case tea.PasteMsg:
		var cmd tea.Cmd
		m.modelList, cmd = m.modelList.Update(msg)
		return m, cmd
	}
	
	return m, nil
}

func (m *localModelDialogCmp) View() string {
	t := styles.CurrentTheme()
	
	listView := m.modelList.View()
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		t.S().Base.Padding(0, 1, 1, 1).Render(core.Title("Choose Local Model", m.width-4)),
		listView,
		"",
		t.S().Base.Width(m.width-2).PaddingLeft(1).AlignHorizontal(lipgloss.Left).Render(m.help.View(m.keyMap)),
	)
	return m.style().Render(content)
}

func (m *localModelDialogCmp) Cursor() *tea.Cursor {
	cursor := m.modelList.Cursor()
	if cursor != nil {
		cursor = m.moveCursor(cursor)
		return cursor
	}
	return nil
}

func (m *localModelDialogCmp) style() lipgloss.Style {
	t := styles.CurrentTheme()
	return t.S().Base.
		Width(m.width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocus)
}

func (m *localModelDialogCmp) listWidth() int {
	if m.width <= 2 {
		return 10 // Default width
	}
	return max(10, m.width-2)
}

func (m *localModelDialogCmp) listHeight() int {
	if m.wHeight == 0 {
		// Conservative default when we don't know terminal size yet
		// This prevents overflow on initial render
		return 5
	}
	// Reserve space for dialog chrome:
	// - Title: 2 lines
	// - Borders: 2 lines
	// - Help text: 2 lines
	// - Padding: 2 lines
	// - Bottom margin: 2 lines
	// Total chrome: ~10 lines
	availableHeight := m.wHeight - 10
	// Use at most 2/3 of available height, minimum 5 lines
	return max(5, min(availableHeight*2/3, 20))
}

func (m *localModelDialogCmp) Position() (int, int) {
	// Use reasonable defaults if window size not set yet
	width := m.wWidth
	height := m.wHeight
	if width == 0 {
		width = 80  // Conservative default terminal width
	}
	if height == 0 {
		height = 24  // Conservative default terminal height
	}
	
	// Calculate actual dialog height
	// listHeight already accounts for most chrome, but we need to add:
	// - Title: 2 lines
	// - Borders: 2 lines  
	// - Help: 2 lines
	// - Extra padding: 2 lines
	dialogHeight := m.listHeight() + 8
	
	// Center vertically but ensure it fits
	row := (height - dialogHeight) / 2
	
	// Make sure dialog doesn't go off screen at the bottom
	// Leave at least 3 lines at bottom for status and visibility
	maxRow := height - dialogHeight - 3
	if row > maxRow {
		row = maxRow
	}
	
	// Ensure minimum position at top
	if row < 2 {
		row = 2
	}
	
	// Center horizontally
	col := (width - m.width) / 2
	if col < 2 {
		col = 2
	}
	
	return row, col
}

func (m *localModelDialogCmp) moveCursor(cursor *tea.Cursor) *tea.Cursor {
	row, col := m.Position()
	offset := row + 3 // Border + title
	cursor.Y += offset
	cursor.X = cursor.X + col + 2
	return cursor
}

func (m *localModelDialogCmp) ID() dialogs.DialogID {
	return LocalModelsDialogID
}