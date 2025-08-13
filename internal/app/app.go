package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/chasedut/toke/internal/config"
	"github.com/chasedut/toke/internal/csync"
	"github.com/chasedut/toke/internal/db"
	"github.com/chasedut/toke/internal/format"
	"github.com/chasedut/toke/internal/history"
	"github.com/chasedut/toke/internal/llm/agent"
	"github.com/chasedut/toke/internal/log"
	"github.com/chasedut/toke/internal/pubsub"

	"github.com/chasedut/toke/internal/lsp"
	"github.com/chasedut/toke/internal/message"
	"github.com/chasedut/toke/internal/backend"
	"github.com/chasedut/toke/internal/permission"
	"github.com/chasedut/toke/internal/session"
)

type App struct {
	Sessions    session.Service
	Messages    message.Service
	History     history.Service
	Permissions permission.Service

	CoderAgent agent.Service

	LSPClients map[string]*lsp.Client

	clientsMutex sync.RWMutex

	watcherCancelFuncs *csync.Slice[context.CancelFunc]
	lspWatcherWG       sync.WaitGroup

	config *config.Config

	serviceEventsWG *sync.WaitGroup
	eventsCtx       context.Context
	events          chan tea.Msg
	tuiWG           *sync.WaitGroup

	// global context and cleanup functions
	globalCtx    context.Context
	cleanupFuncs []func()

	// Local model backend
	localBackend    *backend.Orchestrator
	needsFirstSetup bool
}

// New initializes a new applcation instance.
func New(ctx context.Context, conn *sql.DB, cfg *config.Config) (*App, error) {
	q := db.New(conn)
	sessions := session.NewService(q)
	messages := message.NewService(q)
	files := history.NewService(q, conn)
	skipPermissionsRequests := cfg.Permissions != nil && cfg.Permissions.SkipRequests
	allowedTools := []string{}
	if cfg.Permissions != nil && cfg.Permissions.AllowedTools != nil {
		allowedTools = cfg.Permissions.AllowedTools
	}

	app := &App{
		Sessions:    sessions,
		Messages:    messages,
		History:     files,
		Permissions: permission.NewPermissionService(cfg.WorkingDir(), skipPermissionsRequests, allowedTools),
		LSPClients:  make(map[string]*lsp.Client),

		globalCtx: ctx,

		config: cfg,

		watcherCancelFuncs: csync.NewSlice[context.CancelFunc](),

		events:          make(chan tea.Msg, 100),
		serviceEventsWG: &sync.WaitGroup{},
		tuiWG:           &sync.WaitGroup{},
	}

	app.setupEvents()

	// Initialize LSP clients in the background.
	app.initLSPClients(ctx)

	// Check for local model configuration
	if localConfig, err := cfg.GetLocalModelConfig(); err == nil && localConfig != nil && localConfig.Enabled {
		// Initialize local backend
		dataDir := ".toke"
		if cfg.Options != nil && cfg.Options.DataDirectory != "" {
			dataDir = cfg.Options.DataDirectory
		}
		app.localBackend = backend.NewOrchestrator(dataDir)
		
		// Start local backend if model is configured
		if model := backend.GetModelByID(localConfig.ModelID); model != nil {
			// Setup and start in background
			go func() {
				if err := app.localBackend.SetupModel(ctx, model, func(downloaded, total int64) {
					// Progress is logged, not shown in non-interactive mode
					slog.Debug("Model download progress", "downloaded", downloaded, "total", total)
				}); err != nil {
					slog.Error("Failed to setup local model", "error", err)
					return
				}
				
				if err := app.localBackend.Start(ctx); err != nil {
					slog.Error("Failed to start local backend", "error", err)
				}
			}()
		}
	}

	// Check if any providers are configured
	if cfg.IsConfigured() {
		if err := app.InitCoderAgent(); err != nil {
			return nil, fmt.Errorf("failed to initialize coder agent: %w", err)
		}
	} else {
		// No providers configured - mark for first-time setup
		app.needsFirstSetup = true
		slog.Info("No providers configured - will show first-time setup")
	}
	
	return app, nil
}

// Config returns the application configuration.
func (app *App) Config() *config.Config {
	return app.config
}

// RunNonInteractive handles the execution flow when a prompt is provided via
// CLI flag.
func (app *App) RunNonInteractive(ctx context.Context, prompt string, quiet bool) error {
	slog.Info("Running in non-interactive mode")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start spinner if not in quiet mode.
	var spinner *format.Spinner
	if !quiet {
		spinner = format.NewSpinner(ctx, cancel, "Generating")
		spinner.Start()
	}

	// Helper function to stop spinner once.
	stopSpinner := func() {
		if !quiet && spinner != nil {
			spinner.Stop()
			spinner = nil
		}
	}
	defer stopSpinner()

	const maxPromptLengthForTitle = 100
	titlePrefix := "Non-interactive: "
	var titleSuffix string

	if len(prompt) > maxPromptLengthForTitle {
		titleSuffix = prompt[:maxPromptLengthForTitle] + "..."
	} else {
		titleSuffix = prompt
	}
	title := titlePrefix + titleSuffix

	sess, err := app.Sessions.Create(ctx, title)
	if err != nil {
		return fmt.Errorf("failed to create session for non-interactive mode: %w", err)
	}
	slog.Info("Created session for non-interactive run", "session_id", sess.ID)

	// Automatically approve all permission requests for this non-interactive session
	app.Permissions.AutoApproveSession(sess.ID)

	done, err := app.CoderAgent.Run(ctx, sess.ID, prompt)
	if err != nil {
		return fmt.Errorf("failed to start agent processing stream: %w", err)
	}

	messageEvents := app.Messages.Subscribe(ctx)
	readBts := 0

	for {
		select {
		case result := <-done:
			stopSpinner()

			if result.Error != nil {
				if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, agent.ErrRequestCancelled) {
					slog.Info("Non-interactive: agent processing cancelled", "session_id", sess.ID)
					return nil
				}
				return fmt.Errorf("agent processing failed: %w", result.Error)
			}

			msgContent := result.Message.Content().String()
			if len(msgContent) < readBts {
				slog.Error("Non-interactive: message content is shorter than read bytes", "message_length", len(msgContent), "read_bytes", readBts)
				return fmt.Errorf("message content is shorter than read bytes: %d < %d", len(msgContent), readBts)
			}
			fmt.Println(msgContent[readBts:])

			slog.Info("Non-interactive: run completed", "session_id", sess.ID)
			return nil

		case event := <-messageEvents:
			msg := event.Payload
			if msg.SessionID == sess.ID && msg.Role == message.Assistant && len(msg.Parts) > 0 {
				stopSpinner()
				part := msg.Content().String()[readBts:]
				fmt.Print(part)
				readBts += len(part)
			}

		case <-ctx.Done():
			stopSpinner()
			return ctx.Err()
		}
	}
}

func (app *App) UpdateAgentModel() error {
	return app.CoderAgent.UpdateModel()
}

func (app *App) setupEvents() {
	ctx, cancel := context.WithCancel(app.globalCtx)
	app.eventsCtx = ctx
	setupSubscriber(ctx, app.serviceEventsWG, "sessions", app.Sessions.Subscribe, app.events)
	setupSubscriber(ctx, app.serviceEventsWG, "messages", app.Messages.Subscribe, app.events)
	setupSubscriber(ctx, app.serviceEventsWG, "permissions", app.Permissions.Subscribe, app.events)
	setupSubscriber(ctx, app.serviceEventsWG, "permissions-notifications", app.Permissions.SubscribeNotifications, app.events)
	setupSubscriber(ctx, app.serviceEventsWG, "history", app.History.Subscribe, app.events)
	setupSubscriber(ctx, app.serviceEventsWG, "mcp", agent.SubscribeMCPEvents, app.events)
	setupSubscriber(ctx, app.serviceEventsWG, "lsp", SubscribeLSPEvents, app.events)
	cleanupFunc := func() {
		cancel()
		app.serviceEventsWG.Wait()
	}
	app.cleanupFuncs = append(app.cleanupFuncs, cleanupFunc)
}

func setupSubscriber[T any](
	ctx context.Context,
	wg *sync.WaitGroup,
	name string,
	subscriber func(context.Context) <-chan pubsub.Event[T],
	outputCh chan<- tea.Msg,
) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		subCh := subscriber(ctx)
		for {
			select {
			case event, ok := <-subCh:
				if !ok {
					slog.Debug("subscription channel closed", "name", name)
					return
				}
				var msg tea.Msg = event
				select {
				case outputCh <- msg:
				case <-time.After(2 * time.Second):
					slog.Warn("message dropped due to slow consumer", "name", name)
				case <-ctx.Done():
					slog.Debug("subscription cancelled", "name", name)
					return
				}
			case <-ctx.Done():
				slog.Debug("subscription cancelled", "name", name)
				return
			}
		}
	}()
}

func (app *App) InitCoderAgent() error {
	coderAgentCfg := app.config.Agents["coder"]
	if coderAgentCfg.ID == "" {
		return fmt.Errorf("coder agent configuration is missing")
	}
	var err error
	app.CoderAgent, err = agent.NewAgent(
		app.globalCtx,
		coderAgentCfg,
		app.Permissions,
		app.Sessions,
		app.Messages,
		app.History,
		app.LSPClients,
	)
	if err != nil {
		slog.Error("Failed to create coder agent", "err", err)
		return err
	}

	// Add MCP client cleanup to shutdown process
	app.cleanupFuncs = append(app.cleanupFuncs, agent.CloseMCPClients)

	setupSubscriber(app.eventsCtx, app.serviceEventsWG, "coderAgent", app.CoderAgent.Subscribe, app.events)
	return nil
}

// Subscribe sends events to the TUI as tea.Msgs.
func (app *App) Subscribe(program *tea.Program) {
	defer log.RecoverPanic("app.Subscribe", func() {
		slog.Info("TUI subscription panic: attempting graceful shutdown")
		program.Quit()
	})

	app.tuiWG.Add(1)
	tuiCtx, tuiCancel := context.WithCancel(app.globalCtx)
	app.cleanupFuncs = append(app.cleanupFuncs, func() {
		slog.Debug("Cancelling TUI message handler")
		tuiCancel()
		app.tuiWG.Wait()
	})
	defer app.tuiWG.Done()

	for {
		select {
		case <-tuiCtx.Done():
			slog.Debug("TUI message handler shutting down")
			return
		case msg, ok := <-app.events:
			if !ok {
				slog.Debug("TUI message channel closed")
				return
			}
			program.Send(msg)
		}
	}
}

// NeedsFirstSetup returns true if the app needs first-time setup
func (app *App) NeedsFirstSetup() bool {
	return app.needsFirstSetup
}

// SetupLocalModel handles the local model setup process
func (app *App) SetupLocalModel(ctx context.Context, model *backend.ModelOption, progressFn func(status string, downloaded, total int64)) error {
	// Initialize backend if not already done
	if app.localBackend == nil {
		dataDir := ".toke"
		if app.config.Options != nil && app.config.Options.DataDirectory != "" {
			dataDir = app.config.Options.DataDirectory
		}
		app.localBackend = backend.NewOrchestrator(dataDir)
	}
	
	// Run quick setup
	if err := app.localBackend.QuickSetup(ctx, progressFn); err != nil {
		return fmt.Errorf("failed to setup local model: %w", err)
	}
	
	// Configure the model in config
	if err := app.config.ConfigureLocalModel(model); err != nil {
		return fmt.Errorf("failed to configure local model: %w", err)
	}
	
	// Initialize the coder agent with the new configuration
	if err := app.InitCoderAgent(); err != nil {
		return fmt.Errorf("failed to initialize agent with local model: %w", err)
	}
	
	app.needsFirstSetup = false
	return nil
}

// Shutdown performs a graceful shutdown of the application.
func (app *App) Shutdown() {
	if app.CoderAgent != nil {
		app.CoderAgent.CancelAll()
	}

	// Stop local backend if running
	if app.localBackend != nil {
		if err := app.localBackend.Stop(); err != nil {
			slog.Error("Failed to stop local backend", "error", err)
		}
	}

	for cancel := range app.watcherCancelFuncs.Seq() {
		cancel()
	}

	// Wait for all LSP watchers to finish.
	app.lspWatcherWG.Wait()

	// Get all LSP clients.
	app.clientsMutex.RLock()
	clients := make(map[string]*lsp.Client, len(app.LSPClients))
	maps.Copy(clients, app.LSPClients)
	app.clientsMutex.RUnlock()

	// Shutdown all LSP clients.
	for name, client := range clients {
		shutdownCtx, cancel := context.WithTimeout(app.globalCtx, 5*time.Second)
		if err := client.Shutdown(shutdownCtx); err != nil {
			slog.Error("Failed to shutdown LSP client", "name", name, "error", err)
		}
		cancel()
	}

	// Call call cleanup functions.
	for _, cleanup := range app.cleanupFuncs {
		if cleanup != nil {
			cleanup()
		}
	}
}
