package provider

import (
	"fmt"

	"github.com/charmbracelet/catwalk/pkg/catwalk"
	"github.com/chasedut/toke/internal/config"
)

// NewLocalProvider creates a provider for local llama.cpp server
func NewLocalProvider(modelID string, port int, opts ...ProviderClientOption) Provider {
	// Create provider config for OpenAI-compatible local server
	cfg := config.ProviderConfig{
		ID:      "local",
		Name:    "Local Model",
		Type:    catwalk.TypeOpenAI, // llama.cpp serves OpenAI-compatible API
		BaseURL: fmt.Sprintf("http://localhost:%d/v1", port),
		APIKey:  "", // No API key needed
		Models: []catwalk.Model{
			{
				ID:               modelID,
				Name:             modelID,
				ContextWindow:    8192,
				DefaultMaxTokens: 4096,
				CostPer1MIn:      0, // Free!
				CostPer1MOut:     0, // Free!
			},
		},
	}
	
	// Build provider options
	clientOptions := providerClientOptions{
		baseURL: cfg.BaseURL,
		config:  cfg,
		apiKey:  "",
		model: func(tp config.SelectedModelType) catwalk.Model {
			return cfg.Models[0]
		},
	}
	
	// Apply any additional options
	for _, o := range opts {
		o(&clientOptions)
	}
	
	// Create OpenAI client pointing to local server
	return &baseProvider[OpenAIClient]{
		options: clientOptions,
		client:  newOpenAIClient(clientOptions),
	}
}