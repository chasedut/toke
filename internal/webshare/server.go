package webshare

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chasedut/toke/internal/config"
	"github.com/chasedut/toke/internal/db"
)

type SessionShare struct {
	server       *http.Server
	port         int
	localURL     string
	ngrokURL     string
	sessionID    string
	ctx          context.Context
	cancel       context.CancelFunc
	ngrokProcess *exec.Cmd
	sseClients   map[chan string]bool
}

type ShareURLs struct {
	LocalURL string
	NgrokURL string
}

type PageData struct {
	SessionID   string
	SessionTitle string
	Model       string
	Messages    []MessageData
	LocalURL    string
	NgrokURL    string
}

type MessageData struct {
	ID        string
	Role      string
	Content   template.HTML
	Timestamp string
}

func NewSessionShare(sessionID string) *SessionShare {
	return &SessionShare{
		sessionID:  sessionID,
		sseClients: make(map[chan string]bool),
	}
}

func (s *SessionShare) Start() (*ShareURLs, error) {
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel

	// Find an available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("failed to find available port: %w", err)
	}
	s.port = listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Set up HTTP endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleHome)
	mux.HandleFunc("/api/messages", s.handleMessagesAPI)
	mux.HandleFunc("/events", s.handleSSE)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	// Start the server
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server error: %v\n", err)
		}
	}()

	// Get local URL
	s.localURL = fmt.Sprintf("http://localhost:%d", s.port)

	// Start ngrok
	if err := s.startNgrok(); err != nil {
		// Non-fatal - continue without ngrok
		fmt.Printf("Warning: Failed to start ngrok: %v\n", err)
	}

	return &ShareURLs{
		LocalURL: s.localURL,
		NgrokURL: s.ngrokURL,
	}, nil
}

func (s *SessionShare) handleHome(w http.ResponseWriter, r *http.Request) {
	// Get session from database
	session, err := db.GetSession(s.sessionID)
	if err != nil {
		http.Error(w, "Failed to get session", http.StatusInternalServerError)
		return
	}

	// Get messages from database
	messages, err := db.GetSessionMessages(s.sessionID)
	if err != nil {
		http.Error(w, "Failed to get messages", http.StatusInternalServerError)
		return
	}

	// Get model info
	cfg := config.Get()
	modelName := "unknown"
	if agentCfg, ok := cfg.Agents["coder"]; ok {
		if model := cfg.GetModelByType(agentCfg.Model); model != nil {
			modelName = model.Name
		}
	}

	// Convert messages for template - extract text for initial display
	messageData := make([]MessageData, len(messages))
	for i, msg := range messages {
		timestamp := time.Unix(msg.CreatedAt, 0).Format("15:04:05")
		
		// Extract text content from JSON parts for initial display
		textContent := extractTextFromJSONParts(msg.Parts)
		
		messageData[i] = MessageData{
			ID:        msg.ID,
			Role:      msg.Role,
			Content:   template.HTML(textContent), // Display text, not JSON
			Timestamp: timestamp,
		}
	}

	pageData := PageData{
		SessionID:    session.ID,
		SessionTitle: session.Title,
		Model:        modelName,
		Messages:     messageData,
		LocalURL:     s.localURL,
		NgrokURL:     s.ngrokURL,
	}

	tmpl := template.Must(template.New("index").Parse(htmlTemplate))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, pageData)
}

func (s *SessionShare) handleMessagesAPI(w http.ResponseWriter, r *http.Request) {
	// Get messages from database
	messages, err := db.GetSessionMessages(s.sessionID)
	if err != nil {
		http.Error(w, "Failed to get messages", http.StatusInternalServerError)
		return
	}

	// Convert to API format - send raw data for client-side rendering
	apiMessages := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		timestamp := time.Unix(msg.CreatedAt, 0).Format("15:04:05")
		apiMsg := map[string]interface{}{
			"id":        msg.ID,
			"role":      msg.Role,
			"parts":     msg.Parts, // Raw JSON parts data
			"timestamp": timestamp,
		}
		apiMessages = append(apiMessages, apiMsg)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiMessages)
}

func (s *SessionShare) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create a channel for this client
	clientChan := make(chan string)
	s.sseClients[clientChan] = true
	defer func() {
		delete(s.sseClients, clientChan)
		close(clientChan)
	}()

	// Send initial connection message
	fmt.Fprintf(w, "data: {\"type\":\"connected\"}\n\n")
	w.(http.Flusher).Flush()

	// Keep connection alive
	for {
		select {
		case msg := <-clientChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			return
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *SessionShare) BroadcastUpdate() {
	// Get latest messages
	messages, err := db.GetSessionMessages(s.sessionID)
	if err != nil {
		return
	}

	// Convert to JSON - send raw data for client-side rendering
	apiMessages := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		timestamp := time.Unix(msg.CreatedAt, 0).Format("15:04:05")
		apiMsg := map[string]interface{}{
			"id":        msg.ID,
			"role":      msg.Role,
			"parts":     msg.Parts, // Raw JSON parts data
			"timestamp": timestamp,
		}
		apiMessages = append(apiMessages, apiMsg)
	}

	data, _ := json.Marshal(map[string]interface{}{
		"type":     "messages",
		"messages": apiMessages,
	})

	// Send to all SSE clients
	for client := range s.sseClients {
		select {
		case client <- string(data):
		default:
			// Client buffer full, skip
		}
	}
}

func (s *SessionShare) startNgrok() error {
	// Look for ngrok in multiple locations
	ngrokPath := s.findNgrok()
	if ngrokPath == "" {
		return fmt.Errorf("ngrok not found. Please install ngrok or rebuild with --all flag")
	}

	// Check if ngrok is authenticated
	if !s.isNgrokAuthenticated(ngrokPath) {
		// Check environment variable first
		authToken := os.Getenv("NGROK_AUTHTOKEN")
		if authToken != "" {
			// Configure ngrok with the token
			configCmd := exec.Command(ngrokPath, "config", "add-authtoken", authToken)
			if err := configCmd.Run(); err != nil {
				return fmt.Errorf("failed to configure ngrok auth: %w", err)
			}
		} else {
			// Return error indicating auth is needed
			return fmt.Errorf("ngrok authentication required. Please set NGROK_AUTHTOKEN environment variable or run: ngrok config add-authtoken <token>")
		}
	}

	// Start ngrok
	s.ngrokProcess = exec.CommandContext(s.ctx, ngrokPath, "http", fmt.Sprintf("%d", s.port))
	if err := s.ngrokProcess.Start(); err != nil {
		return fmt.Errorf("failed to start ngrok: %w", err)
	}

	// Wait a moment for ngrok to start
	time.Sleep(2 * time.Second)

	// Get ngrok URL from API
	resp, err := http.Get("http://localhost:4040/api/tunnels")
	if err != nil {
		return fmt.Errorf("failed to get ngrok tunnels: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read ngrok response: %w", err)
	}

	var tunnels struct {
		Tunnels []struct {
			PublicURL string `json:"public_url"`
			Proto     string `json:"proto"`
		} `json:"tunnels"`
	}

	if err := json.Unmarshal(body, &tunnels); err != nil {
		return fmt.Errorf("failed to parse ngrok response: %w", err)
	}

	// Find HTTPS tunnel
	for _, tunnel := range tunnels.Tunnels {
		if tunnel.Proto == "https" {
			s.ngrokURL = tunnel.PublicURL
			break
		}
	}

	if s.ngrokURL == "" && len(tunnels.Tunnels) > 0 {
		s.ngrokURL = tunnels.Tunnels[0].PublicURL
	}

	return nil
}

func (s *SessionShare) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}

	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
	}

	if s.ngrokProcess != nil && s.ngrokProcess.Process != nil {
		if runtime.GOOS == "windows" {
			s.ngrokProcess.Process.Kill()
		} else {
			s.ngrokProcess.Process.Signal(os.Interrupt)
		}
		s.ngrokProcess.Wait()
	}

	return nil
}

func (s *SessionShare) GetURLs() *ShareURLs {
	return &ShareURLs{
		LocalURL: s.localURL,
		NgrokURL: s.ngrokURL,
	}
}

// GetNgrokPath returns the path to ngrok if found
func (s *SessionShare) GetNgrokPath() string {
	return s.findNgrok()
}

// isNgrokAuthenticated checks if ngrok has an auth token configured
func (s *SessionShare) isNgrokAuthenticated(ngrokPath string) bool {
	// First check environment variable
	if os.Getenv("NGROK_AUTHTOKEN") != "" {
		return true
	}
	
	// Try to check ngrok config
	// ngrok v3 uses "ngrok config check" to verify configuration
	cmd := exec.Command(ngrokPath, "config", "check")
	output, err := cmd.CombinedOutput()
	
	// If command succeeded, check output
	if err == nil {
		outputStr := string(output)
		// Valid config will contain "Valid configuration"
		if strings.Contains(outputStr, "Valid") {
			return true
		}
	}
	
	// Try alternative method: check if config file exists and has authtoken
	// This works for ngrok v2 and v3
	configCheckCmd := exec.Command(ngrokPath, "authtoken", "--log=false")
	if err := configCheckCmd.Run(); err != nil {
		// If this fails with specific error about missing token, not authenticated
		if strings.Contains(err.Error(), "authtoken") {
			return false
		}
	}
	
	// Default to false if we can't determine
	return false
}

// findNgrok looks for ngrok in various locations
func (s *SessionShare) findNgrok() string {
	// Get the directory where the executable is located
	exePath, err := os.Executable()
	if err != nil {
		exePath = ""
	}
	exeDir := filepath.Dir(exePath)
	
	// Check locations in order of preference
	locations := []string{}
	
	if runtime.GOOS == "windows" {
		locations = []string{
			// Same directory as executable
			filepath.Join(exeDir, "ngrok.exe"),
			// Build directory relative to exe
			filepath.Join(exeDir, "build", "ngrok.exe"),
			// Current working directory
			"./ngrok.exe",
			"./build/ngrok.exe",
			// System PATH
			"ngrok.exe",
		}
	} else {
		locations = []string{
			// Same directory as executable
			filepath.Join(exeDir, "ngrok"),
			// Build directory relative to exe
			filepath.Join(exeDir, "build", "ngrok"),
			// Current working directory
			"./ngrok",
			"./build/ngrok",
			// System PATH
			"ngrok",
		}
	}
	
	for _, loc := range locations {
		if loc == "ngrok" || loc == "ngrok.exe" {
			// Check system PATH
			if path, err := exec.LookPath(loc); err == nil {
				return path
			}
		} else {
			// Check file existence
			if _, err := os.Stat(loc); err == nil {
				absPath, _ := filepath.Abs(loc)
				return absPath
			}
		}
	}
	
	return ""
}

// extractTextFromJSONParts extracts plain text from JSON message parts
func extractTextFromJSONParts(partsJSON string) string {
	var parts []map[string]interface{}
	if err := json.Unmarshal([]byte(partsJSON), &parts); err == nil {
		var textParts []string
		for _, part := range parts {
			if partType, ok := part["type"].(string); ok {
				switch partType {
				case "text":
					if text, ok := part["text"].(string); ok {
						textParts = append(textParts, text)
					}
				case "data":
					if data, ok := part["data"].(map[string]interface{}); ok {
						if text, ok := data["text"].(string); ok {
							textParts = append(textParts, text)
						}
					}
				}
			} else if partData, ok := part["data"].(map[string]interface{}); ok {
				// Handle the structure from your screenshot
				if text, ok := partData["text"].(string); ok {
					textParts = append(textParts, text)
				}
			}
		}
		if len(textParts) > 0 {
			return textParts[0] // Return first text part for initial display
		}
	}
	
	// Try as single object
	var singlePart map[string]interface{}
	if err := json.Unmarshal([]byte(partsJSON), &singlePart); err == nil {
		if data, ok := singlePart["data"].(map[string]interface{}); ok {
			if text, ok := data["text"].(string); ok {
				return text
			}
		}
		if partType, ok := singlePart["type"].(string); ok && partType == "text" {
			if text, ok := singlePart["text"].(string); ok {
				return text
			}
		}
	}
	
	return partsJSON // Fallback to raw content
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.SessionTitle}} • Toke</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        :root {
            --background-primary: #0e0e0e;
            --background-secondary: #1a1a1a;
            --background-tertiary: #242424;
            --background-hover: #2a2a2a;
            --text-primary: #ececec;
            --text-secondary: #a0a0a0;
            --text-tertiary: #707070;
            --accent-primary: #7c3aed;
            --accent-secondary: #a78bfa;
            --accent-gradient: linear-gradient(135deg, #7c3aed 0%, #a78bfa 100%);
            --border-color: #2e2e2e;
            --border-hover: #3e3e3e;
            --user-bg: linear-gradient(135deg, #2e1065 0%, #1e1b4b 100%);
            --assistant-bg: transparent;
            --code-bg: #0a0a0a;
            --success: #10b981;
            --warning: #f59e0b;
            --error: #ef4444;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Helvetica', 'Arial', sans-serif;
            background: var(--background-primary);
            color: var(--text-primary);
            line-height: 1.6;
            font-size: 14px;
            height: 100vh;
            overflow: hidden;
            display: flex;
            flex-direction: column;
        }
        
        .app-container {
            display: flex;
            height: 100vh;
            overflow: hidden;
        }
        
        /* Sidebar Styles */
        .sidebar {
            width: 280px;
            background: var(--background-secondary);
            border-right: 1px solid var(--border-color);
            display: flex;
            flex-direction: column;
            flex-shrink: 0;
        }
        
        .sidebar-header {
            padding: 1.5rem;
            border-bottom: 1px solid var(--border-color);
        }
        
        .sidebar-logo {
            font-size: 1.5rem;
            font-weight: 700;
            background: var(--accent-gradient);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
            letter-spacing: -0.02em;
            margin-bottom: 0.5rem;
        }
        
        .sidebar-session {
            font-size: 0.875rem;
            color: var(--text-secondary);
        }
        
        .invite-section {
            padding: 1.5rem;
            border-bottom: 1px solid var(--border-color);
        }
        
        .invite-title {
            font-size: 0.875rem;
            font-weight: 600;
            color: var(--text-primary);
            margin-bottom: 1rem;
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }
        
        .invite-icon {
            width: 16px;
            height: 16px;
            opacity: 0.7;
        }
        
        .invite-url {
            margin-bottom: 1rem;
        }
        
        .invite-label {
            font-size: 0.75rem;
            color: var(--text-tertiary);
            margin-bottom: 0.25rem;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }
        
        .invite-input-group {
            display: flex;
            gap: 0.5rem;
        }
        
        .invite-input {
            flex: 1;
            background: var(--background-tertiary);
            border: 1px solid var(--border-color);
            border-radius: 6px;
            padding: 0.5rem 0.75rem;
            font-size: 0.8125rem;
            color: var(--text-primary);
            font-family: 'SF Mono', Monaco, monospace;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }
        
        .invite-input:focus {
            outline: none;
            border-color: var(--accent-primary);
        }
        
        .copy-btn {
            background: var(--background-tertiary);
            border: 1px solid var(--border-color);
            border-radius: 6px;
            padding: 0.5rem 0.75rem;
            font-size: 0.75rem;
            color: var(--text-secondary);
            cursor: pointer;
            transition: all 0.2s ease;
            white-space: nowrap;
        }
        
        .copy-btn:hover {
            background: var(--background-hover);
            border-color: var(--border-hover);
            color: var(--text-primary);
        }
        
        .copy-btn.copied {
            background: var(--success);
            border-color: var(--success);
            color: white;
        }
        
        .sidebar-content {
            flex: 1;
            overflow-y: auto;
            padding: 1rem;
        }
        
        .main-content {
            flex: 1;
            display: flex;
            flex-direction: column;
            overflow: hidden;
        }
        
        header {
            background: var(--background-secondary);
            border-bottom: 1px solid var(--border-color);
            padding: 0.75rem 1.5rem;
            flex-shrink: 0;
        }
        
        .header-content {
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        
        .header-title {
            font-size: 0.875rem;
            color: var(--text-secondary);
        }
        
        .model-badge {
            background: var(--background-tertiary);
            padding: 0.25rem 0.75rem;
            border-radius: 9999px;
            font-size: 0.75rem;
            color: var(--text-secondary);
            border: 1px solid var(--border-color);
        }
        
        .status {
            display: flex;
            align-items: center;
            gap: 0.5rem;
            font-size: 0.75rem;
            color: var(--text-tertiary);
            background: var(--background-tertiary);
            padding: 0.25rem 0.75rem;
            border-radius: 9999px;
            border: 1px solid var(--border-color);
        }
        
        .status-dot {
            width: 6px;
            height: 6px;
            border-radius: 50%;
            background: var(--success);
            animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
        }
        
        @keyframes pulse {
            0%, 100% { 
                opacity: 1;
                transform: scale(1);
            }
            50% { 
                opacity: 0.5;
                transform: scale(0.9);
            }
        }
        
        main {
            flex: 1;
            overflow-y: auto;
            overflow-x: hidden;
            scroll-behavior: smooth;
        }
        
        .chat-container {
            max-width: 900px;
            margin: 0 auto;
            padding: 2rem 1rem;
        }
        
        .messages {
            display: flex;
            flex-direction: column;
            gap: 0;
        }
        
        .message-group {
            border-bottom: 1px solid var(--border-color);
            transition: background 0.2s ease;
        }
        
        .message-group:hover {
            background: rgba(255, 255, 255, 0.02);
        }
        
        .message-group:last-child {
            border-bottom: none;
        }
        
        .message {
            padding: 1.5rem 0;
            animation: fadeIn 0.3s ease;
        }
        
        @keyframes fadeIn {
            from {
                opacity: 0;
                transform: translateY(10px);
            }
            to {
                opacity: 1;
                transform: translateY(0);
            }
        }
        
        .message-inner {
            display: flex;
            gap: 1rem;
            align-items: flex-start;
        }
        
        .avatar {
            width: 32px;
            height: 32px;
            border-radius: 8px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-weight: 600;
            font-size: 0.875rem;
            flex-shrink: 0;
            margin-top: 0.125rem;
        }
        
        .message.user .avatar {
            background: var(--accent-gradient);
            color: white;
        }
        
        .message.assistant .avatar {
            background: var(--background-tertiary);
            border: 1px solid var(--border-color);
            color: var(--accent-secondary);
        }
        
        .message.system .avatar,
        .message.tool .avatar {
            background: var(--background-tertiary);
            border: 1px solid var(--border-color);
            color: var(--text-tertiary);
            font-size: 0.75rem;
        }
        
        .message-main {
            flex: 1;
            min-width: 0;
        }
        
        .message-header {
            display: flex;
            align-items: center;
            gap: 0.75rem;
            margin-bottom: 0.5rem;
        }
        
        .message-role {
            font-weight: 600;
            font-size: 0.875rem;
            text-transform: capitalize;
        }
        
        .message.user .message-role {
            color: var(--accent-secondary);
        }
        
        .message.assistant .message-role {
            color: var(--text-primary);
        }
        
        .message.system .message-role {
            color: var(--warning);
        }
        
        .message.tool .message-role {
            color: var(--text-tertiary);
        }
        
        .message-time {
            font-size: 0.75rem;
            color: var(--text-tertiary);
        }
        
        .message-content {
            font-size: 0.9375rem;
            line-height: 1.7;
            color: var(--text-primary);
            word-wrap: break-word;
        }
        
        .message-content p {
            margin-bottom: 1rem;
        }
        
        .message-content p:last-child {
            margin-bottom: 0;
        }
        
        .message-content code {
            background: var(--background-tertiary);
            color: var(--accent-secondary);
            padding: 0.125rem 0.375rem;
            border-radius: 4px;
            font-family: 'SF Mono', 'Monaco', 'Consolas', 'Liberation Mono', 'Courier New', monospace;
            font-size: 0.875em;
            border: 1px solid var(--border-color);
        }
        
        .message-content pre {
            background: var(--code-bg);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 1rem;
            overflow-x: auto;
            margin: 1rem 0;
            position: relative;
        }
        
        .message-content pre:hover {
            border-color: var(--border-hover);
        }
        
        .message-content pre code {
            background: none;
            color: #abb2bf;
            padding: 0;
            border: none;
            font-size: 0.875rem;
            line-height: 1.5;
        }
        
        .json-content,
        .tool-content {
            background: var(--code-bg) !important;
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 1rem;
            margin: 1rem 0;
            overflow-x: auto;
        }
        
        .json-content code {
            color: #61dafb !important;
            background: none !important;
            border: none !important;
        }
        
        .tool-content code {
            color: #c678dd !important;
            background: none !important;
            border: none !important;
        }
        
        .message-content strong {
            color: var(--text-primary);
            font-weight: 600;
        }
        
        .message-content ul,
        .message-content ol {
            margin: 1rem 0;
            padding-left: 1.5rem;
        }
        
        .message-content li {
            margin: 0.5rem 0;
        }
        
        .message-content blockquote {
            border-left: 3px solid var(--accent-primary);
            padding-left: 1rem;
            margin: 1rem 0;
            color: var(--text-secondary);
        }
        
        .empty-state {
            text-align: center;
            padding: 4rem 2rem;
            color: var(--text-tertiary);
        }
        
        .empty-state h2 {
            font-size: 1.5rem;
            margin-bottom: 0.5rem;
            color: var(--text-secondary);
            font-weight: 600;
        }
        
        .empty-state p {
            font-size: 0.875rem;
        }
        
        .loading {
            display: flex;
            justify-content: center;
            align-items: center;
            padding: 2rem;
        }
        
        .typing-indicator {
            display: flex;
            align-items: center;
            gap: 0.25rem;
            padding: 0.5rem 0;
        }
        
        .typing-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: var(--accent-primary);
            animation: typing 1.4s infinite;
        }
        
        .typing-dot:nth-child(2) {
            animation-delay: 0.2s;
        }
        
        .typing-dot:nth-child(3) {
            animation-delay: 0.4s;
        }
        
        @keyframes typing {
            0%, 60%, 100% {
                transform: translateY(0);
                opacity: 0.5;
            }
            30% {
                transform: translateY(-10px);
                opacity: 1;
            }
        }
        
        /* Responsive design */
        @media (max-width: 768px) {
            .chat-container {
                padding: 1rem 0.75rem;
            }
            
            .message {
                padding: 1rem 0;
            }
            
            .avatar {
                width: 28px;
                height: 28px;
                font-size: 0.75rem;
            }
            
            .message-content {
                font-size: 0.875rem;
            }
            
            .header-content {
                flex-direction: column;
                align-items: flex-start;
                gap: 0.5rem;
            }
            
            .session-info::before {
                display: none;
            }
        }
        
        /* Scrollbar styling */
        ::-webkit-scrollbar {
            width: 8px;
            height: 8px;
        }
        
        ::-webkit-scrollbar-track {
            background: var(--background-primary);
        }
        
        ::-webkit-scrollbar-thumb {
            background: var(--background-tertiary);
            border-radius: 4px;
        }
        
        ::-webkit-scrollbar-thumb:hover {
            background: var(--border-hover);
        }
        
        /* Copy button for code blocks */
        .code-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 0.5rem 1rem;
            background: rgba(255, 255, 255, 0.02);
            border-bottom: 1px solid var(--border-color);
            margin: -1rem -1rem 0.75rem -1rem;
            border-radius: 8px 8px 0 0;
        }
        
        .code-lang {
            font-size: 0.75rem;
            color: var(--text-tertiary);
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }
        
        .copy-button {
            background: transparent;
            border: 1px solid var(--border-color);
            color: var(--text-secondary);
            padding: 0.25rem 0.5rem;
            border-radius: 4px;
            font-size: 0.75rem;
            cursor: pointer;
            transition: all 0.2s ease;
        }
        
        .copy-button:hover {
            background: var(--background-hover);
            border-color: var(--border-hover);
            color: var(--text-primary);
        }
        
        .copy-button.copied {
            background: var(--success);
            border-color: var(--success);
            color: white;
        }
    </style>
</head>
<body>
    <header>
        <div class="header-content">
            <div class="header-left">
                <div class="logo">Toke</div>
                <div class="session-info">
                    <span>{{if .SessionTitle}}{{.SessionTitle}}{{else}}Untitled Session{{end}}</span>
                    <span class="model-badge">{{.Model}}</span>
                </div>
            </div>
            <div class="status">
                <div class="status-dot"></div>
                <span id="status-text">Live</span>
            </div>
        </div>
    </header>
    
    <main>
        <div class="chat-container">
            <div class="messages" id="messages">
                {{if .Messages}}
                    {{range .Messages}}
                    <div class="message-group">
                        <div class="message {{.Role}}" data-id="{{.ID}}">
                            <div class="message-inner">
                                <div class="avatar">
                                    {{if eq .Role "user"}}U{{else if eq .Role "assistant"}}AI{{else if eq .Role "system"}}S{{else}}T{{end}}
                                </div>
                                <div class="message-main">
                                    <div class="message-header">
                                        <span class="message-role">{{.Role}}</span>
                                        <span class="message-time">{{.Timestamp}}</span>
                                    </div>
                                    <div class="message-content">{{.Content}}</div>
                                </div>
                            </div>
                        </div>
                    </div>
                    {{end}}
                {{else}}
                    <div class="empty-state">
                        <h2>Session Ready</h2>
                        <p>Messages will appear here as the conversation progresses</p>
                    </div>
                {{end}}
            </div>
        </div>
    </main>
    
    <script>
        let lastMessageCount = {{len .Messages}};
        let eventSource;
        let isUserAtBottom = true;
        
        // ===== MESSAGE PARSING AND RENDERING =====
        
        // Extract text content from message parts JSON
        function extractTextFromParts(partsString) {
            try {
                const parts = JSON.parse(partsString);
                const textParts = [];
                
                if (Array.isArray(parts)) {
                    for (const part of parts) {
                        // Handle different part structures
                        if (part.data && part.data.text) {
                            // Structure like: {"data": {"text": "..."}, "type": "text"}
                            textParts.push(part.data.text);
                        } else if (part.type === 'text' && part.text) {
                            // Structure like: {"type": "text", "text": "..."}
                            textParts.push(part.text);
                        } else if (part.type === 'tool_use' && part.name) {
                            textParts.push('*Using tool: ' + part.name + '*');
                        } else if (part.type === 'tool_result' && part.content) {
                            textParts.push(part.content);
                        }
                    }
                } else if (typeof parts === 'object') {
                    // Single part object
                    if (parts.data && parts.data.text) {
                        textParts.push(parts.data.text);
                    } else if (parts.type === 'text' && parts.text) {
                        textParts.push(parts.text);
                    }
                }
                
                return textParts.join('\n\n') || partsString;
            } catch (e) {
                // If not valid JSON, return as-is
                return partsString;
            }
        }
        
        // Render markdown to HTML
        function renderMarkdown(text) {
            if (!text) return '';
            
            // Escape HTML
            text = text.replace(/&/g, '&amp;')
                      .replace(/</g, '&lt;')
                      .replace(/>/g, '&gt;')
                      .replace(/"/g, '&quot;')
                      .replace(/'/g, '&#039;');
            
            // Process code blocks first  
            const codeBlocks = [];
            let codeBlockIndex = 0;
            text = text.replace(/\` + "`" + `\` + "`" + `\` + "`" + `(\w*)\n?([\s\S]*?)\` + "`" + `\` + "`" + `\` + "`" + `/g, (match, lang, code) => {
                const placeholder = '___CODEBLOCK_' + codeBlockIndex + '___';
                const langClass = lang || 'plaintext';
                codeBlocks[codeBlockIndex] = '<pre><code class="language-' + langClass + '">' + code.trim() + '</code></pre>';
                codeBlockIndex++;
                return placeholder;
            });
            
            // Split into lines for processing
            const lines = text.split('\n');
            const processedLines = [];
            let inList = false;
            let listType = null;
            
            for (let i = 0; i < lines.length; i++) {
                let line = lines[i];
                
                // Skip if in code block
                if (line.includes('<pre>') || line.includes('</pre>')) {
                    processedLines.push(line);
                    continue;
                }
                
                // Headers
                const headerMatch = line.match(/^(#{1,6})\s+(.+)$/);
                if (headerMatch) {
                    const level = headerMatch[1].length;
                    line = '<h' + level + '>' + processInlineMarkdown(headerMatch[2]) + '</h' + level + '>';
                }
                
                // Lists
                const unorderedListMatch = line.match(/^\s*[-*+]\s+(.+)$/);
                const orderedListMatch = line.match(/^\s*\d+\.\s+(.+)$/);
                
                if (unorderedListMatch) {
                    if (!inList || listType !== 'ul') {
                        if (inList) processedLines.push('</' + listType + '>');
                        processedLines.push('<ul>');
                        inList = true;
                        listType = 'ul';
                    }
                    line = '<li>' + processInlineMarkdown(unorderedListMatch[1]) + '</li>';
                } else if (orderedListMatch) {
                    if (!inList || listType !== 'ol') {
                        if (inList) processedLines.push('</' + listType + '>');
                        processedLines.push('<ol>');
                        inList = true;
                        listType = 'ol';
                    }
                    line = '<li>' + processInlineMarkdown(orderedListMatch[1]) + '</li>';
                } else {
                    if (inList) {
                        processedLines.push('</' + listType + '>');
                        inList = false;
                        listType = null;
                    }
                    
                    // Process inline markdown if not a header
                    if (!headerMatch) {
                        line = processInlineMarkdown(line);
                    }
                }
                
                processedLines.push(line);
            }
            
            // Close any open lists
            if (inList) {
                processedLines.push('</' + listType + '>');
            }
            
            // Join lines and handle paragraphs
            let html = processedLines.join('\n');
            
            // Create paragraphs from double newlines
            html = html.split('\n\n').map(para => {
                para = para.trim();
                if (para && !para.startsWith('<') && !para.includes('___CODEBLOCK_')) {
                    return '<p>' + para + '</p>';
                }
                return para;
            }).join('\n');
            
            // Restore code blocks
            for (let i = 0; i < codeBlocks.length; i++) {
                html = html.replace('___CODEBLOCK_' + i + '___', codeBlocks[i]);
            }
            
            return html;
        }
        
        // Process inline markdown elements
        function processInlineMarkdown(text) {
            // Bold
            text = text.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
            text = text.replace(/__(.+?)__/g, '<strong>$1</strong>');
            
            // Italic
            text = text.replace(/\*([^*]+)\*/g, '<em>$1</em>');
            text = text.replace(/_([^_]+)_/g, '<em>$1</em>');
            
            // Inline code
            text = text.replace(/\` + "`" + `([^\` + "`" + `]+)\` + "`" + `/g, '<code>$1</code>');
            
            // Links
            text = text.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank">$1</a>');
            
            return text;
        }
        
        // Format message content based on role
        function formatMessageContent(partsString, role) {
            const textContent = extractTextFromParts(partsString);
            
            if (role === 'tool') {
                // Tool messages get special formatting
                return '<pre class="tool-content"><code>' + textContent + '</code></pre>';
            }
            
            return renderMarkdown(textContent);
        }
        
        // ===== UI FUNCTIONS =====
        
        // Check if user is at bottom of scroll
        function checkScrollPosition() {
            const main = document.querySelector('main');
            const threshold = 100;
            isUserAtBottom = main.scrollHeight - main.scrollTop - main.clientHeight < threshold;
        }
        
        // Smooth scroll to bottom
        function scrollToBottom(behavior = 'smooth') {
            const main = document.querySelector('main');
            main.scrollTo({
                top: main.scrollHeight,
                behavior: behavior
            });
        }
        
        // Update connection status
        function updateStatus(text, color) {
            document.getElementById('status-text').textContent = text;
            document.querySelector('.status-dot').style.background = color;
        }
        
        // Create message element
        function createMessageElement(msg) {
            const messageGroup = document.createElement('div');
            messageGroup.className = 'message-group';
            
            const avatarText = msg.role === 'user' ? 'U' : 
                              msg.role === 'assistant' ? 'AI' : 
                              msg.role === 'system' ? 'S' : 'T';
            
            // Format the message content
            const formattedContent = formatMessageContent(msg.parts, msg.role);
            
            messageGroup.innerHTML = '<div class="message ' + msg.role + '" data-id="' + msg.id + '">' +
                '<div class="message-inner">' +
                    '<div class="avatar">' + avatarText + '</div>' +
                    '<div class="message-main">' +
                        '<div class="message-header">' +
                            '<span class="message-role">' + msg.role + '</span>' +
                            '<span class="message-time">' + msg.timestamp + '</span>' +
                        '</div>' +
                        '<div class="message-content">' + formattedContent + '</div>' +
                    '</div>' +
                '</div>' +
            '</div>';
            
            // Add copy buttons to code blocks
            messageGroup.querySelectorAll('pre').forEach(pre => {
                if (!pre.querySelector('.code-header')) {
                    addCopyButton(pre);
                }
            });
            
            return messageGroup;
        }
        
        // Add copy button to code blocks
        function addCopyButton(pre) {
            const codeHeader = document.createElement('div');
            codeHeader.className = 'code-header';
            
            const langLabel = document.createElement('span');
            langLabel.className = 'code-lang';
            const codeElement = pre.querySelector('code');
            if (codeElement && codeElement.className) {
                const langMatch = codeElement.className.match(/language-(\w+)/);
                langLabel.textContent = langMatch ? langMatch[1].toUpperCase() : 'CODE';
            } else {
                langLabel.textContent = 'CODE';
            }
            
            const copyBtn = document.createElement('button');
            copyBtn.className = 'copy-button';
            copyBtn.textContent = 'Copy';
            copyBtn.onclick = function() {
                const code = pre.querySelector('code');
                const text = code ? code.textContent : pre.textContent;
                
                navigator.clipboard.writeText(text).then(() => {
                    copyBtn.textContent = 'Copied!';
                    copyBtn.classList.add('copied');
                    setTimeout(() => {
                        copyBtn.textContent = 'Copy';
                        copyBtn.classList.remove('copied');
                    }, 2000);
                });
            };
            
            codeHeader.appendChild(langLabel);
            codeHeader.appendChild(copyBtn);
            pre.insertBefore(codeHeader, pre.firstChild);
        }
        
        // Update messages display
        function updateMessages(messages) {
            if (messages.length === lastMessageCount) return;
            
            const container = document.getElementById('messages');
            
            // Remove empty state if it exists
            const emptyState = container.querySelector('.empty-state');
            if (emptyState) {
                emptyState.remove();
            }
            
            // Add new messages
            for (let i = lastMessageCount; i < messages.length; i++) {
                const msg = messages[i];
                const messageElement = createMessageElement(msg);
                container.appendChild(messageElement);
                
                // Auto-scroll if user was at bottom
                if (isUserAtBottom) {
                    scrollToBottom();
                }
            }
            
            lastMessageCount = messages.length;
        }
        
        // ===== SSE AND POLLING =====
        
        function connectSSE() {
            eventSource = new EventSource('/events');
            
            eventSource.onmessage = function(event) {
                const data = JSON.parse(event.data);
                
                if (data.type === 'connected') {
                    updateStatus('Live', 'var(--success)');
                } else if (data.type === 'messages') {
                    updateMessages(data.messages);
                }
            };
            
            eventSource.onerror = function() {
                updateStatus('Reconnecting', 'var(--warning)');
                setTimeout(connectSSE, 5000);
            };
        }
        
        // Poll for updates as backup
        function pollMessages() {
            fetch('/api/messages')
                .then(res => res.json())
                .then(messages => {
                    updateMessages(messages);
                })
                .catch(err => {
                    console.error('Poll error:', err);
                    updateStatus('Connection Error', 'var(--error)');
                });
        }
        
        // ===== INITIALIZATION =====
        
        document.addEventListener('DOMContentLoaded', () => {
            // Track scroll position
            const main = document.querySelector('main');
            main.addEventListener('scroll', checkScrollPosition);
            
            // Add copy buttons to existing code blocks
            document.querySelectorAll('pre').forEach(pre => {
                addCopyButton(pre);
            });
            
            // Connect SSE
            connectSSE();
            
            // Poll every 3 seconds as backup
            setInterval(pollMessages, 3000);
            
            // Auto-scroll to bottom on load
            scrollToBottom('auto');
        });
        
        // Handle visibility change
        document.addEventListener('visibilitychange', () => {
            if (!document.hidden && eventSource.readyState === EventSource.CLOSED) {
                connectSSE();
            }
        });
    </script>
</body>
</html>`