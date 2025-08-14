package shell

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// UserShell represents the type of user's shell
type UserShell string

const (
	UserShellBash UserShell = "bash"
	UserShellZsh  UserShell = "zsh"
	UserShellFish UserShell = "fish"
)

// ShellConfig contains information about a shell configuration
type ShellConfig struct {
	Type       UserShell
	ConfigFile string
	RCFile     string // The actual rc file to modify
}

// DetectUserShell detects the current user's shell
func DetectUserShell() (UserShell, error) {
	// First check the SHELL environment variable
	shell := os.Getenv("SHELL")
	if shell == "" {
		// Fallback to checking the current process
		parentPID := os.Getppid()
		cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", parentPID), "-o", "comm=")
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to detect shell: %w", err)
		}
		shell = strings.TrimSpace(string(output))
	}
	
	// Extract the shell name from the path
	shellName := filepath.Base(shell)
	
	switch {
	case strings.Contains(shellName, "zsh"):
		return UserShellZsh, nil
	case strings.Contains(shellName, "bash"):
		return UserShellBash, nil
	case strings.Contains(shellName, "fish"):
		return UserShellFish, nil
	default:
		// Default to bash if unknown
		return UserShellBash, nil
	}
}

// GetShellConfig returns the configuration files for the detected shell
func GetShellConfig() (*ShellConfig, error) {
	shellType, err := DetectUserShell()
	if err != nil {
		return nil, err
	}
	
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	
	config := &ShellConfig{
		Type: shellType,
	}
	
	switch shellType {
	case UserShellZsh:
		config.ConfigFile = filepath.Join(homeDir, ".zshrc")
		config.RCFile = config.ConfigFile
	case UserShellBash:
		// Check for .bash_profile on macOS, .bashrc on Linux
		profilePath := filepath.Join(homeDir, ".bash_profile")
		bashrcPath := filepath.Join(homeDir, ".bashrc")
		
		// On macOS, .bash_profile is typically used
		if _, err := os.Stat(profilePath); err == nil {
			config.ConfigFile = profilePath
		} else if _, err := os.Stat(bashrcPath); err == nil {
			config.ConfigFile = bashrcPath
		} else {
			// Default to .bash_profile on macOS
			if strings.Contains(strings.ToLower(os.Getenv("OSTYPE")), "darwin") {
				config.ConfigFile = profilePath
			} else {
				config.ConfigFile = bashrcPath
			}
		}
		config.RCFile = config.ConfigFile
	case UserShellFish:
		config.ConfigFile = filepath.Join(homeDir, ".config", "fish", "config.fish")
		config.RCFile = config.ConfigFile
	}
	
	return config, nil
}

// HasShortcut checks if a toke shortcut already exists in the shell config
func HasShortcut() (bool, error) {
	config, err := GetShellConfig()
	if err != nil {
		return false, err
	}
	
	// Check if the config file exists
	if _, err := os.Stat(config.RCFile); os.IsNotExist(err) {
		return false, nil
	}
	
	// Read the config file
	content, err := os.ReadFile(config.RCFile)
	if err != nil {
		return false, fmt.Errorf("failed to read shell config: %w", err)
	}
	
	// Check for toke alias or PATH modification
	contentStr := string(content)
	hasAlias := strings.Contains(contentStr, "alias toke=") || 
	           strings.Contains(contentStr, "alias tk=")
	hasPath := strings.Contains(contentStr, "/.toke/bin") || 
	          strings.Contains(contentStr, "/toke")
	
	return hasAlias || hasPath, nil
}

// InstallShortcut installs the toke shortcut to the user's shell configuration
func InstallShortcut(execPath string) error {
	config, err := GetShellConfig()
	if err != nil {
		return err
	}
	
	// Create the config file if it doesn't exist
	if _, err := os.Stat(config.RCFile); os.IsNotExist(err) {
		// Create parent directory if needed
		dir := filepath.Dir(config.RCFile)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
		
		// Create the file
		file, err := os.Create(config.RCFile)
		if err != nil {
			return fmt.Errorf("failed to create shell config: %w", err)
		}
		file.Close()
	}
	
	// Read existing content
	content, err := os.ReadFile(config.RCFile)
	if err != nil {
		return fmt.Errorf("failed to read shell config: %w", err)
	}
	
	// Prepare the shortcut content based on shell type
	var shortcutContent string
	switch config.Type {
	case UserShellFish:
		shortcutContent = fmt.Sprintf(`
# Toke - AI coding assistant
alias toke='%s'
alias tk='%s'
`, execPath, execPath)
	default: // Bash and Zsh
		shortcutContent = fmt.Sprintf(`
# Toke - AI coding assistant
alias toke='%s'
alias tk='%s'
`, execPath, execPath)
	}
	
	// Append to the config file
	newContent := string(content)
	if !strings.HasSuffix(newContent, "\n") && len(newContent) > 0 {
		newContent += "\n"
	}
	newContent += shortcutContent
	
	// Write back to the file
	if err := os.WriteFile(config.RCFile, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write shell config: %w", err)
	}
	
	return nil
}

// GetShellName returns a user-friendly name for the shell
func GetShellName(shellType UserShell) string {
	switch shellType {
	case UserShellZsh:
		return "Zsh"
	case UserShellBash:
		return "Bash"
	case UserShellFish:
		return "Fish"
	default:
		return "Shell"
	}
}

// GetShellSourceCommand returns the command to source the shell config
func GetShellSourceCommand() (string, error) {
	config, err := GetShellConfig()
	if err != nil {
		return "", err
	}
	
	switch config.Type {
	case UserShellFish:
		return fmt.Sprintf("source %s", config.RCFile), nil
	default:
		return fmt.Sprintf("source %s", config.RCFile), nil
	}
}