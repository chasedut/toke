package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type UpdateOptions struct {
	// Whether to check for updates automatically on startup
	AutoCheck bool `json:"auto_check,omitempty" jsonschema:"description=Automatically check for updates on startup,default=true"`
	
	// How often to check for updates (in hours)
	CheckInterval int `json:"check_interval,omitempty" jsonschema:"description=Hours between automatic update checks,minimum=1,maximum=720,default=24"`
	
	// Whether to skip update checks for pre-release versions
	SkipPrerelease bool `json:"skip_prerelease,omitempty" jsonschema:"description=Skip pre-release versions when checking for updates,default=false"`
	
	// Last time an update check was performed (internal use)
	LastCheck time.Time `json:"last_check,omitempty" jsonschema:"description=Timestamp of last update check (managed automatically)"`
}

type UpdateState struct {
	LastCheck      time.Time `json:"last_check"`
	LastVersion    string    `json:"last_version"`
	LastNotified   time.Time `json:"last_notified"`
	SkippedVersion string    `json:"skipped_version,omitempty"`
}

func GetUpdateState(dataDir string) (*UpdateState, error) {
	stateFile := filepath.Join(dataDir, ".update-state.json")
	
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &UpdateState{}, nil
		}
		return nil, err
	}
	
	var state UpdateState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	
	return &state, nil
}

func SaveUpdateState(dataDir string, state *UpdateState) error {
	stateFile := filepath.Join(dataDir, ".update-state.json")
	
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(stateFile, data, 0600)
}

func DefaultUpdateOptions() *UpdateOptions {
	return &UpdateOptions{
		AutoCheck:      true,
		CheckInterval:  24,
		SkipPrerelease: false,
	}
}