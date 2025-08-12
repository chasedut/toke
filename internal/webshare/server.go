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
	buddies      map[string]*Buddy  // Track connected buddies
	buddyMsgChan chan BuddyMessage  // Channel for buddy messages
}

type Buddy struct {
	ID       string
	Name     string
	JoinedAt time.Time
}

type BuddyMessage struct {
	FromID   string    `json:"from_id"`
	FromName string    `json:"from_name"`
	ToID     string    `json:"to_id"`
	Message  string    `json:"message"`
	Time     time.Time `json:"time"`
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
		sessionID:    sessionID,
		sseClients:   make(map[chan string]bool),
		buddies:      make(map[string]*Buddy),
		buddyMsgChan: make(chan BuddyMessage, 100),
	}
}

func (s *SessionShare) Start() (*ShareURLs, error) {
	fmt.Fprintf(os.Stderr, "[DEBUG] SessionShare.Start: Starting\n")
	
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel

	// Set up HTTP endpoints
	fmt.Fprintf(os.Stderr, "[DEBUG] SessionShare.Start: Setting up HTTP endpoints\n")
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleHome)
	mux.HandleFunc("/api/messages", s.handleMessagesAPI)
	mux.HandleFunc("/events", s.handleSSE)
	mux.HandleFunc("/api/buddy/join", s.handleBuddyJoin)
	mux.HandleFunc("/api/buddy/message", s.handleBuddyMessage)
	mux.HandleFunc("/api/buddy/list", s.handleBuddyList)

	// Find an available port and start server
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] SessionShare.Start: Failed to create listener: %v\n", err)
		return nil, fmt.Errorf("failed to create listener: %w", err)
	}
	
	s.port = listener.Addr().(*net.TCPAddr).Port
	fmt.Fprintf(os.Stderr, "[DEBUG] SessionShare.Start: Got port %d\n", s.port)
	
	s.server = &http.Server{
		Addr:    fmt.Sprintf("localhost:%d", s.port),
		Handler: mux,
	}

	// Start server in background
	go func() {
		fmt.Fprintf(os.Stderr, "[DEBUG] SessionShare.Start: Starting server on localhost:%d\n", s.port)
		// Close the temporary listener first
		listener.Close()
		// Start the actual server
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "[DEBUG] Server error: %v\n", err)
		}
	}()

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)
	
	// Verify server is running
	for i := 0; i < 5; i++ {
		testConn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", s.port))
		if err == nil {
			testConn.Close()
			fmt.Fprintf(os.Stderr, "[DEBUG] SessionShare.Start: Server verified on port %d\n", s.port)
			break
		}
		if i == 4 {
			fmt.Fprintf(os.Stderr, "[DEBUG] SessionShare.Start: WARNING - Server not responding on port %d after 5 attempts\n", s.port)
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Get local URL
	s.localURL = fmt.Sprintf("http://localhost:%d", s.port)
	fmt.Fprintf(os.Stderr, "[DEBUG] SessionShare.Start: Local URL: %s\n", s.localURL)

	// Start ngrok asynchronously
	fmt.Fprintf(os.Stderr, "[DEBUG] SessionShare.Start: Starting ngrok async\n")
	go func() {
		fmt.Fprintf(os.Stderr, "[DEBUG] SessionShare.Start: ngrok goroutine started\n")
		if err := s.startNgrok(); err != nil {
			// Non-fatal - continue without ngrok
			fmt.Fprintf(os.Stderr, "[DEBUG] Warning: Failed to start ngrok: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "[DEBUG] SessionShare.Start: ngrok started successfully, URL: %s\n", s.ngrokURL)
		}
	}()

	// Return immediately with local URL, ngrok URL will be populated later
	fmt.Fprintf(os.Stderr, "[DEBUG] SessionShare.Start: Returning URLs\n")
	return &ShareURLs{
		LocalURL: s.localURL,
		NgrokURL: "", // Will be populated asynchronously
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
	ngrokCmd := fmt.Sprintf("%s http %d", ngrokPath, s.port)
	fmt.Fprintf(os.Stderr, "[DEBUG] startNgrok: Running command: %s\n", ngrokCmd)
	s.ngrokProcess = exec.CommandContext(s.ctx, ngrokPath, "http", fmt.Sprintf("%d", s.port))
	if err := s.ngrokProcess.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] startNgrok: Failed to start ngrok: %v\n", err)
		return fmt.Errorf("failed to start ngrok: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[DEBUG] startNgrok: ngrok process started, PID: %d\n", s.ngrokProcess.Process.Pid)

	// Retry getting ngrok URL with exponential backoff
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		// Wait before checking (exponential backoff)
		time.Sleep(time.Duration(500*(i+1)) * time.Millisecond)
		
		// Get ngrok URL from API
		resp, err := http.Get("http://localhost:4040/api/tunnels")
		if err != nil {
			if i == maxRetries-1 {
				return fmt.Errorf("failed to get ngrok tunnels after %d retries: %w", maxRetries, err)
			}
			continue // Retry
		}
		
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			if i == maxRetries-1 {
				return fmt.Errorf("failed to read ngrok response: %w", err)
			}
			continue // Retry
		}

		var tunnels struct {
			Tunnels []struct {
				PublicURL string `json:"public_url"`
				Proto     string `json:"proto"`
			} `json:"tunnels"`
		}

		if err := json.Unmarshal(body, &tunnels); err != nil {
			if i == maxRetries-1 {
				return fmt.Errorf("failed to parse ngrok response: %w", err)
			}
			continue // Retry
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
		
		// If we got a URL, we're done
		if s.ngrokURL != "" {
			break
		}
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

// StartBroadcastLoop starts a goroutine that periodically broadcasts updates
func (s *SessionShare) StartBroadcastLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			s.BroadcastUpdate()
		case <-s.ctx.Done():
			// Context cancelled, exit the goroutine
			return
		}
	}
}

func (s *SessionShare) GetURLs() *ShareURLs {
	return &ShareURLs{
		LocalURL: s.localURL,
		NgrokURL: s.ngrokURL,
	}
}

// GetNgrokURL returns the current ngrok URL (may be empty if still starting)
func (s *SessionShare) GetNgrokURL() string {
	return s.ngrokURL
}

// Buddy chat handlers
func (s *SessionShare) handleBuddyJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Generate buddy ID
	buddyID := fmt.Sprintf("buddy_%d", time.Now().UnixNano())
	
	buddy := &Buddy{
		ID:       buddyID,
		Name:     req.Name,
		JoinedAt: time.Now(),
	}
	
	s.buddies[buddyID] = buddy
	
	// Broadcast buddy joined event
	s.broadcastBuddyEvent("buddy_joined", buddy)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"id":   buddyID,
		"name": buddy.Name,
	})
}

func (s *SessionShare) handleBuddyMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var msg BuddyMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	
	msg.Time = time.Now()
	
	// Broadcast buddy message
	s.broadcastBuddyMessage(msg)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *SessionShare) handleBuddyList(w http.ResponseWriter, r *http.Request) {
	buddyList := make([]*Buddy, 0, len(s.buddies))
	for _, buddy := range s.buddies {
		buddyList = append(buddyList, buddy)
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(buddyList)
}

func (s *SessionShare) broadcastBuddyEvent(eventType string, buddy *Buddy) {
	data, _ := json.Marshal(map[string]interface{}{
		"type":      eventType,
		"buddy":     buddy,
		"timestamp": time.Now().Format("15:04:05"),
	})
	
	for client := range s.sseClients {
		select {
		case client <- string(data):
		default:
		}
	}
}

func (s *SessionShare) broadcastBuddyMessage(msg BuddyMessage) {
	// Store message in channel for TUI polling
	select {
	case s.buddyMsgChan <- msg:
	default:
		// Channel full, skip
	}
	
	// Broadcast to SSE clients
	data, _ := json.Marshal(map[string]interface{}{
		"type":    "buddy_message",
		"message": msg,
	})
	
	for client := range s.sseClients {
		select {
		case client <- string(data):
		default:
		}
	}
}

// GetBuddies returns the list of connected buddies
func (s *SessionShare) GetBuddies() []*Buddy {
	buddyList := make([]*Buddy, 0, len(s.buddies))
	for _, buddy := range s.buddies {
		buddyList = append(buddyList, buddy)
	}
	return buddyList
}

// SendMessageToBuddy sends a message from the host to a buddy
func (s *SessionShare) SendMessageToBuddy(buddyID, message string) {
	msg := BuddyMessage{
		FromID:   "host",
		FromName: "Host",
		ToID:     buddyID,
		Message:  message,
		Time:     time.Now(),
	}
	s.broadcastBuddyMessage(msg)
}

// GetPendingMessages returns any pending buddy messages (for polling)
func (s *SessionShare) GetPendingMessages() []BuddyMessage {
	messages := make([]BuddyMessage, 0)
	
	// Try to get messages from channel without blocking
	for {
		select {
		case msg := <-s.buddyMsgChan:
			messages = append(messages, msg)
		default:
			// No more messages
			return messages
		}
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

// extractTextFromJSONParts extracts and formats text from JSON message parts
func extractTextFromJSONParts(partsJSON string) string {
	var parts []interface{}
	if err := json.Unmarshal([]byte(partsJSON), &parts); err != nil {
		return "Error parsing message"
	}
	
	var output []string
	for _, p := range parts {
		part, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		
		partType, _ := part["type"].(string)
		
		switch partType {
		case "text":
			// Handle text parts
			if data, ok := part["data"].(map[string]interface{}); ok {
				if text, ok := data["text"].(string); ok {
					output = append(output, text)
				}
			} else if text, ok := part["text"].(string); ok {
				output = append(output, text)
			}
			
		case "tool_call":
			// Format tool calls nicely
			if data, ok := part["data"].(map[string]interface{}); ok {
				toolName, _ := data["name"].(string)
				output = append(output, fmt.Sprintf("\n🔧 Using tool: %s\n", toolName))
				
				// Optionally show input in a code block
				if input, ok := data["input"].(map[string]interface{}); ok {
					if len(input) > 0 {
						inputJSON, _ := json.MarshalIndent(input, "", "  ")
						output = append(output, fmt.Sprintf("```json\n%s\n```\n", string(inputJSON)))
					}
				}
			}
			
		case "tool_use":
			// Legacy tool use format
			if name, ok := part["name"].(string); ok {
				output = append(output, fmt.Sprintf("\n🔧 Using tool: %s\n", name))
			}
			
		case "tool_result":
			// Format tool results
			if data, ok := part["data"].(map[string]interface{}); ok {
				if content, ok := data["content"].(string); ok {
					// Truncate very long outputs
					if len(content) > 500 {
						content = content[:500] + "...\n[Output truncated]"
					}
					output = append(output, fmt.Sprintf("```\n%s\n```\n", content))
				}
			} else if content, ok := part["content"].(string); ok {
				if len(content) > 500 {
					content = content[:500] + "...\n[Output truncated]"
				}
				output = append(output, fmt.Sprintf("```\n%s\n```\n", content))
			}
			
		case "finish":
			// Handle finish messages
			if data, ok := part["data"].(map[string]interface{}); ok {
				if reason, ok := data["reason"].(string); ok && reason == "stop" {
					// Normal completion, don't show anything
				} else if reason == "error" {
					output = append(output, "\n❌ An error occurred")
				}
			}
		}
	}
	
	if len(output) == 0 {
		return "No content to display"
	}
	
	return strings.Join(output, "\n")
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
        /* Buddy Chat Styles */
        .buddy-chat-container {
            position: fixed;
            bottom: 20px;
            right: 20px;
            width: 350px;
            max-height: 500px;
            background: var(--background-secondary);
            border: 1px solid var(--border-color);
            border-radius: 12px;
            display: flex;
            flex-direction: column;
            box-shadow: 0 10px 40px rgba(0, 0, 0, 0.5);
            z-index: 1000;
            transition: all 0.3s ease;
        }
        
        .buddy-chat-container.collapsed {
            height: 50px;
            max-height: 50px;
        }
        
        .buddy-chat-header {
            padding: 1rem;
            background: var(--background-tertiary);
            border-radius: 12px 12px 0 0;
            border-bottom: 1px solid var(--border-color);
            cursor: pointer;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        
        .buddy-chat-title {
            font-weight: 600;
            color: var(--text-primary);
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }
        
        .buddy-status {
            font-size: 0.75rem;
            color: var(--text-secondary);
        }
        
        .buddy-join-form {
            padding: 1.5rem;
            border-bottom: 1px solid var(--border-color);
        }
        
        .buddy-join-form input {
            width: 100%;
            padding: 0.75rem;
            background: var(--background-primary);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            color: var(--text-primary);
            margin-bottom: 1rem;
        }
        
        .buddy-join-form button {
            width: 100%;
            padding: 0.75rem;
            background: var(--accent-gradient);
            border: none;
            border-radius: 8px;
            color: white;
            font-weight: 600;
            cursor: pointer;
            transition: opacity 0.2s;
        }
        
        .buddy-join-form button:hover {
            opacity: 0.9;
        }
        
        .buddy-messages {
            flex: 1;
            overflow-y: auto;
            padding: 1rem;
            max-height: 300px;
        }
        
        .buddy-message {
            margin-bottom: 1rem;
            animation: fadeIn 0.3s ease;
        }
        
        .buddy-message-header {
            display: flex;
            align-items: center;
            gap: 0.5rem;
            margin-bottom: 0.25rem;
        }
        
        .buddy-message-name {
            font-weight: 600;
            color: var(--accent-secondary);
            font-size: 0.875rem;
        }
        
        .buddy-message-time {
            font-size: 0.75rem;
            color: var(--text-tertiary);
        }
        
        .buddy-message-text {
            color: var(--text-primary);
            font-size: 0.875rem;
            padding: 0.5rem;
            background: var(--background-primary);
            border-radius: 8px;
            word-wrap: break-word;
        }
        
        .buddy-message.host .buddy-message-name {
            color: var(--accent-primary);
        }
        
        .buddy-input-container {
            padding: 1rem;
            border-top: 1px solid var(--border-color);
            display: flex;
            gap: 0.5rem;
        }
        
        .buddy-input {
            flex: 1;
            padding: 0.5rem;
            background: var(--background-primary);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            color: var(--text-primary);
        }
        
        .buddy-send-btn {
            padding: 0.5rem 1rem;
            background: var(--accent-gradient);
            border: none;
            border-radius: 8px;
            color: white;
            cursor: pointer;
            transition: opacity 0.2s;
        }
        
        .buddy-send-btn:hover {
            opacity: 0.9;
        }
        
        .buddy-chat-container.hidden {
            display: none;
        }
        
        @keyframes fadeIn {
            from { opacity: 0; transform: translateY(10px); }
            to { opacity: 1; transform: translateY(0); }
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
    
    <!-- Buddy Chat -->
    <div class="buddy-chat-container" id="buddyChat">
        <div class="buddy-chat-header" onclick="toggleBuddyChat()">
            <div class="buddy-chat-title">
                💬 Buddy Chat
                <span class="buddy-status" id="buddyStatus">Not connected</span>
            </div>
            <span id="toggleIcon">▼</span>
        </div>
        
        <!-- Join Form (shown when not connected) -->
        <div class="buddy-join-form" id="buddyJoinForm">
            <input type="text" id="buddyName" placeholder="Enter your name" maxlength="20">
            <button onclick="joinBuddyChat()">Join Chat</button>
        </div>
        
        <!-- Chat Interface (shown when connected) -->
        <div id="buddyChatInterface" style="display: none; flex: 1; display: flex; flex-direction: column;">
            <div class="buddy-messages" id="buddyMessages"></div>
            <div class="buddy-input-container">
                <input type="text" class="buddy-input" id="buddyMessageInput" placeholder="Type a message..." onkeypress="if(event.key==='Enter') sendBuddyMessage()">
                <button class="buddy-send-btn" onclick="sendBuddyMessage()">Send</button>
            </div>
        </div>
    </div>
    
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
        
        // Render markdown to HTML with better formatting
        function renderMarkdown(text) {
            if (!text) return '';
            
            // Escape HTML first
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
            
            // Process existing messages to render markdown properly
            document.querySelectorAll('.message-content').forEach(content => {
                const text = content.textContent;
                // Re-render with proper markdown
                if (text && !content.dataset.processed) {
                    content.innerHTML = renderMarkdown(text);
                    content.dataset.processed = 'true';
                }
            });
            
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
        
        // ===== BUDDY CHAT FUNCTIONALITY =====
        
        let buddyId = null;
        let buddyName = null;
        let isChatCollapsed = false;
        
        function toggleBuddyChat() {
            const chat = document.getElementById('buddyChat');
            const icon = document.getElementById('toggleIcon');
            isChatCollapsed = !isChatCollapsed;
            
            if (isChatCollapsed) {
                chat.classList.add('collapsed');
                icon.textContent = '▲';
            } else {
                chat.classList.remove('collapsed');
                icon.textContent = '▼';
            }
        }
        
        function joinBuddyChat() {
            const nameInput = document.getElementById('buddyName');
            const name = nameInput.value.trim();
            
            if (!name) {
                alert('Please enter your name');
                return;
            }
            
            // Join as buddy
            fetch('/api/buddy/join', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({name: name})
            })
            .then(res => res.json())
            .then(data => {
                buddyId = data.id;
                buddyName = data.name;
                
                // Update UI
                document.getElementById('buddyStatus').textContent = 'Connected as ' + buddyName;
                document.getElementById('buddyJoinForm').style.display = 'none';
                document.getElementById('buddyChatInterface').style.display = 'flex';
                
                // Notify host via SSE
                console.log('Joined as buddy:', buddyName);
            })
            .catch(err => {
                console.error('Failed to join:', err);
                alert('Failed to join chat');
            });
        }
        
        function sendBuddyMessage() {
            const input = document.getElementById('buddyMessageInput');
            const message = input.value.trim();
            
            if (!message || !buddyId) return;
            
            // Send message
            fetch('/api/buddy/message', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({
                    from_id: buddyId,
                    from_name: buddyName,
                    to_id: 'host',
                    message: message
                })
            })
            .then(res => res.json())
            .then(data => {
                // Don't add message locally - it will come back via SSE
                input.value = '';
            })
            .catch(err => {
                console.error('Failed to send message:', err);
            });
        }
        
        function addBuddyMessage(name, message, time, isHost) {
            const messagesContainer = document.getElementById('buddyMessages');
            const messageDiv = document.createElement('div');
            messageDiv.className = 'buddy-message' + (isHost ? ' host' : '');
            
            messageDiv.innerHTML = '<div class="buddy-message-header">' +
                '<span class="buddy-message-name">' + name + '</span>' +
                '<span class="buddy-message-time">' + time + '</span>' +
                '</div>' +
                '<div class="buddy-message-text">' + message + '</div>';
            
            messagesContainer.appendChild(messageDiv);
            messagesContainer.scrollTop = messagesContainer.scrollHeight;
        }
        
        // Listen for buddy events via SSE
        if (eventSource) {
            eventSource.addEventListener('message', (event) => {
                try {
                    const data = JSON.parse(event.data);
                    if (data.type === 'buddy_message') {
                        const msg = data.message;
                        // Show messages that are:
                        // 1. From this buddy (their own messages coming back)
                        // 2. From host to this buddy or to all buddies
                        if (msg.from_id === buddyId || 
                            (msg.from_id === 'host' && (msg.to_id === buddyId || msg.to_id === 'all'))) {
                            addBuddyMessage(
                                msg.from_name,
                                msg.message,
                                new Date(msg.time || Date.now()).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'}),
                                msg.from_id === 'host'
                            );
                        }
                    }
                } catch (e) {
                    // Ignore parsing errors
                }
            });
        }
    </script>
</body>
</html>`