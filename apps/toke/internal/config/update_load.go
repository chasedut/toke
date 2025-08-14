package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LoadUpdateConfig loads just the configuration needed for update checks
func LoadUpdateConfig() (*Config, error) {
	// Try to find config file in current directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	
	// Look for .toke/config.json
	configPath := filepath.Join(cwd, ".toke", "config.json")
	
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty config with defaults
			return &Config{
				Options: &Options{
					Update: DefaultUpdateOptions(),
					DataDirectory: ".toke",
				},
			}, nil
		}
		return nil, err
	}
	
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	
	// Ensure update options exist
	if cfg.Options == nil {
		cfg.Options = &Options{}
	}
	if cfg.Options.Update == nil {
		cfg.Options.Update = DefaultUpdateOptions()
	}
	if cfg.Options.DataDirectory == "" {
		cfg.Options.DataDirectory = ".toke"
	}
	
	return &cfg, nil
}