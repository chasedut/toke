package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chasedut/toke/internal/config"
	"github.com/chasedut/toke/internal/update"
	"github.com/chasedut/toke/internal/version"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	checkOnly bool
	force     bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and install updates",
	Long:  `Check for new versions of Toke and optionally install them.`,
	RunE:  runUpdate,
}

func init() {
	updateCmd.Flags().BoolVarP(&checkOnly, "check", "c", false, "Only check for updates without installing")
	updateCmd.Flags().BoolVarP(&force, "force", "f", false, "Force update even if already on latest version")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	updater := update.New(version.Version)
	
	fmt.Println("🔍 Checking for updates...")
	
	info, err := updater.CheckForUpdate(ctx)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	if !info.Available && !force {
		color.Green("✅ You're already running the latest version %s", info.CurrentVersion)
		return nil
	}

	if force && !info.Available {
		color.Yellow("⚠️  Already on latest version, but forcing update anyway...")
	} else {
		color.Cyan("🎉 New version available!")
		fmt.Printf("   Current: %s\n", info.CurrentVersion)
		fmt.Printf("   Latest:  %s\n", info.LatestVersion)
		fmt.Printf("   Released: %s\n", info.PublishedAt.Format("01/02/2006"))
		
		if info.ReleaseNotes != "" {
			fmt.Println("\n📝 Release Notes:")
			lines := strings.Split(info.ReleaseNotes, "\n")
			for i, line := range lines {
				if i > 10 {
					fmt.Println("   ... (truncated, see full notes at " + info.ReleaseURL + ")")
					break
				}
				if line != "" {
					fmt.Printf("   %s\n", line)
				}
			}
		}
	}

	if checkOnly {
		return nil
	}

	fmt.Printf("\n📦 Download size: %.2f MB\n", float64(info.AssetSize)/(1024*1024))
	
	if !confirmUpdate() {
		fmt.Println("Update cancelled.")
		return nil
	}

	fmt.Println("\n⬇️  Downloading update...")
	
	var lastProgress int64
	progressCallback := func(current, total int64) {
		if total > 0 {
			percent := int64(float64(current) / float64(total) * 100)
			if percent != lastProgress && percent%10 == 0 {
				fmt.Printf("   %d%% complete...\n", percent)
				lastProgress = percent
			}
		}
	}
	
	if err := updater.DownloadAndInstall(ctx, info, progressCallback); err != nil {
		return fmt.Errorf("failed to install update: %w", err)
	}

	color.Green("\n✅ Update successfully installed!")
	color.Yellow("🔄 Please restart Toke to use the new version.")
	
	return nil
}

func confirmUpdate() bool {
	fmt.Print("\nDo you want to install this update? [y/N]: ")
	
	var response string
	fmt.Scanln(&response)
	
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

func CheckForUpdateOnStartup(ctx context.Context) bool {
	// Get config to check if auto-update is enabled
	cfg, err := config.LoadUpdateConfig()
	if err != nil || cfg == nil || cfg.Options == nil || cfg.Options.Update == nil {
		cfg = &config.Config{
			Options: &config.Options{
				Update: config.DefaultUpdateOptions(),
			},
		}
	}
	
	updateOpts := cfg.Options.Update
	if updateOpts == nil {
		updateOpts = config.DefaultUpdateOptions()
	}
	
	if !updateOpts.AutoCheck {
		return false
	}
	
	// Check if enough time has passed since last check
	dataDir := cfg.Options.DataDirectory
	if dataDir == "" {
		dataDir = ".toke"
	}
	
	state, err := config.GetUpdateState(dataDir)
	if err != nil {
		return false
	}
	
	hoursSinceLastCheck := time.Since(state.LastCheck).Hours()
	if hoursSinceLastCheck < float64(updateOpts.CheckInterval) {
		return false
	}
	
	updater := update.New(version.Version)
	
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	
	info, err := updater.CheckForUpdate(checkCtx)
	if err != nil {
		return false
	}
	
	// Update the state
	state.LastCheck = time.Now()
	state.LastVersion = info.LatestVersion
	
	if info.Available {
		// Only notify if we haven't notified about this version recently
		if state.LastNotified.IsZero() || state.LastVersion != info.LatestVersion || time.Since(state.LastNotified).Hours() > 24 {
			color.Yellow("\n🆕 A new version of Toke is available: %s (current: %s)", info.LatestVersion, info.CurrentVersion)
			fmt.Printf("   Released: %s\n", info.PublishedAt.Format("01/02/2006"))
			
			if info.ReleaseNotes != "" {
				fmt.Println("\n📝 Release Notes:")
				lines := strings.Split(info.ReleaseNotes, "\n")
				for i, line := range lines {
					if i > 5 {
						fmt.Println("   ... (see more at " + info.ReleaseURL + ")")
						break
					}
					if line != "" {
						fmt.Printf("   %s\n", line)
					}
				}
			}
			
			color.Cyan("\n   Would you like to update now? [y/N]: ")
			
			var response string
			fmt.Scanln(&response)
			response = strings.ToLower(strings.TrimSpace(response))
			
			if response == "y" || response == "yes" {
				fmt.Println("\n⬇️  Downloading update...")
				
				var lastProgress int64
				progressCallback := func(current, total int64) {
					if total > 0 {
						percent := int64(float64(current) / float64(total) * 100)
						if percent != lastProgress && percent%10 == 0 {
							fmt.Printf("   %d%% complete...\n", percent)
							lastProgress = percent
						}
					}
				}
				
				if err := updater.DownloadAndInstall(ctx, info, progressCallback); err != nil {
					color.Red("❌ Failed to install update: %v", err)
					color.Yellow("   You can try again later with 'toke update'")
					return false
				}
				
				color.Green("\n✅ Update successfully installed!")
				color.Yellow("🔄 Toke will now restart with the new version...")
				
				// Save state before returning
				state.LastNotified = time.Now()
				config.SaveUpdateState(dataDir, state)
				
				return true // Signal that we need to restart
			}
			
			state.LastNotified = time.Now()
		}
	}
	
	config.SaveUpdateState(dataDir, state)
	return false
}