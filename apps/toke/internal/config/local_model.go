package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/catwalk/pkg/catwalk"
	"github.com/chasedut/toke/internal/backend"
	"github.com/chasedut/toke/internal/csync"
)

// LocalModelConfig stores configuration for the local model backend
type LocalModelConfig struct {
	Enabled  bool   `json:"enabled"`
	ModelID  string `json:"model_id"`
	Port     int    `json:"port"`
	Endpoint string `json:"endpoint"`
}

// ConfigureLocalModel sets up configuration for a local model
func (c *Config) ConfigureLocalModel(model *backend.ModelOption) error {
	if model == nil {
		return fmt.Errorf("model cannot be nil")
	}
	
	// Determine port based on provider
	port := 11434 // Default for llama.cpp
	if model.Provider == "mlx" {
		port = 11435 // MLX backend uses different port
	}
	
	// Create provider configuration for local model
	localProvider := ProviderConfig{
		ID:      "local",
		Name:    "Local Model",
		BaseURL: fmt.Sprintf("http://localhost:%d/v1", port),
		Type:    catwalk.TypeOpenAI, // Both llama.cpp and MLX serve OpenAI-compatible API
		APIKey:  "", // No API key needed
		Disable: false,
		Models: []catwalk.Model{
			{
				ID:               model.ID,
				Name:             model.Name,
				ContextWindow:    8192,
				DefaultMaxTokens: 4096,
				CostPer1MIn:      0, // Free!
				CostPer1MOut:     0, // Free!
			},
		},
	}
	
	// Store provider config
	if c.Providers == nil {
		c.Providers = csync.NewMap[string, ProviderConfig]()
	}
	c.Providers.Set("local", localProvider)
	
	// Set as default model for both large and small
	c.Models = map[SelectedModelType]SelectedModel{
		SelectedModelTypeLarge: {
			Model:    model.ID,
			Provider: "local",
		},
		SelectedModelTypeSmall: {
			Model:    model.ID,
			Provider: "local",
		},
	}
	
	// Save to config file
	if err := c.SetConfigField("providers.local", localProvider); err != nil {
		return fmt.Errorf("failed to save local provider config: %w", err)
	}
	
	if err := c.SetConfigField("models", c.Models); err != nil {
		return fmt.Errorf("failed to save model config: %w", err)
	}
	
	// Setup agents configuration for the local model
	c.SetupAgents()
	
	// Save local model info to a separate file for persistence
	localConfigPath := filepath.Join(filepath.Dir(c.dataConfigDir), "local_model.json")
	localConfig := LocalModelConfig{
		Enabled:  true,
		ModelID:  model.ID,
		Port:     port,
		Endpoint: fmt.Sprintf("http://localhost:%d/v1", port),
	}
	
	data, err := json.MarshalIndent(localConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal local model config: %w", err)
	}
	
	if err := os.WriteFile(localConfigPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write local model config: %w", err)
	}
	
	return nil
}

// GetLocalModelConfig loads the local model configuration
func (c *Config) GetLocalModelConfig() (*LocalModelConfig, error) {
	localConfigPath := filepath.Join(filepath.Dir(c.dataConfigDir), "local_model.json")
	
	data, err := os.ReadFile(localConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No local model configured
		}
		return nil, fmt.Errorf("failed to read local model config: %w", err)
	}
	
	var config LocalModelConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal local model config: %w", err)
	}
	
	return &config, nil
}

// HasLocalModel checks if a local model is configured
func (c *Config) HasLocalModel() bool {
	config, err := c.GetLocalModelConfig()
	if err != nil {
		return false
	}
	return config != nil && config.Enabled
}

// fixLocalProviderPort updates the local provider port based on the model type
func (c *Config) fixLocalProviderPort() error {
	// Check if local provider exists
	localProvider, exists := c.Providers.Get("local")
	if !exists {
		return nil // No local provider to fix
	}

	// Get the local model configuration to determine the model type
	localConfig, err := c.GetLocalModelConfig()
	if err != nil || localConfig == nil || !localConfig.Enabled {
		return nil // No local model configured
	}

	// Check if this is an MLX model by looking at the model ID
	modelID := localConfig.ModelID
	// MLX models contain "mlx" in their ID (case-insensitive)
	isMLX := strings.Contains(strings.ToLower(modelID), "mlx")

	// Determine the correct port
	correctPort := 11434 // Default for llama.cpp
	if isMLX {
		correctPort = 11435 // MLX backend uses different port
	}

	// Update the BaseURL if it's using the wrong port
	expectedURL := fmt.Sprintf("http://localhost:%d/v1", correctPort)
	if localProvider.BaseURL != expectedURL {
		localProvider.BaseURL = expectedURL
		c.Providers.Set("local", localProvider)
		
		// Also update the local model config file
		localConfig.Port = correctPort
		localConfig.Endpoint = expectedURL
		
		localConfigPath := filepath.Join(filepath.Dir(c.dataConfigDir), "local_model.json")
		data, err := json.MarshalIndent(localConfig, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal local model config: %w", err)
		}
		
		if err := os.WriteFile(localConfigPath, data, 0600); err != nil {
			return fmt.Errorf("failed to write local model config: %w", err)
		}
		
		slog.Info("Fixed local provider port", "model", modelID, "port", correctPort)
	}

	return nil
}

// DisableLocalModel disables the local model
func (c *Config) DisableLocalModel() error {
	// Disable the local provider
	if provider, ok := c.Providers.Get("local"); ok {
		provider.Disable = true
		c.Providers.Set("local", provider)
		
		if err := c.SetConfigField("providers.local.disable", true); err != nil {
			return fmt.Errorf("failed to disable local provider: %w", err)
		}
	}
	
	// Update local model config
	localConfigPath := filepath.Join(filepath.Dir(c.dataConfigDir), "local_model.json")
	config, err := c.GetLocalModelConfig()
	if err != nil {
		return err
	}
	
	if config != nil {
		config.Enabled = false
		data, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal local model config: %w", err)
		}
		
		if err := os.WriteFile(localConfigPath, data, 0600); err != nil {
			return fmt.Errorf("failed to write local model config: %w", err)
		}
	}
	
	return nil
}