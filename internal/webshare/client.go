package webshare

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	
	tea "github.com/charmbracelet/bubbletea/v2"
)

// SSEClient connects to the web share server and listens for buddy events
type SSEClient struct {
	url     string
	program *tea.Program
}

// NewSSEClient creates a new SSE client
func NewSSEClient(baseURL string, program *tea.Program) *SSEClient {
	return &SSEClient{
		url:     baseURL + "/events",
		program: program,
	}
}

// Start begins listening for SSE events in a goroutine
func (c *SSEClient) Start() {
	go c.listen()
}

// listen handles the SSE connection
func (c *SSEClient) listen() {
	fmt.Fprintf(os.Stderr, "[DEBUG] SSEClient: Starting connection to %s\n", c.url)
	
	// Create HTTP request
	req, err := http.NewRequest("GET", c.url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] SSEClient: Failed to create request: %v\n", err)
		return
	}
	
	// Set SSE headers
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	
	// Make request with timeout
	client := &http.Client{
		Timeout: 0, // No timeout for SSE
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] SSEClient: Failed to connect: %v\n", err)
		return
	}
	defer resp.Body.Close()
	
	fmt.Fprintf(os.Stderr, "[DEBUG] SSEClient: Connected successfully\n")
	
	// Read events
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		
		// Parse SSE data
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			
			// Parse JSON
			var event map[string]interface{}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}
			
			// Handle different event types
			eventType, ok := event["type"].(string)
			if !ok {
				continue
			}
			
			var msg tea.Msg
			switch eventType {
			case "buddy_joined":
				if buddyData, ok := event["buddy"].(map[string]interface{}); ok {
					buddy := &Buddy{
						ID:   getString(buddyData, "id"),
						Name: getString(buddyData, "name"),
					}
					msg = BuddyEventMsg{
						Type:  "joined",
						Buddy: buddy,
					}
					fmt.Fprintf(os.Stderr, "[DEBUG] SSEClient: Buddy joined: %s\n", buddy.Name)
				}
				
			case "buddy_message":
				if msgData, ok := event["message"].(map[string]interface{}); ok {
					bmsg := BuddyMessage{
						FromID:   getString(msgData, "from_id"),
						FromName: getString(msgData, "from_name"),
						ToID:     getString(msgData, "to_id"),
						Message:  getString(msgData, "message"),
						Time:     time.Now(),
					}
					msg = BuddyEventMsg{
						Type:    "message",
						Message: bmsg,
					}
					fmt.Fprintf(os.Stderr, "[DEBUG] SSEClient: Message from %s: %s\n", bmsg.FromName, bmsg.Message)
				}
				
			case "buddy_left":
				if buddyData, ok := event["buddy"].(map[string]interface{}); ok {
					buddy := &Buddy{
						ID: getString(buddyData, "id"),
					}
					msg = BuddyEventMsg{
						Type:  "left",
						Buddy: buddy,
					}
					fmt.Fprintf(os.Stderr, "[DEBUG] SSEClient: Buddy left: %s\n", buddy.ID)
				}
			}
			
			// Send message to the program if we have one
			if msg != nil && c.program != nil {
				c.program.Send(msg)
			}
		}
	}
	
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] SSEClient: Scanner error: %v\n", err)
	}
}

// getString safely gets a string from a map
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}