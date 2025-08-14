package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

const (
	InitFlagFilename = "init"
)

type ProjectInitFlag struct {
	Initialized bool `json:"initialized"`
}

// TODO: we need to remove the global config instance keeping it now just until everything is migrated
var instance atomic.Pointer[Config]

func Init(workingDir string, debug bool) (*Config, error) {
	cfg, err := Load(workingDir, debug)
	if err != nil {
		return nil, err
	}
	instance.Store(cfg)
	return instance.Load(), nil
}

func Get() *Config {
	cfg := instance.Load()
	return cfg
}

func ProjectNeedsInitialization() (bool, error) {
	cfg := Get()
	if cfg == nil {
		return false, fmt.Errorf("config not loaded")
	}

	flagFilePath := filepath.Join(cfg.Options.DataDirectory, InitFlagFilename)

	_, err := os.Stat(flagFilePath)
	if err == nil {
		return false, nil
	}

	if !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to check init flag file: %w", err)
	}

	tokeExists, err := tokeMdExists(cfg.WorkingDir())
	if err != nil {
		return false, fmt.Errorf("failed to check for TOKE.md files: %w", err)
	}
	if tokeExists {
		return false, nil
	}

	return true, nil
}

func tokeMdExists(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := strings.ToLower(entry.Name())
		if name == "toke.md" {
			return true, nil
		}
	}

	return false, nil
}

func MarkProjectInitialized() error {
	cfg := Get()
	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}
	flagFilePath := filepath.Join(cfg.Options.DataDirectory, InitFlagFilename)

	file, err := os.Create(flagFilePath)
	if err != nil {
		return fmt.Errorf("failed to create init flag file: %w", err)
	}
	defer file.Close()

	return nil
}

func HasInitialDataConfig() bool {
	// Check if the actual config file exists (not just providers.json)
	cfgPath := GlobalConfigData()
	if _, err := os.Stat(cfgPath); err != nil {
		// No config file at all - definitely need onboarding
		return false
	}
	
	// Also verify that there's actual user configuration
	cfg := Get()
	if cfg == nil {
		return false
	}
	
	// Check if user has selected models configured
	// This is a better indicator of whether onboarding has been completed
	if len(cfg.Models) == 0 {
		// No models selected - show onboarding
		return false
	}
	
	// Check if the selected models actually have valid providers
	for _, model := range cfg.Models {
		if model.Provider != "" && model.Model != "" {
			// User has selected a model - onboarding was completed
			return true
		}
	}
	
	// No valid model selection - show onboarding
	return false
}
