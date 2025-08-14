package models

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
	"github.com/google/uuid"
	"log/slog"
)

const (
	GitHubOAuthDialogID dialogs.DialogID = "github-oauth"
	githubClientID      string          = "YOUR_GITHUB_OAUTH_APP_CLIENT_ID" // TODO: Replace with actual client ID
	githubClientSecret  string          = "YOUR_GITHUB_OAUTH_APP_CLIENT_SECRET" // TODO: Replace with actual secret
)

type GitHubOAuthDialog struct {
	width      int
	height     int
	state      string
	server     *http.Server
	authCode   string
	authError  error
	stateToken string
	redirectURI string
}

type GitHubAuthSuccessMsg struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

type GitHubAuthProgressMsg struct {
	State string
}

func NewGitHubOAuthDialog() *GitHubOAuthDialog {
	return &GitHubOAuthDialog{
		state:      "initializing",
		stateToken: uuid.NewString(),
	}
}

func (d *GitHubOAuthDialog) Init() tea.Cmd {
	return d.startOAuthServer()
}

func (d *GitHubOAuthDialog) startOAuthServer() tea.Cmd {
	return func() tea.Msg {
		// Find an available port
		listener, err := net.Listen("tcp", "localhost:0")
		if err != nil {
			d.authError = fmt.Errorf("failed to start OAuth server: %w", err)
			return GitHubAuthProgressMsg{State: "error"}
		}
		
		port := listener.Addr().(*net.TCPAddr).Port
		d.redirectURI = fmt.Sprintf("http://localhost:%d/callback", port)
		
		// Set up HTTP server
		mux := http.NewServeMux()
		mux.HandleFunc("/callback", d.handleCallback)
		
		d.server = &http.Server{
			Handler: mux,
		}
		
		// Start server in background
		go func() {
			if err := d.server.Serve(listener); err != nil && err != http.ErrServerClosed {
				slog.Error("OAuth server error", "error", err)
			}
		}()
		
		// Open browser with GitHub OAuth URL
		authURL := d.buildAuthURL()
		if err := openBrowser(authURL); err != nil {
			d.authError = fmt.Errorf("failed to open browser: %w", err)
			return GitHubAuthProgressMsg{State: "error"}
		}
		
		return GitHubAuthProgressMsg{State: "waiting"}
	}
}

func (d *GitHubOAuthDialog) buildAuthURL() string {
	params := url.Values{}
	params.Set("client_id", githubClientID)
	params.Set("redirect_uri", d.redirectURI)
	params.Set("scope", "copilot")
	params.Set("state", d.stateToken)
	
	return fmt.Sprintf("https://github.com/login/oauth/authorize?%s", params.Encode())
}

func (d *GitHubOAuthDialog) handleCallback(w http.ResponseWriter, r *http.Request) {
	// Check state token
	state := r.URL.Query().Get("state")
	if state != d.stateToken {
		d.authError = fmt.Errorf("invalid state token")
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}
	
	// Get authorization code
	code := r.URL.Query().Get("code")
	if code == "" {
		errorMsg := r.URL.Query().Get("error")
		errorDesc := r.URL.Query().Get("error_description")
		d.authError = fmt.Errorf("authorization failed: %s - %s", errorMsg, errorDesc)
		http.Error(w, "Authorization failed", http.StatusBadRequest)
		return
	}
	
	d.authCode = code
	
	// Show success page
	successHTML := `
<!DOCTYPE html>
<html>
<head>
    <title>Authorization Successful</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            background-color: #f6f8fa;
        }
        .container {
            text-align: center;
            padding: 2rem;
            background: white;
            border-radius: 8px;
            box-shadow: 0 1px 3px rgba(0,0,0,0.12);
        }
        .checkmark {
            width: 64px;
            height: 64px;
            margin: 0 auto 1rem;
            background: #28a745;
            border-radius: 50%;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .checkmark svg {
            width: 32px;
            height: 32px;
            fill: white;
        }
        h1 {
            color: #24292e;
            margin: 0 0 0.5rem;
        }
        p {
            color: #586069;
            margin: 0;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="checkmark">
            <svg viewBox="0 0 16 16">
                <path d="M13.78 4.22a.75.75 0 010 1.06l-7.25 7.25a.75.75 0 01-1.06 0L2.22 9.28a.75.75 0 111.06-1.06L6 10.94l6.72-6.72a.75.75 0 011.06 0z"/>
            </svg>
        </div>
        <h1>Authorization Successful!</h1>
        <p>You can now close this window and return to Toke.</p>
    </div>
</body>
</html>`
	
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(successHTML))
	
	// Close the server after a short delay
	go func() {
		time.Sleep(1 * time.Second)
		d.server.Close()
	}()
}

func (d *GitHubOAuthDialog) exchangeCodeForToken() tea.Cmd {
	return func() tea.Msg {
		// Exchange code for access token
		tokenURL := "https://github.com/login/oauth/access_token"
		
		data := url.Values{}
		data.Set("client_id", githubClientID)
		data.Set("client_secret", githubClientSecret)
		data.Set("code", d.authCode)
		data.Set("redirect_uri", d.redirectURI)
		
		req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
		if err != nil {
			d.authError = err
			return GitHubAuthProgressMsg{State: "error"}
		}
		
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		
		req = req.WithContext(ctx)
		
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			d.authError = err
			return GitHubAuthProgressMsg{State: "error"}
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			d.authError = fmt.Errorf("token exchange failed with status: %s", resp.Status)
			return GitHubAuthProgressMsg{State: "error"}
		}
		
		var tokenResp struct {
			AccessToken  string `json:"access_token"`
			TokenType    string `json:"token_type"`
			Scope        string `json:"scope"`
			RefreshToken string `json:"refresh_token,omitempty"`
			ExpiresIn    int    `json:"expires_in,omitempty"`
			Error        string `json:"error,omitempty"`
			ErrorDesc    string `json:"error_description,omitempty"`
		}
		
		if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
			d.authError = err
			return GitHubAuthProgressMsg{State: "error"}
		}
		
		if tokenResp.Error != "" {
			d.authError = fmt.Errorf("%s: %s", tokenResp.Error, tokenResp.ErrorDesc)
			return GitHubAuthProgressMsg{State: "error"}
		}
		
		// Verify we got copilot scope
		if !strings.Contains(tokenResp.Scope, "copilot") {
			d.authError = fmt.Errorf("copilot scope not granted")
			return GitHubAuthProgressMsg{State: "error"}
		}
		
		// Success!
		return GitHubAuthSuccessMsg{
			AccessToken:  tokenResp.AccessToken,
			RefreshToken: tokenResp.RefreshToken,
			ExpiresIn:    tokenResp.ExpiresIn,
		}
	}
}

func (d *GitHubOAuthDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			if d.server != nil {
				d.server.Close()
			}
			return d, util.CmdHandler(dialogs.CloseDialogMsg{})
		}
		
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
		return d, nil
		
	case GitHubAuthProgressMsg:
		d.state = msg.State
		if msg.State == "error" {
			return d, nil
		}
		return d, nil
		
	case GitHubAuthSuccessMsg:
		// Format the token for storage
		tokenData := msg.AccessToken
		if msg.RefreshToken != "" && msg.ExpiresIn > 0 {
			expiry := time.Now().Add(time.Duration(msg.ExpiresIn) * time.Second)
			tokenData = fmt.Sprintf("%s|%s|%s", msg.AccessToken, msg.RefreshToken, expiry.Format(time.RFC3339))
		}
		
		// Close dialog and return token
		return d, tea.Batch(
			util.CmdHandler(dialogs.CloseDialogMsg{}),
			util.CmdHandler(APIKeyVerifiedMsg{
				ProviderID: "github_copilot",
				APIKey:     tokenData,
			}),
		)
	}
	
	// Check if we have an auth code and haven't exchanged it yet
	if d.authCode != "" && d.state == "waiting" {
		d.state = "exchanging"
		return d, d.exchangeCodeForToken()
	}
	
	return d, nil
}

func (d *GitHubOAuthDialog) View() string {
	if d.width == 0 || d.height == 0 {
		return ""
	}
	
	t := styles.CurrentTheme()
	
	var content strings.Builder
	
	// Title
	title := t.S().Base.Bold(true).Foreground(t.Primary).Render("🔐 GitHub Copilot Authentication")
	content.WriteString(lipgloss.NewStyle().Width(d.width).Align(lipgloss.Center).Render(title))
	content.WriteString("\n\n")
	
	// Status
	switch d.state {
	case "initializing":
		content.WriteString(t.S().Base.Render("Starting authentication server..."))
		
	case "waiting":
		content.WriteString(t.S().Base.Bold(true).Render("Waiting for authorization..."))
		content.WriteString("\n\n")
		content.WriteString(t.S().Base.Foreground(t.FgMuted).Render("A browser window should have opened."))
		content.WriteString("\n")
		content.WriteString(t.S().Base.Foreground(t.FgMuted).Render("Please authorize Toke to access GitHub Copilot."))
		content.WriteString("\n\n")
		content.WriteString(t.S().Base.Foreground(t.FgHalfMuted).Render("If the browser didn't open, please visit:"))
		content.WriteString("\n")
		content.WriteString(t.S().Base.Foreground(t.Primary).Render(d.buildAuthURL()))
		
	case "exchanging":
		content.WriteString(t.S().Base.Bold(true).Render("Exchanging authorization code..."))
		
	case "error":
		content.WriteString(t.S().Base.Foreground(t.Error).Render("❌ Authentication failed"))
		content.WriteString("\n\n")
		if d.authError != nil {
			content.WriteString(t.S().Base.Foreground(t.FgMuted).Render(d.authError.Error()))
		}
	}
	
	content.WriteString("\n\n")
	
	// Footer
	footer := t.S().Base.Foreground(t.FgHalfMuted).Render("Press Esc to cancel")
	content.WriteString(footer)
	
	// Dialog styling
	dialogStyle := t.S().Base.
		Width(70).
		Padding(2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border)
	
	dialog := dialogStyle.Render(content.String())
	
	// Center in viewport
	return lipgloss.Place(
		d.width,
		d.height,
		lipgloss.Center,
		lipgloss.Center,
		dialog,
	)
}

func (d *GitHubOAuthDialog) SetSize(width, height int) tea.Cmd {
	d.width = width
	d.height = height
	return nil
}

func (d *GitHubOAuthDialog) ID() dialogs.DialogID {
	return GitHubOAuthDialogID
}

func (d *GitHubOAuthDialog) Position() (int, int) {
	return 0, 0
}

// openBrowser opens the URL in the default browser
func openBrowser(url string) error {
	var cmd string
	var args []string
	
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default: // Linux and others
		cmd = "xdg-open"
		args = []string{url}
	}
	
	return exec.Command(cmd, args...).Start()
}
