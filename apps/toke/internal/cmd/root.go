package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/chasedut/toke/internal/app"
	"github.com/chasedut/toke/internal/config"
	"github.com/chasedut/toke/internal/db"
	"github.com/chasedut/toke/internal/env"
	"github.com/chasedut/toke/internal/tui"
	"github.com/chasedut/toke/internal/version"
	"github.com/charmbracelet/fang"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

// termSize attempts to get the terminal size
type terminalSize struct {
	Width  int
	Height int
}

func termSize() (terminalSize, error) {
	if w, h, err := term.GetSize(os.Stdout.Fd()); err == nil {
		slog.Info("Raw terminal size from term.GetSize",
			"raw_width", w, "raw_height", h)
		// Ensure we have reasonable minimum sizes
		if w < 80 {
			w = 80
		}
		if h < 24 {
			h = 24
		}
		return terminalSize{Width: w, Height: h}, nil
	}
	// Fallback to default size if we can't get terminal size
	slog.Warn("Failed to get terminal size, using defaults")
	return terminalSize{Width: 80, Height: 24}, nil
}

func init() {
	rootCmd.PersistentFlags().StringP("cwd", "c", "", "Current working directory")
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Debug")

	rootCmd.Flags().BoolP("help", "h", false, "Help")
	rootCmd.Flags().BoolP("yolo", "y", false, "Automatically accept all permissions (dangerous mode)")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(depsCmd)
}

var rootCmd = &cobra.Command{
	Use:   "toke",
	Short: "Terminal-based AI assistant for software development",
	Long: `Toke is a powerful terminal-based AI assistant that helps with software development tasks.
It provides an interactive chat interface with AI capabilities, code analysis, and LSP integration
to assist developers in writing, debugging, and understanding code directly from the terminal.`,
	Example: `
# Run in interactive mode
toke

# Run with debug logging
toke -d

# Run with debug logging in a specific directory
toke -d -c /path/to/project

# Print version
toke -v

# Run a single non-interactive prompt
toke run "Explain the use of context in Go"

# Run in dangerous mode (auto-accept all permissions)
toke -y
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check for updates on startup (blocking if update accepted)
		if CheckForUpdateOnStartup(cmd.Context()) {
			// If update was installed, restart the application
			executable, err := os.Executable()
			if err != nil {
				return fmt.Errorf("failed to get executable path: %v", err)
			}
			
			// Prepare the same arguments
			newArgs := os.Args[1:] // Skip the first argument (the program name)
			
			// Execute the new version
			execCmd := exec.Command(executable, newArgs...)
			execCmd.Stdin = os.Stdin
			execCmd.Stdout = os.Stdout
			execCmd.Stderr = os.Stderr
			
			if err := execCmd.Start(); err != nil {
				return fmt.Errorf("failed to restart with new version: %v", err)
			}
			
			// Exit the current process
			os.Exit(0)
		}
		
		app, err := setupApp(cmd)
		if err != nil {
			return err
		}
		defer app.Shutdown()

		// Get terminal size for initial setup
		size, _ := termSize()
		slog.Info("Terminal size obtained", 
			"width", size.Width, "height", size.Height,
			"needsFirstSetup", app.NeedsFirstSetup())
		
		// Set up the TUI.
		program := tea.NewProgram(
			tui.NewWithSize(app, size.Width, size.Height),
			tea.WithAltScreen(),
			tea.WithContext(cmd.Context()),
			tea.WithMouseCellMotion(),            // Use cell motion instead of all motion to reduce event flooding
			tea.WithFilter(tui.MouseEventFilter), // Filter mouse events based on focus state
			tea.WithWindowSize(size.Width, size.Height), // Set initial window size
		)

		go app.Subscribe(program)

		if _, err := program.Run(); err != nil {
			slog.Error("TUI run error", "error", err)
			return fmt.Errorf("TUI error: %v", err)
		}
		return nil
	},
}

func Execute() {
	// Load .env file if it exists
	if err := env.LoadDotEnv(); err != nil {
		// Log but don't fail - .env is optional
		slog.Warn("Failed to load .env file", "error", err)
	}
	
	// Initialize dependencies on first run (except for deps command itself)
	if len(os.Args) < 2 || (len(os.Args) >= 2 && os.Args[1] != "deps") {
		if err := InitializeDependencies(); err != nil {
			slog.Error("Failed to initialize dependencies", "error", err)
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			fmt.Fprintf(os.Stderr, "You can manually install dependencies with: toke deps install\n")
		}
	}
	
	if err := fang.Execute(
		context.Background(),
		rootCmd,
		fang.WithVersion(version.Version),
		fang.WithNotifySignal(os.Interrupt),
	); err != nil {
		os.Exit(1)
	}
}

// setupApp handles the common setup logic for both interactive and non-interactive modes.
// It returns the app instance, config, cleanup function, and any error.
func setupApp(cmd *cobra.Command) (*app.App, error) {
	debug, _ := cmd.Flags().GetBool("debug")
	yolo, _ := cmd.Flags().GetBool("yolo")
	ctx := cmd.Context()

	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return nil, err
	}

	cfg, err := config.Init(cwd, debug)
	if err != nil {
		return nil, err
	}

	if cfg.Permissions == nil {
		cfg.Permissions = &config.Permissions{}
	}
	cfg.Permissions.SkipRequests = yolo

	// Connect to DB; this will also run migrations.
	conn, err := db.Connect(ctx, cfg.Options.DataDirectory)
	if err != nil {
		return nil, err
	}
	
	// Initialize global DB for webshare
	if err := db.InitGlobalDB(ctx, cfg.Options.DataDirectory); err != nil {
		return nil, err
	}

	appInstance, err := app.New(ctx, conn, cfg)
	if err != nil {
		slog.Error("Failed to create app instance", "error", err)
		return nil, err
	}

	return appInstance, nil
}

func MaybePrependStdin(prompt string) (string, error) {
	if term.IsTerminal(os.Stdin.Fd()) {
		return prompt, nil
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return prompt, err
	}
	if fi.Mode()&os.ModeNamedPipe == 0 {
		return prompt, nil
	}
	bts, err := io.ReadAll(os.Stdin)
	if err != nil {
		return prompt, err
	}
	return string(bts) + "\n\n" + prompt, nil
}

func ResolveCwd(cmd *cobra.Command) (string, error) {
	cwd, _ := cmd.Flags().GetString("cwd")
	if cwd != "" {
		err := os.Chdir(cwd)
		if err != nil {
			return "", fmt.Errorf("failed to change directory: %v", err)
		}
		return cwd, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %v", err)
	}
	return cwd, nil
}
