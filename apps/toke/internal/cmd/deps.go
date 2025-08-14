package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/chasedut/toke/internal/deps"
	"github.com/chasedut/toke/internal/ui"
	"github.com/spf13/cobra"
)

var (
	depManager *deps.Manager
)

// InitializeDependencies checks and installs required dependencies
func InitializeDependencies() error {
	var baseDir string
	
	if runtime.GOOS == "windows" {
		// On Windows, use LocalAppData
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to get home directory: %w", err)
			}
			localAppData = filepath.Join(homeDir, "AppData", "Local")
		}
		baseDir = filepath.Join(localAppData, "toke", "backends")
	} else {
		// On Unix-like systems, use ~/.toke
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		baseDir = filepath.Join(homeDir, ".toke", "backends")
	}
	
	depManager = deps.NewManager(baseDir, "chasedut/toke")

	if err := depManager.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize dependency manager: %w", err)
	}

	// Check if we need to install anything
	ctx := context.Background()
	manifest, err := depManager.GetManifest(ctx)
	if err != nil {
		return fmt.Errorf("failed to get manifest: %w", err)
	}

	needsInstall := false
	for _, dep := range manifest.Dependencies {
		// Check for local build first
		if localPath, _ := depManager.GetExecutablePath(ctx, dep.Name); localPath != "" {
			if _, err := os.Stat(localPath); err == nil {
				continue // Local build exists
			}
		}

		// Check if installed
		installPath := filepath.Join(baseDir, dep.Name, dep.Executable)
		if _, err := os.Stat(installPath); os.IsNotExist(err) {
			needsInstall = true
			break
		}
	}

	if needsInstall {
		// Show loading screen
		progressChan := make(chan ui.ProgressMsg, 100)
		
		go func() {
			defer close(progressChan)
			
			// Convert dependency manager progress to UI progress
			depProgressChan := depManager.GetProgressChannel()
			go func() {
				for update := range depProgressChan {
					progressChan <- ui.ProgressMsg{
						Name:         update.Name,
						CurrentBytes: update.CurrentBytes,
						TotalBytes:   update.TotalBytes,
						Status:       update.Status,
						Error:        update.Error,
					}
				}
			}()

			// Install dependencies
			if err := depManager.CheckAndInstall(context.Background()); err != nil {
				fmt.Fprintf(os.Stderr, "Error installing dependencies: %v\n", err)
			}
		}()

		// Run the loading screen
		if err := ui.RunLoadingScreen(progressChan); err != nil {
			return fmt.Errorf("loading screen error: %w", err)
		}
	}

	return nil
}

// depsCmd represents the deps command
var depsCmd = &cobra.Command{
	Use:   "deps",
	Short: "Manage Toke dependencies",
	Long:  `Check, install, and update Toke backend dependencies.`,
}

var depsCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check dependency status",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		manifest, err := depManager.GetManifest(ctx)
		if err != nil {
			return err
		}

		versions := depManager.GetInstalledVersions(ctx)

		fmt.Println("📦 Toke Dependencies Status")
		fmt.Println("================================================================================")
		fmt.Printf("%-15s %-20s %-15s %s\n", "BACKEND", "VERSION", "STATUS", "PATH")
		fmt.Println("--------------------------------------------------------------------------------")
		
		for _, dep := range manifest.Dependencies {
			execPath, err := depManager.GetExecutablePath(ctx, dep.Name)
			status := "❌ Not installed"
			version := versions[dep.Name]
			
			if err == nil {
				if _, err := os.Stat(execPath); err == nil {
					if strings.Contains(version, "local") {
						status = "✅ Local build"
					} else if version == "not installed" {
						status = "❌ Not installed"
					} else if version == "unknown" {
						status = "⚠️  No version"
					} else {
						status = "✅ Installed"
					}
				}
			}
			
			// Truncate path if too long
			displayPath := execPath
			if len(displayPath) > 40 {
				displayPath = "..." + displayPath[len(displayPath)-37:]
			}
			
			fmt.Printf("%-15s %-20s %-15s %s\n", dep.Name, version, status, displayPath)
		}
		
		fmt.Println("--------------------------------------------------------------------------------")
		fmt.Printf("\nManifest version: %s\n", manifest.Version)
		fmt.Printf("To check for updates: toke deps update\n")
		
		return nil
	},
}

var depsUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and install updates",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		updates, err := depManager.CheckForUpdates(ctx)
		if err != nil {
			return err
		}

		if len(updates) == 0 {
			fmt.Println("✅ All dependencies are up to date!")
			return nil
		}

		fmt.Printf("Found %d updates available:\n", len(updates))
		for _, dep := range updates {
			fmt.Printf("  - %s (%s)\n", dep.Name, dep.Version)
		}

		// Install updates
		progressChan := make(chan ui.ProgressMsg, 100)
		
		go func() {
			defer close(progressChan)
			
			for _, dep := range updates {
				progressChan <- ui.ProgressMsg{
					Name:   dep.Name,
					Status: "Updating",
				}
				
				// The actual update would happen here
				// For now, we'll use CheckAndInstall which will update if needed
				depManager.CheckAndInstall(context.Background())
			}
		}()

		return ui.RunLoadingScreen(progressChan)
	},
}

var depsInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install all dependencies",
	RunE: func(cmd *cobra.Command, args []string) error {
		progressChan := make(chan ui.ProgressMsg, 100)
		
		go func() {
			defer close(progressChan)
			
			// Convert dependency manager progress to UI progress
			depProgressChan := depManager.GetProgressChannel()
			go func() {
				for update := range depProgressChan {
					progressChan <- ui.ProgressMsg{
						Name:         update.Name,
						CurrentBytes: update.CurrentBytes,
						TotalBytes:   update.TotalBytes,
						Status:       update.Status,
						Error:        update.Error,
					}
				}
			}()

			// Force install all dependencies
			if err := depManager.CheckAndInstall(context.Background()); err != nil {
				fmt.Fprintf(os.Stderr, "Error installing dependencies: %v\n", err)
			}
		}()

		return ui.RunLoadingScreen(progressChan)
	},
}

func init() {
	depsCmd.AddCommand(depsCheckCmd)
	depsCmd.AddCommand(depsUpdateCmd)
	depsCmd.AddCommand(depsInstallCmd)
}