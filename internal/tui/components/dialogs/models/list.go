package models

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/catwalk/pkg/catwalk"
	"github.com/chasedut/toke/internal/config"
	"github.com/chasedut/toke/internal/tui/exp/list"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
)

type listModel = list.FilterableGroupList[list.CompletionItem[ModelOption]]

type ModelListComponent struct {
	list           listModel
	modelType      int
	providers      []catwalk.Provider
	excludeLocal   bool // Whether to exclude local models from the list
}

func NewModelListComponent(keyMap list.KeyMap, inputPlaceholder string, shouldResize bool) *ModelListComponent {
	t := styles.CurrentTheme()
	inputStyle := t.S().Base.PaddingLeft(1).PaddingBottom(1)
	options := []list.ListOption{
		list.WithKeyMap(keyMap),
		list.WithWrapNavigation(),
	}
	if shouldResize {
		options = append(options, list.WithResizeByList())
	}
	modelList := list.NewFilterableGroupedList(
		[]list.Group[list.CompletionItem[ModelOption]]{},
		list.WithFilterInputStyle(inputStyle),
		list.WithFilterPlaceholder(inputPlaceholder),
		list.WithFilterListOptions(
			options...,
		),
	)

	return &ModelListComponent{
		list:      modelList,
		modelType: LargeModelType,
	}
}

func (m *ModelListComponent) Init() tea.Cmd {
	var cmds []tea.Cmd
	if len(m.providers) == 0 {
		providers, err := config.Providers()
		filteredProviders := []catwalk.Provider{}
		for _, p := range providers {
			hasAPIKeyEnv := strings.HasPrefix(p.APIKey, "$")
			if hasAPIKeyEnv && p.ID != catwalk.InferenceProviderAzure {
				filteredProviders = append(filteredProviders, p)
			}
		}

		m.providers = filteredProviders
		if err != nil {
			cmds = append(cmds, util.ReportError(err))
		}
	}
	cmds = append(cmds, m.list.Init(), m.SetModelType(m.modelType))
	return tea.Batch(cmds...)
}

func (m *ModelListComponent) Update(msg tea.Msg) (*ModelListComponent, tea.Cmd) {
	u, cmd := m.list.Update(msg)
	m.list = u.(listModel)
	return m, cmd
}

func (m *ModelListComponent) View() string {
	return m.list.View()
}

func (m *ModelListComponent) Cursor() *tea.Cursor {
	return m.list.Cursor()
}

func (m *ModelListComponent) SetSize(width, height int) tea.Cmd {
	return m.list.SetSize(width, height)
}

func (m *ModelListComponent) SelectedModel() *ModelOption {
	s := m.list.SelectedItem()
	if s == nil {
		return nil
	}
	sv := *s
	model := sv.Value()
	return &model
}

func (m *ModelListComponent) SetModelType(modelType int) tea.Cmd {
	t := styles.CurrentTheme()
	m.modelType = modelType

	var groups []list.Group[list.CompletionItem[ModelOption]]
	// first none section
	// selectedItemID := "" // Commented out - we don't auto-select to allow free navigation

	cfg := config.Get()
	var currentModel config.SelectedModel
	if m.modelType == LargeModelType {
		currentModel = cfg.Models[config.SelectedModelTypeLarge]
	} else {
		currentModel = cfg.Models[config.SelectedModelTypeSmall]
	}

	configuredIcon := t.S().Base.Foreground(t.Success).Render(styles.CheckIcon)
	configured := fmt.Sprintf("%s %s", configuredIcon, t.S().Subtle.Render("Configured"))
	
	// Check if Copilot is authenticated and add it first
	if isCopilotAuthenticated() {
		copilotSection := list.NewItemSection("🚁 GitHub Copilot")
		copilotSection.SetInfo(configured)
		
		copilotGroup := list.Group[list.CompletionItem[ModelOption]]{
			Section: copilotSection,
		}
		
		copilotItem := list.NewCompletionItem(
			"✨ Copilot Chat (Premium AI Pair Programmer)",
			ModelOption{
				Provider: catwalk.Provider{ID: "copilot", Name: "GitHub Copilot"},
				Model:    catwalk.Model{ID: "copilot-chat", Name: "Copilot Chat"},
			},
			list.WithCompletionID("copilot:chat"),
		)
		copilotGroup.Items = append(copilotGroup.Items, copilotItem)
		groups = append(groups, copilotGroup)
	}
	
	// Add Local Models section only if a local model is configured and not excluded
	if !m.excludeLocal {
		if localConfig, _ := cfg.GetLocalModelConfig(); localConfig != nil && localConfig.Enabled {
			localSection := list.NewItemSection("🖥️  Local Models")
			localSection.SetInfo(configured)
			
			localGroup := list.Group[list.CompletionItem[ModelOption]]{
				Section: localSection,
			}
			
			localModelItem := list.NewCompletionItem(
				fmt.Sprintf("✓ %s (Active)", localConfig.ModelID),
				ModelOption{
					Provider: catwalk.Provider{ID: "local", Name: "Local Model"},
					Model:    catwalk.Model{ID: localConfig.ModelID, Name: localConfig.ModelID},
				},
				list.WithCompletionID(fmt.Sprintf("local:%s", localConfig.ModelID)),
			)
			localGroup.Items = append(localGroup.Items, localModelItem)
			
			// Mark if this is the currently selected model (but don't auto-focus)
			// Just for tracking, not for selection when currentModel.Provider == "local"
			
			groups = append(groups, localGroup)
		}
	}

	// Create a map to track which providers we've already added
	addedProviders := make(map[string]bool)

	// First, add any configured providers that are not in the known providers list
	// These should appear at the top of the list
	knownProviders, err := config.Providers()
	if err != nil {
		return util.ReportError(err)
	}
	for providerID, providerConfig := range cfg.Providers.Seq2() {
		if providerConfig.Disable {
			continue
		}
		
		// Skip the local provider as it's already handled above
		if providerID == "local" {
			continue
		}

		// Check if this provider is not in the known providers list
		if !slices.ContainsFunc(knownProviders, func(p catwalk.Provider) bool { return p.ID == catwalk.InferenceProvider(providerID) }) ||
			!slices.ContainsFunc(m.providers, func(p catwalk.Provider) bool { return p.ID == catwalk.InferenceProvider(providerID) }) {
			// Convert config provider to provider.Provider format
			configProvider := catwalk.Provider{
				Name:   providerConfig.Name,
				ID:     catwalk.InferenceProvider(providerID),
				Models: make([]catwalk.Model, len(providerConfig.Models)),
			}

			// Convert models
			for i, model := range providerConfig.Models {
				configProvider.Models[i] = catwalk.Model{
					ID:                     model.ID,
					Name:                   model.Name,
					CostPer1MIn:            model.CostPer1MIn,
					CostPer1MOut:           model.CostPer1MOut,
					CostPer1MInCached:      model.CostPer1MInCached,
					CostPer1MOutCached:     model.CostPer1MOutCached,
					ContextWindow:          model.ContextWindow,
					DefaultMaxTokens:       model.DefaultMaxTokens,
					CanReason:              model.CanReason,
					HasReasoningEffort:     model.HasReasoningEffort,
					DefaultReasoningEffort: model.DefaultReasoningEffort,
					SupportsImages:         model.SupportsImages,
				}
			}

			// Add this unknown provider to the list
			name := configProvider.Name
			if name == "" {
				name = string(configProvider.ID)
			}
			section := list.NewItemSection(name)
			section.SetInfo(configured)
			group := list.Group[list.CompletionItem[ModelOption]]{
				Section: section,
			}
			for _, model := range configProvider.Models {
				name := model.Name
				// Add checkmark if this is the current model
				if model.ID == currentModel.Model && string(configProvider.ID) == currentModel.Provider {
					name = "✓ " + name + " (Active)"
				}
				
				item := list.NewCompletionItem(name, ModelOption{
					Provider: configProvider,
					Model:    model,
				},
					list.WithCompletionID(
						fmt.Sprintf("%s:%s", providerConfig.ID, model.ID),
					),
				)

				group.Items = append(group.Items, item)
				// Don't auto-select, just mark visually
				// if model.ID == currentModel.Model && string(configProvider.ID) == currentModel.Provider {
				// 	selectedItemID = item.ID()
				// }
			}
			groups = append(groups, group)

			addedProviders[providerID] = true
		}
	}

	// Then add the known providers from the predefined list
	for _, provider := range m.providers {
		// Skip if we already added this provider as an unknown provider
		if addedProviders[string(provider.ID)] {
			continue
		}
		
		// Skip the local provider as it's already handled above
		if string(provider.ID) == "local" {
			continue
		}

		// Check if this provider is configured and not disabled
		if providerConfig, exists := cfg.Providers.Get(string(provider.ID)); exists && providerConfig.Disable {
			continue
		}

		name := provider.Name
		if name == "" {
			name = string(provider.ID)
		}

		section := list.NewItemSection(name)
		if _, ok := cfg.Providers.Get(string(provider.ID)); ok {
			section.SetInfo(configured)
		}
		group := list.Group[list.CompletionItem[ModelOption]]{
			Section: section,
		}
		for _, model := range provider.Models {
			name := model.Name
			// Add checkmark if this is the current model
			if model.ID == currentModel.Model && string(provider.ID) == currentModel.Provider {
				name = "✓ " + name + " (Active)"
			}
			
			item := list.NewCompletionItem(name, ModelOption{
				Provider: provider,
				Model:    model,
			},
				list.WithCompletionID(
					fmt.Sprintf("%s:%s", provider.ID, model.ID),
				),
			)
			group.Items = append(group.Items, item)
			// Don't auto-select, just mark visually
			// if model.ID == currentModel.Model && string(provider.ID) == currentModel.Provider {
			// 	selectedItemID = item.ID()
			// }
		}
		groups = append(groups, group)
	}

	var cmds []tea.Cmd

	cmd := m.list.SetGroups(groups)

	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	
	// Don't auto-select any item - let user navigate freely from the beginning
	// This allows navigation to setup options at the top
	// if selectedItemID != "" {
	// 	cmd = m.list.SetSelected(selectedItemID)
	// 	if cmd != nil {
	// 		cmds = append(cmds, cmd)
	// 	}
	// }

	return tea.Sequence(cmds...)
}

// GetModelType returns the current model type
func (m *ModelListComponent) GetModelType() int {
	return m.modelType
}

func (m *ModelListComponent) SetInputPlaceholder(placeholder string) {
	m.list.SetInputPlaceholder(placeholder)
}

// isCopilotAuthenticated checks if GitHub Copilot is authenticated
func isCopilotAuthenticated() bool {
	// Check if gh is authenticated first
	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return false
	}
	
	// Check if Copilot extension is installed and authenticated
	cmd = exec.Command("gh", "extension", "list")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	
	// Check if copilot is in the list of extensions
	if !strings.Contains(string(output), "copilot") {
		return false
	}
	
	// Try to check copilot status
	cmd = exec.Command("gh", "copilot", "status")
	if err := cmd.Run(); err != nil {
		return false
	}
	
	return true
}
