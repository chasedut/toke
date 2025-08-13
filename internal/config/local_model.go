package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
	
	// Create provider configuration for local model
	localProvider := ProviderConfig{
		ID:      "local",
		Name:    "Local Model",
		BaseURL: "http://localhost:11434/v1",
		Type:    catwalk.TypeOpenAI, // llama.cpp serves OpenAI-compatible API
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
	
	// Save local model info to a separate file for persistence
	localConfigPath := filepath.Join(filepath.Dir(c.dataConfigDir), "local_model.json")
	localConfig := LocalModelConfig{
		Enabled:  true,
		ModelID:  model.ID,
		Port:     11434,
		Endpoint: "http://localhost:11434/v1",
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