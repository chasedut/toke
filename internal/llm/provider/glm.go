package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/catwalk/pkg/catwalk"
	"github.com/google/uuid"
	"github.com/chasedut/toke/internal/llm/tools"
	"github.com/chasedut/toke/internal/message"
)

// GLMProvider wraps GLM models served via MLX with OpenAI-compatible API
type GLMProvider struct {
	baseProvider[*glmClient]
}

type glmClient struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      catwalk.Model
	maxTokens  int64
}

// NewGLMProvider creates a new GLM provider for MLX-served models
func NewGLMProvider(opts ...ProviderClientOption) Provider {
	options := providerClientOptions{
		baseURL: "http://localhost:11434/v1", // Default local MLX server
	}
	
	for _, opt := range opts {
		opt(&options)
	}
	
	client := &glmClient{
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		baseURL:    options.baseURL,
		apiKey:     options.apiKey,
		model:      options.model(options.modelType),
		maxTokens:  options.maxTokens,
	}
	
	return &GLMProvider{
		baseProvider: baseProvider[*glmClient]{
			options: options,
			client:  client,
		},
	}
}

func (c *glmClient) Model() catwalk.Model {
	return c.model
}

// GLM-specific message format
type glmMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type glmRequest struct {
	Model       string       `json:"model"`
	Messages    []glmMessage `json:"messages"`
	MaxTokens   int64        `json:"max_tokens,omitempty"`
	Temperature float64      `json:"temperature"`
	Stream      bool         `json:"stream"`
	Tools       []glmTool    `json:"tools,omitempty"`
}

type glmTool struct {
	Type     string      `json:"type"`
	Function glmFunction `json:"function"`
}

type glmFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type glmResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []glmChoice  `json:"choices"`
	Usage   glmUsage     `json:"usage,omitempty"`
	Error   *glmError    `json:"error,omitempty"`
}

type glmChoice struct {
	Index        int            `json:"index"`
	Message      glmMessage     `json:"message,omitempty"`
	Delta        glmMessage     `json:"delta,omitempty"`
	FinishReason string         `json:"finish_reason,omitempty"`
	ToolCalls    []glmToolCall  `json:"tool_calls,omitempty"`
}

type glmToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function glmToolFunction `json:"function"`
}

type glmToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type glmUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type glmError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// Convert internal messages to GLM format
func (c *glmClient) convertMessages(messages []message.Message) []glmMessage {
	var glmMessages []glmMessage
	
	for _, msg := range messages {
		content := msg.Content().String()
		
		// Map roles appropriately
		role := string(msg.Role)
		if role == "human" {
			role = "user"
		}
		
		// Handle tool results specially for GLM
		if msg.Role == message.User && len(msg.ToolResults()) > 0 {
			// Inject tool results into the content for GLM to understand
			var toolContent strings.Builder
			toolContent.WriteString(content)
			if content != "" {
				toolContent.WriteString("\n\n")
			}
			toolContent.WriteString("Tool Results:\n")
			for _, result := range msg.ToolResults() {
				toolContent.WriteString(fmt.Sprintf("- %s: %s\n", result.Name, result.Content))
			}
			content = toolContent.String()
		}
		
		glmMessages = append(glmMessages, glmMessage{
			Role:    role,
			Content: content,
		})
	}
	
	return glmMessages
}

// Convert tools to GLM format (OpenAI-compatible)
func (c *glmClient) convertTools(tools []tools.BaseTool) []glmTool {
	if len(tools) == 0 {
		return nil
	}
	
	var glmTools []glmTool
	for _, tool := range tools {
		info := tool.Info()
		glmTools = append(glmTools, glmTool{
			Type: "function",
			Function: glmFunction{
				Name:        info.Name,
				Description: info.Description,
				Parameters:  info.Parameters,
			},
		})
	}
	
	return glmTools
}

// Parse tool calls from GLM response
func (c *glmClient) parseToolCalls(toolCalls []glmToolCall) []message.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	
	var calls []message.ToolCall
	for _, tc := range toolCalls {
		calls = append(calls, message.ToolCall{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: tc.Function.Arguments,
			Type:  "function",
		})
	}
	
	return calls
}

func (c *glmClient) send(ctx context.Context, messages []message.Message, tools []tools.BaseTool) (*ProviderResponse, error) {
	req := glmRequest{
		Model:       "glm-4.5-air-3bit", // Model identifier for MLX
		Messages:    c.convertMessages(messages),
		MaxTokens:   c.maxTokens,
		Temperature: 0.7,
		Stream:      false,
		Tools:       c.convertTools(tools),
	}
	
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	var glmResp glmResponse
	if err := json.Unmarshal(body, &glmResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	
	if glmResp.Error != nil {
		return nil, fmt.Errorf("GLM API error: %s", glmResp.Error.Message)
	}
	
	if len(glmResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}
	
	choice := glmResp.Choices[0]
	
	return &ProviderResponse{
		Content:   choice.Message.Content,
		ToolCalls: c.parseToolCalls(choice.ToolCalls),
		Usage: TokenUsage{
			InputTokens:  int64(glmResp.Usage.PromptTokens),
			OutputTokens: int64(glmResp.Usage.CompletionTokens),
		},
		FinishReason: c.mapFinishReason(choice.FinishReason),
	}, nil
}

func (c *glmClient) stream(ctx context.Context, messages []message.Message, tools []tools.BaseTool) <-chan ProviderEvent {
	events := make(chan ProviderEvent)
	
	go func() {
		defer close(events)
		
		req := glmRequest{
			Model:       "glm-4.5-air-3bit",
			Messages:    c.convertMessages(messages),
			MaxTokens:   c.maxTokens,
			Temperature: 0.7,
			Stream:      true,
			Tools:       c.convertTools(tools),
		}
		
		jsonData, err := json.Marshal(req)
		if err != nil {
			events <- ProviderEvent{Type: EventError, Error: fmt.Errorf("failed to marshal request: %w", err)}
			return
		}
		
		httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
		if err != nil {
			events <- ProviderEvent{Type: EventError, Error: fmt.Errorf("failed to create request: %w", err)}
			return
		}
		
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")
		if c.apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		}
		
		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			events <- ProviderEvent{Type: EventError, Error: fmt.Errorf("failed to send request: %w", err)}
			return
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			events <- ProviderEvent{Type: EventError, Error: fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))}
			return
		}
		
		// Parse SSE stream
		c.parseSSEStream(resp.Body, events)
	}()
	
	return events
}

func (c *glmClient) parseSSEStream(reader io.Reader, events chan<- ProviderEvent) {
	scanner := bufio.NewScanner(reader)
	var contentBuffer strings.Builder
	var toolCalls []message.ToolCall
	var currentToolCall *message.ToolCall
	var usage TokenUsage
	
	for scanner.Scan() {
		line := scanner.Text()
		
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			// Send final response
			events <- ProviderEvent{
				Type: EventComplete,
				Response: &ProviderResponse{
					Content:      contentBuffer.String(),
					ToolCalls:    toolCalls,
					Usage:        usage,
					FinishReason: message.FinishReasonEndTurn,
				},
			}
			return
		}
		
		var chunk glmResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		
		if len(chunk.Choices) == 0 {
			continue
		}
		
		choice := chunk.Choices[0]
		
		// Handle content delta
		if choice.Delta.Content != "" {
			if contentBuffer.Len() == 0 {
				events <- ProviderEvent{Type: EventContentStart}
			}
			contentBuffer.WriteString(choice.Delta.Content)
			events <- ProviderEvent{
				Type:    EventContentDelta,
				Content: choice.Delta.Content,
			}
		}
		
		// Handle tool calls
		if len(choice.ToolCalls) > 0 {
			for _, tc := range choice.ToolCalls {
				if currentToolCall == nil || currentToolCall.ID != tc.ID {
					// New tool call
					currentToolCall = &message.ToolCall{
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Input: tc.Function.Arguments,
						Type:  "function",
					}
					toolCalls = append(toolCalls, *currentToolCall)
					events <- ProviderEvent{
						Type:     EventToolUseStart,
						ToolCall: currentToolCall,
					}
				} else {
					// Continue existing tool call
					currentToolCall.Input += tc.Function.Arguments
					events <- ProviderEvent{
						Type:     EventToolUseDelta,
						ToolCall: currentToolCall,
					}
				}
			}
		}
		
		// Handle finish reason
		if choice.FinishReason != "" {
			if currentToolCall != nil {
				events <- ProviderEvent{
					Type:     EventToolUseStop,
					ToolCall: currentToolCall,
				}
				currentToolCall = nil
			}
			if contentBuffer.Len() > 0 {
				events <- ProviderEvent{Type: EventContentStop}
			}
		}
	}
	
	if err := scanner.Err(); err != nil {
		events <- ProviderEvent{Type: EventError, Error: fmt.Errorf("stream reading error: %w", err)}
	}
}

func (c *glmClient) mapFinishReason(reason string) message.FinishReason {
	switch reason {
	case "stop":
		return message.FinishReasonEndTurn
	case "length", "max_tokens":
		return message.FinishReasonMaxTokens
	case "tool_calls", "function_call":
		return message.FinishReasonToolUse
	default:
		return message.FinishReasonEndTurn
	}
}

// Helper to generate unique IDs for tool calls
func generateToolCallID() string {
	return "call_" + uuid.New().String()[:8]
}