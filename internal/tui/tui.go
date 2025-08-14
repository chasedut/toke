package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/v2/key"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/chasedut/toke/internal/app"
	"github.com/chasedut/toke/internal/backend"
	"github.com/chasedut/toke/internal/config"
	"github.com/chasedut/toke/internal/llm/agent"
	"github.com/chasedut/toke/internal/permission"
	"github.com/chasedut/toke/internal/pubsub"
	"github.com/chasedut/toke/internal/shell"
	cmpChat "github.com/chasedut/toke/internal/tui/components/chat"
	"github.com/chasedut/toke/internal/tui/components/chat/splash"
	"github.com/chasedut/toke/internal/tui/components/completions"
	"github.com/chasedut/toke/internal/tui/components/core"
	"github.com/chasedut/toke/internal/tui/components/core/background"
	"github.com/chasedut/toke/internal/tui/components/core/layout"
	"github.com/chasedut/toke/internal/tui/components/core/status"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	backendDlg "github.com/chasedut/toke/internal/tui/components/dialogs/backend"
	"github.com/chasedut/toke/internal/tui/components/dialogs/commands"
	"github.com/chasedut/toke/internal/tui/components/dialogs/compact"
	"github.com/chasedut/toke/internal/tui/components/dialogs/filepicker"
	"github.com/chasedut/toke/internal/tui/components/dialogs/models"
	"github.com/chasedut/toke/internal/tui/components/dialogs/permissions"
	"github.com/chasedut/toke/internal/tui/components/dialogs/quit"
	"github.com/chasedut/toke/internal/tui/components/dialogs/sessions"
	shellDlg "github.com/chasedut/toke/internal/tui/components/dialogs/shell"
	"github.com/chasedut/toke/internal/tui/components/dialogs/ngrokauth"
	webshareDialog "github.com/chasedut/toke/internal/tui/components/dialogs/webshare"
	"github.com/chasedut/toke/internal/tui/loading"
	"github.com/chasedut/toke/internal/tui/page"
	"github.com/chasedut/toke/internal/tui/page/chat"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
	"github.com/chasedut/toke/internal/webshare"
	"github.com/charmbracelet/lipgloss/v2"
)

var lastMouseEvent time.Time

func MouseEventFilter(m tea.Model, msg tea.Msg) tea.Msg {
	switch msg.(type) {
	case tea.MouseWheelMsg, tea.MouseMotionMsg:
		now := time.Now()
		// trackpad is sending too many requests
		if now.Sub(lastMouseEvent) < 15*time.Millisecond {
			return nil
		}
		lastMouseEvent = now
	}
	return msg
}

// appModel represents the main application model that manages pages, dialogs, and UI state.
type appModel struct {
	wWidth, wHeight int // Window dimensions
	width, height   int
	keyMap          KeyMap

	currentPage  page.PageID
	previousPage page.PageID
	pages        map[page.PageID]util.Model
	loadedPages  map[page.PageID]bool

	// Status
	status          status.StatusCmp
	showingFullHelp bool

	app *app.App

	dialog       dialogs.DialogCmp
	completions  completions.Completions
	isConfigured bool

	// Chat Page Specific
	selectedSessionID string // The ID of the currently selected session
	
	// Shell shortcut check
	hasCheckedShortcut bool
	
	// Loading state
	isLoading      bool
	loadingScreen  tea.Model
	
	// Background downloads
	downloadManager *background.DownloadManager
	
	// Web sharing
	webShare *webshare.SessionShare
}

// Init initializes the application model and returns initial commands.
func (a *appModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	
	// Request initial terminal size immediately
	cmds = append(cmds, func() tea.Msg { return tea.RequestWindowSize() })
	
	// Initialize dialog component (needed even during loading)
	if a.dialog != nil {
		cmd := a.dialog.Init()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	
	// If loading, initialize the loading screen
	if a.isLoading {
		if a.loadingScreen != nil {
			cmd := a.loadingScreen.Init()
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		// Start the actual initialization after a brief delay
		initCmd := a.startInitialization()
		if initCmd != nil {
			cmds = append(cmds, initCmd)
		}
		return tea.Batch(cmds...)
	}
	
	// Normal initialization
	if page, exists := a.pages[a.currentPage]; exists && page != nil {
		cmd := page.Init()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		a.loadedPages[a.currentPage] = true
	}

	if a.status != nil {
		cmd := a.status.Init()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	cmds = append(cmds, tea.EnableMouseAllMotion)
	
	// Check for shell shortcuts immediately
	shortcutCmd := a.checkShellShortcuts()
	if shortcutCmd != nil {
		cmds = append(cmds, shortcutCmd)
	}

	return tea.Batch(cmds...)
}

// InitializationCompleteMsg signals that initialization is complete
type InitializationCompleteMsg struct{}

// Update handles incoming messages and updates the application state.
func (a *appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle loading screen updates
	if a.isLoading {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			a.wWidth, a.wHeight = msg.Width, msg.Height
			a.width, a.height = msg.Width, msg.Height-2 // Account for status bar
			if a.loadingScreen != nil {
				a.loadingScreen, _ = a.loadingScreen.Update(msg)
			}
			return a, nil
			
		case InitializationCompleteMsg:
			// Transition from loading to main app
			a.isLoading = false
			// Initialize the main app components only if not already initialized
			var cmds []tea.Cmd
			if _, loaded := a.loadedPages[a.currentPage]; !loaded {
				cmd := a.pages[a.currentPage].Init()
				cmds = append(cmds, cmd)
				a.loadedPages[a.currentPage] = true
			}
			
			// Initialize status if needed
			if a.status != nil {
				cmd := a.status.Init()
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
			
			// Apply initial window size if we have it
			if a.wWidth > 0 && a.wHeight > 0 {
				cmds = append(cmds, a.handleWindowResize(a.wWidth, a.wHeight))
			}
			
			cmds = append(cmds, tea.EnableMouseAllMotion)
			cmds = append(cmds, a.checkShellShortcuts())
			
			return a, tea.Batch(cmds...)
			
		// Dialog messages should be handled even during loading
		case dialogs.OpenDialogMsg, dialogs.CloseDialogMsg:
			u, dialogCmd := a.dialog.Update(msg)
			a.dialog = u.(dialogs.DialogCmp)
			// Also update loading screen if active
			if a.loadingScreen != nil {
				var loadingCmd tea.Cmd
				a.loadingScreen, loadingCmd = a.loadingScreen.Update(msg)
				return a, tea.Batch(dialogCmd, loadingCmd)
			}
			return a, dialogCmd
			
		default:
			// Update loading screen animation
			if a.loadingScreen != nil {
				var cmd tea.Cmd
				a.loadingScreen, cmd = a.loadingScreen.Update(msg)
				return a, cmd
			}
		}
		return a, nil
	}
	
	var cmds []tea.Cmd
	var cmd tea.Cmd
	a.isConfigured = config.HasInitialDataConfig()

	switch msg := msg.(type) {
	case tea.KeyboardEnhancementsMsg:
		for id, page := range a.pages {
			m, pageCmd := page.Update(msg)
			a.pages[id] = m.(util.Model)
			if pageCmd != nil {
				cmds = append(cmds, pageCmd)
			}
		}
		return a, tea.Batch(cmds...)
	case tea.WindowSizeMsg:
		a.wWidth, a.wHeight = msg.Width, msg.Height
		a.completions.Update(msg)
		return a, a.handleWindowResize(msg.Width, msg.Height)

	// Completions messages
	case completions.OpenCompletionsMsg, completions.FilterCompletionsMsg,
		completions.CloseCompletionsMsg, completions.RepositionCompletionsMsg:
		u, completionCmd := a.completions.Update(msg)
		a.completions = u.(completions.Completions)
		return a, completionCmd

	// Dialog messages
	case dialogs.OpenDialogMsg, dialogs.CloseDialogMsg:
		u, completionCmd := a.completions.Update(completions.CloseCompletionsMsg{})
		a.completions = u.(completions.Completions)
		u, dialogCmd := a.dialog.Update(msg)
		a.dialog = u.(dialogs.DialogCmp)
		return a, tea.Batch(completionCmd, dialogCmd)
	case commands.ShowArgumentsDialogMsg:
		return a, util.CmdHandler(
			dialogs.OpenDialogMsg{
				Model: commands.NewCommandArgumentsDialog(
					msg.CommandID,
					msg.Content,
					msg.ArgNames,
				),
			},
		)
	// Page change messages
	case page.PageChangeMsg:
		return a, a.moveToPage(msg.ID)

	// Status Messages
	case util.InfoMsg, util.ClearStatusMsg:
		s, statusCmd := a.status.Update(msg)
		a.status = s.(status.StatusCmp)
		cmds = append(cmds, statusCmd)
		return a, tea.Batch(cmds...)

	// Session
	case cmpChat.SessionSelectedMsg:
		a.selectedSessionID = msg.ID
	case cmpChat.SessionClearedMsg:
		a.selectedSessionID = ""
	
	// Buddy events from web share
	case webshare.BuddyEventMsg:
		// Forward to chat page
		page, pageCmd := a.pages[a.currentPage].Update(msg)
		a.pages[a.currentPage] = page.(util.Model)
		
		// SSE client is now started automatically in startWebShare
		
		return a, tea.Batch(append(cmds, pageCmd)...)
	// Commands
	case commands.SwitchSessionsMsg:
		return a, func() tea.Msg {
			allSessions, _ := a.app.Sessions.List(context.Background())
			return dialogs.OpenDialogMsg{
				Model: sessions.NewSessionDialogCmp(allSessions, a.selectedSessionID),
			}
		}

	case commands.SwitchModelMsg:
		return a, util.CmdHandler(
			dialogs.OpenDialogMsg{
				Model: models.NewModelDialogCmp(),
			},
		)
	case commands.AddNewModelsMsg:
		// Show the new Add Models dialog (similar to Switch Model dialog)
		return a, util.CmdHandler(dialogs.OpenDialogMsg{Model: models.NewAddModelsCmp()})
	
	case backendDlg.MinimizeToBackgroundMsg:
		// Add download to background manager
		a.downloadManager.AddDownload(msg.Model, msg.Progress, msg.Downloaded, msg.Total)
		// Close the dialog
		u, cmd := a.dialog.Update(dialogs.CloseDialogMsg{})
		a.dialog = u.(dialogs.DialogCmp)
		// Show info in status
		return a, tea.Batch(
			cmd,
			util.CmdHandler(util.InfoMsg{
				Type: util.InfoTypeInfo,
				Msg:  fmt.Sprintf("📥 Downloading %s in background... Press 'b' to continue in background", msg.Model.Name),
				TTL:  5 * time.Second,
			}),
		)
	
	case backendDlg.DownloadProgressMsg:
		// Forward to dialog if it's open (for modal progress updates)
		if a.dialog.HasDialogs() {
			u, cmd := a.dialog.Update(msg)
			a.dialog = u.(dialogs.DialogCmp)
			return a, cmd
		}
		return a, nil
	
	case backendDlg.BackgroundDownloadProgressMsg:
		// Update background download manager with progress
		if a.downloadManager != nil && msg.Model != nil {
			progress := float64(0)
			if msg.Total > 0 {
				progress = float64(msg.Downloaded) / float64(msg.Total)
			}
			a.downloadManager.UpdateProgress(msg.Model.ID, progress, msg.Downloaded, msg.Total)
			
			// Continue polling for background download progress
			// The backend dialog's tickBackgroundDownload continues to send these messages
			// We need to re-trigger it to keep the polling going
			return a, backendDlg.ContinueBackgroundTicker(msg.Model)
		}
		return a, nil
	
	case backendDlg.DownloadCompleteMsg:
		// Mark download as complete in manager
		// Note: We need to track which model completed
		if a.downloadManager != nil && a.downloadManager.HasActiveDownloads() {
			// Mark the first active download as complete (simplified)
			for _, dl := range a.downloadManager.GetActiveDownloads() {
				if dl != nil && dl.Model != nil {
					a.downloadManager.CompleteDownload(dl.Model.ID)
					break
				}
			}
		}
		
		// Forward to dialog if it's open
		var cmds []tea.Cmd
		if a.dialog.HasDialogs() {
			u, cmd := a.dialog.Update(msg)
			a.dialog = u.(dialogs.DialogCmp)
			cmds = append(cmds, cmd)
		}
		
		// Show notification that download is complete
		cmds = append(cmds, util.CmdHandler(util.InfoMsg{
			Type: util.InfoTypeInfo,
			Msg:  "✅ Model download complete! Your new model is ready to use.",
			TTL:  10 * time.Second,
		}))
		
		return a, tea.Batch(cmds...)
	// Compact
	case commands.CompactMsg:
		return a, util.CmdHandler(dialogs.OpenDialogMsg{
			Model: compact.NewCompactDialogCmp(a.app.CoderAgent, msg.SessionID, true),
		})
	case commands.InviteBuddyMsg:
		// Start the web share server
		if msg.SessionID == "" {
			// No active session
			return a, util.CmdHandler(util.InfoMsg{
				Type: util.InfoTypeWarn,
				Msg:  "Please start a chat session first before inviting a buddy",
				TTL:  3 * time.Second,
			})
		}
		// Check if already sharing
		if a.webShare != nil {
			// Show existing share dialog
			return a, util.CmdHandler(dialogs.OpenDialogMsg{
				Model: webshareDialog.NewShareDialog(msg.SessionID, a.webShare.GetURLs()),
			})
		}
		return a, a.startWebShare(msg.SessionID)
	case commands.QuitMsg:
		return a, util.CmdHandler(dialogs.OpenDialogMsg{
			Model: quit.NewQuitDialog(),
		})
	case commands.ToggleYoloModeMsg:
		a.app.Permissions.SetSkipRequests(!a.app.Permissions.SkipRequests())
	case ngrokauth.NgrokAuthSuccessMsg:
		// Retry web share after successful auth
		return a, a.startWebShare(msg.SessionID)
	case commands.ToggleHelpMsg:
		a.status.ToggleFullHelp()
		a.showingFullHelp = !a.showingFullHelp
		return a, a.handleWindowResize(a.wWidth, a.wHeight)
	// Model Switch
	case models.ModelSelectedMsg:
		if a.app.CoderAgent.IsBusy() {
			return a, util.ReportWarn("Agent is busy, please wait...")
		}
		config.Get().UpdatePreferredModel(msg.ModelType, msg.Model)

		// Update the agent with the new model/provider configuration
		if err := a.app.UpdateAgentModel(); err != nil {
			return a, util.ReportError(fmt.Errorf("model changed to %s but failed to update agent: %v", msg.Model.Model, err))
		}

		modelTypeName := "large"
		if msg.ModelType == config.SelectedModelTypeSmall {
			modelTypeName = "small"
		}
		return a, util.ReportInfo(fmt.Sprintf("%s model changed to %s", modelTypeName, msg.Model.Model))
	
	// Launch local model setup
	case models.LaunchLocalSetupMsg:
		// Close any existing dialogs and open the backend setup dialog
		// Create the backend setup dialog with callbacks
		onComplete := func(model *backend.ModelOption) tea.Cmd {
			// Model selected and downloaded - configure it
			if err := config.Get().ConfigureLocalModel(model); err != nil {
				return util.ReportError(fmt.Errorf("Failed to configure local model: %v", err))
			}
			// Update the agent to use the local model
			if err := a.app.UpdateAgentModel(); err != nil {
				return util.ReportError(fmt.Errorf("Failed to update agent: %v", err))
			}
			return util.ReportInfo(fmt.Sprintf("Local model %s is now active", model.Name))
		}
		
		onSkip := func() tea.Cmd {
			// User skipped setup
			return util.ReportInfo("Local model setup skipped")
		}
		
		onAPIKey := func() tea.Cmd {
			// User wants to enter API key instead - open models dialog
			return util.CmdHandler(dialogs.OpenDialogMsg{Model: models.NewModelDialogCmp()})
		}
		
		// Note: The backend dialog will need to be updated to handle tea.Cmd callbacks
		_ = onComplete
		_ = onSkip
		_ = onAPIKey
		
		// Create orchestrator for the dialog
		dataDir := ".toke"
		if cfg := config.Get(); cfg.Options != nil && cfg.Options.DataDirectory != "" {
			dataDir = cfg.Options.DataDirectory
		}
		orchestrator := backend.NewOrchestrator(dataDir)
		
		backendDialog := backendDlg.New(
			orchestrator,
			nil, // Will need to update the dialog to handle commands differently
			nil,
			nil,
		)
		return a, util.CmdHandler(dialogs.OpenDialogMsg{Model: backendDialog})

	// File Picker
	case commands.OpenFilePickerMsg:
		if a.dialog.ActiveDialogID() == filepicker.FilePickerID {
			// If the commands dialog is already open, close it
			return a, util.CmdHandler(dialogs.CloseDialogMsg{})
		}
		return a, util.CmdHandler(dialogs.OpenDialogMsg{
			Model: filepicker.NewFilePickerCmp(a.app.Config().WorkingDir()),
		})
	// Permissions
	case pubsub.Event[permission.PermissionNotification]:
		// forward to page
		updated, cmd := a.pages[a.currentPage].Update(msg)
		a.pages[a.currentPage] = updated.(util.Model)
		return a, cmd
	case pubsub.Event[permission.PermissionRequest]:
		return a, util.CmdHandler(dialogs.OpenDialogMsg{
			Model: permissions.NewPermissionDialogCmp(msg.Payload),
		})
	case permissions.PermissionResponseMsg:
		switch msg.Action {
		case permissions.PermissionAllow:
			a.app.Permissions.Grant(msg.Permission)
		case permissions.PermissionAllowForSession:
			a.app.Permissions.GrantPersistent(msg.Permission)
		case permissions.PermissionDeny:
			a.app.Permissions.Deny(msg.Permission)
		}
		return a, nil
	// Agent Events
	case pubsub.Event[agent.AgentEvent]:
		payload := msg.Payload

		// Forward agent events to dialogs
		if a.dialog.HasDialogs() && a.dialog.ActiveDialogID() == compact.CompactDialogID {
			u, dialogCmd := a.dialog.Update(payload)
			a.dialog = u.(dialogs.DialogCmp)
			cmds = append(cmds, dialogCmd)
		}

		// Handle auto-compact logic
		if payload.Done && payload.Type == agent.AgentEventTypeResponse && a.selectedSessionID != "" {
			// Get current session to check token usage
			session, err := a.app.Sessions.Get(context.Background(), a.selectedSessionID)
			if err == nil {
				model := a.app.CoderAgent.Model()
				contextWindow := model.ContextWindow
				tokens := session.CompletionTokens + session.PromptTokens
				if (tokens >= int64(float64(contextWindow)*0.95)) && !config.Get().Options.DisableAutoSummarize { // Show compact confirmation dialog
					cmds = append(cmds, util.CmdHandler(dialogs.OpenDialogMsg{
						Model: compact.NewCompactDialogCmp(a.app.CoderAgent, a.selectedSessionID, false),
					}))
				}
			}
		}

		return a, tea.Batch(cmds...)
	case splash.OnboardingCompleteMsg:
		a.isConfigured = config.HasInitialDataConfig()
		updated, pageCmd := a.pages[a.currentPage].Update(msg)
		a.pages[a.currentPage] = updated.(util.Model)
		cmds = append(cmds, pageCmd)
		return a, tea.Batch(cmds...)
	
	case ShellShortcutCheckMsg:
		// Only show dialog once per session and if needed
		if msg.NeedsSetup && !a.hasCheckedShortcut && a.isConfigured {
			a.hasCheckedShortcut = true
			dialog, err := shellDlg.NewShellSetupDialog()
			if err == nil {
				return a, util.CmdHandler(dialogs.OpenDialogMsg{Model: dialog})
			}
		}
		return a, nil
	// Key Press Messages
	case tea.KeyPressMsg:
		return a, a.handleKeyPressMsg(msg)

	case tea.MouseWheelMsg:
		if a.dialog.HasDialogs() {
			u, dialogCmd := a.dialog.Update(msg)
			a.dialog = u.(dialogs.DialogCmp)
			cmds = append(cmds, dialogCmd)
		} else {
			updated, pageCmd := a.pages[a.currentPage].Update(msg)
			a.pages[a.currentPage] = updated.(util.Model)
			cmds = append(cmds, pageCmd)
		}
		return a, tea.Batch(cmds...)
	case tea.PasteMsg:
		if a.dialog.HasDialogs() {
			u, dialogCmd := a.dialog.Update(msg)
			a.dialog = u.(dialogs.DialogCmp)
			cmds = append(cmds, dialogCmd)
		} else {
			updated, pageCmd := a.pages[a.currentPage].Update(msg)
			a.pages[a.currentPage] = updated.(util.Model)
			cmds = append(cmds, pageCmd)
		}
		return a, tea.Batch(cmds...)
	}
	s, _ := a.status.Update(msg)
	a.status = s.(status.StatusCmp)
	updated, cmd := a.pages[a.currentPage].Update(msg)
	a.pages[a.currentPage] = updated.(util.Model)
	if a.dialog.HasDialogs() {
		u, dialogCmd := a.dialog.Update(msg)
		a.dialog = u.(dialogs.DialogCmp)
		cmds = append(cmds, dialogCmd)
	}
	cmds = append(cmds, cmd)
	return a, tea.Batch(cmds...)
}

// handleWindowResize processes window resize events and updates all components.
func (a *appModel) handleWindowResize(width, height int) tea.Cmd {
	var cmds []tea.Cmd
	if a.showingFullHelp {
		height -= 5
	} else {
		height -= 2
	}
	a.width, a.height = width, height
	// Update status bar
	s, cmd := a.status.Update(tea.WindowSizeMsg{Width: width, Height: height})
	a.status = s.(status.StatusCmp)
	cmds = append(cmds, cmd)

	// Update the current page
	for p, page := range a.pages {
		updated, pageCmd := page.Update(tea.WindowSizeMsg{Width: width, Height: height})
		a.pages[p] = updated.(util.Model)
		cmds = append(cmds, pageCmd)
	}

	// Update the dialogs
	dialog, cmd := a.dialog.Update(tea.WindowSizeMsg{Width: width, Height: height})
	a.dialog = dialog.(dialogs.DialogCmp)
	cmds = append(cmds, cmd)

	return tea.Batch(cmds...)
}

// handleKeyPressMsg processes keyboard input and routes to appropriate handlers.
func (a *appModel) handleKeyPressMsg(msg tea.KeyPressMsg) tea.Cmd {
	if a.completions.Open() {
		// completions
		keyMap := a.completions.KeyMap()
		switch {
		case key.Matches(msg, keyMap.Up), key.Matches(msg, keyMap.Down),
			key.Matches(msg, keyMap.Select), key.Matches(msg, keyMap.Cancel),
			key.Matches(msg, keyMap.UpInsert), key.Matches(msg, keyMap.DownInsert):
			u, cmd := a.completions.Update(msg)
			a.completions = u.(completions.Completions)
			return cmd
		}
	}
	
	// Check for 'b' key to toggle download modal (only when no dialog is open)
	if msg.String() == "b" && !a.dialog.HasDialogs() && a.downloadManager != nil && a.downloadManager.HasActiveDownloads() {
		// Get the first active download
		downloads := a.downloadManager.GetActiveDownloads()
		if len(downloads) > 0 {
			// Show the download modal for the active download
			return util.CmdHandler(dialogs.OpenDialogMsg{
				Model: backendDlg.NewWithModel(nil, downloads[0].Model, nil),
			})
		}
	}
	
	switch {
	// help
	case key.Matches(msg, a.keyMap.Help):
		a.status.ToggleFullHelp()
		a.showingFullHelp = !a.showingFullHelp
		return a.handleWindowResize(a.wWidth, a.wHeight)
	// dialogs
	case key.Matches(msg, a.keyMap.Quit):
		if a.dialog.ActiveDialogID() == quit.QuitDialogID {
			return tea.Quit
		}
		return util.CmdHandler(dialogs.OpenDialogMsg{
			Model: quit.NewQuitDialog(),
		})

	case key.Matches(msg, a.keyMap.Commands):
		// if the app is not configured show no commands
		if !a.isConfigured {
			return nil
		}
		if a.dialog.ActiveDialogID() == commands.CommandsDialogID {
			return util.CmdHandler(dialogs.CloseDialogMsg{})
		}
		if a.dialog.HasDialogs() {
			return nil
		}
		return util.CmdHandler(dialogs.OpenDialogMsg{
			Model: commands.NewCommandDialog(a.selectedSessionID),
		})
	case key.Matches(msg, a.keyMap.Sessions):
		// if the app is not configured show no sessions
		if !a.isConfigured {
			return nil
		}
		if a.dialog.ActiveDialogID() == sessions.SessionsDialogID {
			return util.CmdHandler(dialogs.CloseDialogMsg{})
		}
		if a.dialog.HasDialogs() && a.dialog.ActiveDialogID() != commands.CommandsDialogID {
			return nil
		}
		var cmds []tea.Cmd
		if a.dialog.ActiveDialogID() == commands.CommandsDialogID {
			// If the commands dialog is open, close it first
			cmds = append(cmds, util.CmdHandler(dialogs.CloseDialogMsg{}))
		}
		cmds = append(cmds,
			func() tea.Msg {
				allSessions, _ := a.app.Sessions.List(context.Background())
				return dialogs.OpenDialogMsg{
					Model: sessions.NewSessionDialogCmp(allSessions, a.selectedSessionID),
				}
			},
		)
		return tea.Sequence(cmds...)
	case key.Matches(msg, a.keyMap.Suspend):
		if a.app.CoderAgent != nil && a.app.CoderAgent.IsBusy() {
			return util.ReportWarn("Agent is busy, please wait...")
		}
		return tea.Suspend
	default:
		if a.dialog.HasDialogs() {
			u, dialogCmd := a.dialog.Update(msg)
			a.dialog = u.(dialogs.DialogCmp)
			return dialogCmd
		} else {
			updated, cmd := a.pages[a.currentPage].Update(msg)
			a.pages[a.currentPage] = updated.(util.Model)
			return cmd
		}
	}
}

// moveToPage handles navigation between different pages in the application.
func (a *appModel) moveToPage(pageID page.PageID) tea.Cmd {
	if a.app.CoderAgent.IsBusy() {
		// TODO: maybe remove this :  For now we don't move to any page if the agent is busy
		return util.ReportWarn("Agent is busy, please wait...")
	}

	var cmds []tea.Cmd
	if _, ok := a.loadedPages[pageID]; !ok {
		cmd := a.pages[pageID].Init()
		cmds = append(cmds, cmd)
		a.loadedPages[pageID] = true
	}
	a.previousPage = a.currentPage
	a.currentPage = pageID
	if sizable, ok := a.pages[a.currentPage].(layout.Sizeable); ok {
		cmd := sizable.SetSize(a.width, a.height)
		cmds = append(cmds, cmd)
	}

	return tea.Batch(cmds...)
}

// View renders the complete application interface including pages, dialogs, and overlays.
func (a *appModel) View() tea.View {
	var view tea.View
	t := styles.CurrentTheme()
	view.BackgroundColor = t.BgBase
	
	// Don't render anything until we have proper dimensions
	if a.wWidth == 0 || a.wHeight == 0 {
		// Return empty view until we get window size
		view.Layer = lipgloss.NewCanvas()
		return view
	}
	
	// Show loading screen if loading
	if a.isLoading && a.loadingScreen != nil {
		// Handle both types of loading screens
		var loadingView string
		switch ls := a.loadingScreen.(type) {
		case *loading.LoadingScreen:
			loadingView = ls.View()
		case *loading.SimpleLoadingScreen:
			loadingView = ls.View()
		}
		
		layers := []*lipgloss.Layer{
			lipgloss.NewLayer(loadingView),
		}
		
		// Add dialog layers if active (even during loading)
		if a.dialog != nil && a.dialog.HasDialogs() {
			layers = append(layers, a.dialog.GetLayers()...)
		}
		
		view.Layer = lipgloss.NewCanvas(layers...)
		return view
	}
	
	if a.wWidth < 25 || a.wHeight < 15 {
		view.Layer = lipgloss.NewCanvas(
			lipgloss.NewLayer(
				t.S().Base.Width(a.wWidth).Height(a.wHeight).
					Align(lipgloss.Center, lipgloss.Center).
					Render(
						t.S().Base.
							Padding(1, 4).
							Foreground(t.White).
							BorderStyle(lipgloss.RoundedBorder()).
							BorderForeground(t.Primary).
							Render("Window too small!"),
					),
			),
		)
		return view
	}

	page := a.pages[a.currentPage]
	if withHelp, ok := page.(core.KeyMapHelp); ok {
		a.status.SetKeyMap(withHelp.Help())
	}
	pageView := page.View()
	components := []string{
		pageView,
	}
	// Add download status if there are active downloads
	if a.downloadManager != nil && a.downloadManager.HasActiveDownloads() {
		downloadStatus := a.renderDownloadStatus()
		if downloadStatus != "" {
			components = append(components, downloadStatus)
		}
	}
	
	components = append(components, a.status.View())

	appView := lipgloss.JoinVertical(lipgloss.Top, components...)
	layers := []*lipgloss.Layer{
		lipgloss.NewLayer(appView),
	}
	if a.dialog.HasDialogs() {
		layers = append(
			layers,
			a.dialog.GetLayers()...,
		)
	}

	var cursor *tea.Cursor
	if v, ok := page.(util.Cursor); ok {
		cursor = v.Cursor()
		// Hide the cursor if it's positioned outside the textarea
		statusHeight := a.height - strings.Count(pageView, "\n") + 1
		if cursor != nil && cursor.Y+statusHeight+chat.EditorHeight-2 <= a.height { // 2 for the top and bottom app padding
			cursor = nil
		}
	}
	activeView := a.dialog.ActiveModel()
	if activeView != nil {
		cursor = nil // Reset cursor if a dialog is active unless it implements util.Cursor
		if v, ok := activeView.(util.Cursor); ok {
			cursor = v.Cursor()
		}
	}

	if a.completions.Open() && cursor != nil {
		cmp := a.completions.View()
		x, y := a.completions.Position()
		layers = append(
			layers,
			lipgloss.NewLayer(cmp).X(x).Y(y),
		)
	}

	canvas := lipgloss.NewCanvas(
		layers...,
	)

	view.Layer = canvas
	view.Cursor = cursor
	return view
}

// renderDownloadStatus renders the background download status bar
func (a *appModel) renderDownloadStatus() string {
	if a.downloadManager == nil {
		return ""
	}
	
	status := a.downloadManager.GetStatusString()
	if status == "" {
		return ""
	}
	
	t := styles.CurrentTheme()
	return t.S().Base.
		Background(t.BgSubtle).
		Foreground(t.FgMuted).
		Padding(0, 1).
		Width(a.width).
		Render(status)
}

// New creates and initializes a new TUI application model.
func New(app *app.App) tea.Model {
	return NewWithSize(app, 80, 24) // Default size
}

// NewWithSize creates and initializes a new TUI application model with initial size.
func NewWithSize(app *app.App, width, height int) tea.Model {
	chatPage := chat.New(app)
	keyMap := DefaultKeyMap()
	keyMap.pageBindings = chatPage.Bindings()

	// Initialize dialog component
	dialogCmp := dialogs.NewDialogCmp()

	// Create loading screen with initial size
	loadingScreen := loading.NewSimple()
	// Directly set the dimensions
	loadingScreen.SetSize(width, height)

	model := &appModel{
		currentPage: chat.ChatPageID,
		app:         app,
		status:      status.NewStatusCmp(),
		loadedPages: make(map[page.PageID]bool),
		keyMap:      keyMap,

		pages: map[page.PageID]util.Model{
			chat.ChatPageID: chatPage,
		},

		dialog:      dialogCmp,
		completions: completions.New(),
		
		// Start with loading screen
		isLoading:     true,
		loadingScreen: loadingScreen,
		
		// Initialize with provided dimensions
		wWidth: width,
		wHeight: height,
		width: width,
		height: height - 2, // Account for status bar
		
		// Background downloads
		downloadManager: background.NewDownloadManager(),
	}

	return model
}

// ShellShortcutCheckMsg is sent when shell shortcut check is complete
type ShellShortcutCheckMsg struct {
	NeedsSetup bool
}

// checkShellShortcuts checks if shell shortcuts are installed
func (a *appModel) checkShellShortcuts() tea.Cmd {
	return func() tea.Msg {
		// Check if shortcuts already exist
		hasShortcut, err := shell.HasShortcut()
		if err != nil {
			// Silently ignore errors
			return nil
		}
		
		// If shortcuts don't exist, return message to show dialog
		if !hasShortcut {
			return ShellShortcutCheckMsg{NeedsSetup: true}
		}
		
		return nil
	}
}

// startInitialization starts the initialization process
func (a *appModel) startInitialization() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		// Brief loading screen to ensure smooth initialization
		return InitializationCompleteMsg{}
	})
}


// findNgrokPath attempts to find ngrok in common locations
func findNgrokPath() string {
	// Check common locations
	paths := []string{
		"./ngrok",
		"./build/ngrok",
		"ngrok", // in PATH
	}
	
	for _, path := range paths {
		if _, err := exec.LookPath(path); err == nil {
			return path
		}
	}
	
	return ""
}

// startWebShare starts the web sharing server for the session
func (a *appModel) startWebShare(sessionID string) tea.Cmd {
	return func() tea.Msg {
		fmt.Fprintf(os.Stderr, "[DEBUG] startWebShare: Starting web share for session %s\n", sessionID)
		
		// Stop any existing web share
		if a.webShare != nil {
			fmt.Fprintf(os.Stderr, "[DEBUG] startWebShare: Stopping existing web share\n")
			a.webShare.Stop()
			// Notify sidebar to clear URLs
			defer func() {
				go func() {
					time.Sleep(100 * time.Millisecond) // Small delay to ensure cleanup
					// This would need to be sent through a channel or program command
				}()
			}()
		}
		
		// Create new web share
		fmt.Fprintf(os.Stderr, "[DEBUG] startWebShare: Creating new SessionShare\n")
		a.webShare = webshare.NewSessionShare(sessionID)
		
		fmt.Fprintf(os.Stderr, "[DEBUG] startWebShare: Calling webShare.Start()\n")
		urls, err := a.webShare.Start()
		fmt.Fprintf(os.Stderr, "[DEBUG] startWebShare: webShare.Start() returned, err=%v\n", err)
		
		if err != nil {
			// Check if the error is about ngrok authentication
			if strings.Contains(err.Error(), "ngrok authentication required") {
				// Find ngrok path to pass to auth dialog
				ngrokPath := a.webShare.GetNgrokPath()
				if ngrokPath == "" {
					// Try to find it ourselves
					ngrokPath = findNgrokPath()
				}
				
				// Show ngrok auth dialog
				return dialogs.OpenDialogMsg{
					Model: ngrokauth.NewNgrokAuthDialog(ngrokPath, sessionID),
				}
			}
			
			return util.InfoMsg{
				Type: util.InfoTypeError,
				Msg:  fmt.Sprintf("Failed to start web share: %v", err),
				TTL:  5 * time.Second,
			}
		}
		
		// Start a goroutine to periodically broadcast updates
		// Use the webShare's context to properly stop the goroutine
		fmt.Fprintf(os.Stderr, "[DEBUG] startWebShare: Starting broadcast loop\n")
		if a.webShare != nil {
			go a.webShare.StartBroadcastLoop()
		}
		
		// Start SSE client in a goroutine (non-blocking)
		if urls != nil && urls.LocalURL != "" {
			fmt.Fprintf(os.Stderr, "[DEBUG] startWebShare: Starting SSE client for %s\n", urls.LocalURL)
			// Note: We need the tea.Program instance to send messages back
			// For now, we'll start it without program and rely on polling
			sseClient := webshare.NewSSEClient(urls.LocalURL, nil)
			sseClient.Start()
		}
		
		fmt.Fprintf(os.Stderr, "[DEBUG] startWebShare: Creating batch commands\n")
		// Return both the dialog and a message to update the sidebar
		return tea.Batch(
			func() tea.Msg {
				fmt.Fprintf(os.Stderr, "[DEBUG] startWebShare: Returning OpenDialogMsg\n")
				return dialogs.OpenDialogMsg{
					Model: webshareDialog.NewShareDialog(sessionID, urls),
				}
			},
			func() tea.Msg {
				fmt.Fprintf(os.Stderr, "[DEBUG] startWebShare: Returning WebShareStartedMsg\n")
				localURL := ""
				ngrokURL := ""
				if urls != nil {
					localURL = urls.LocalURL
					ngrokURL = urls.NgrokURL
				}
				return commands.WebShareStartedMsg{
					LocalURL: localURL,
					NgrokURL: ngrokURL,
					WebShare: a.webShare,
				}
			},
		)()
	}
}
