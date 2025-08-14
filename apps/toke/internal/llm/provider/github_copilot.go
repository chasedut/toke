package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/catwalk/pkg/catwalk"
	"github.com/chasedut/toke/internal/config"
	"github.com/chasedut/toke/internal/llm/tools"
	"github.com/chasedut/toke/internal/log"
	"github.com/chasedut/toke/internal/message"
	"github.com/google/uuid"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
)

const (
	// GitHub Copilot API endpoint
	githubCopilotAPIURL = "https://api.githubcopilot.com"
	// GitHub OAuth scopes required for Copilot
	githubCopilotScopes = "copilot"
)

type githubCopilotClient struct {
	providerOptions providerClientOptions
	client          openai.Client // GitHub Copilot API is OpenAI-compatible
	accessToken     string
	refreshToken    string
	tokenExpiry     time.Time
}

type GitHubCopilotClient ProviderClient

// GitHubOAuthToken represents the OAuth token response from GitHub
type GitHubOAuthToken struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
}

func newGitHubCopilotClient(opts providerClientOptions) GitHubCopilotClient {
	// Parse access token from API key
	// Expected format: "github_pat_..." or OAuth token
	var accessToken, refreshToken string
	var tokenExpiry time.Time
	
	// Check if API key contains OAuth token data
	if strings.Contains(opts.apiKey, "|") {
		// Format: "access_token|refresh_token|expiry"
		parts := strings.Split(opts.apiKey, "|")
		if len(parts) >= 1 {
			accessToken = parts[0]
		}
		if len(parts) >= 2 {
			refreshToken = parts[1]
		}
		if len(parts) >= 3 {
			if exp, err := time.Parse(time.RFC3339, parts[2]); err == nil {
				tokenExpiry = exp
			}
		}
	} else {
		// Simple PAT or access token
		accessToken = opts.apiKey
	}
	
	client := &githubCopilotClient{
		providerOptions: opts,
		accessToken:     accessToken,
		refreshToken:    refreshToken,
		tokenExpiry:     tokenExpiry,
	}
	
	client.updateClient()
	return client
}

func (g *githubCopilotClient) updateClient() {
	openaiClientOptions := []option.RequestOption{
		option.WithBaseURL(githubCopilotAPIURL),
	}
	
	// Use Bearer token authentication
	if g.accessToken != "" {
		openaiClientOptions = append(openaiClientOptions, 
			option.WithHeader("Authorization", "Bearer "+g.accessToken),
		)
	}
	
	if config.Get().Options.Debug {
		httpClient := log.NewHTTPClient()
		openaiClientOptions = append(openaiClientOptions, option.WithHTTPClient(httpClient))
	}
	
	for key, value := range g.providerOptions.extraHeaders {
		openaiClientOptions = append(openaiClientOptions, option.WithHeader(key, value))
	}
	
	for extraKey, extraValue := range g.providerOptions.extraBody {
		openaiClientOptions = append(openaiClientOptions, option.WithJSONSet(extraKey, extraValue))
	}
	
	g.client = openai.NewClient(openaiClientOptions...)
}

func (g *githubCopilotClient) refreshAccessToken(ctx context.Context) error {
	if g.refreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}
	
	// Make refresh token request to GitHub OAuth
	req, err := http.NewRequestWithContext(ctx, "POST", "https://github.com/login/oauth/access_token", 
		strings.NewReader(fmt.Sprintf("grant_type=refresh_token&refresh_token=%s", g.refreshToken)))
	if err != nil {
		return err
	}
	
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to refresh token: %s", resp.Status)
	}
	
	var tokenResp GitHubOAuthToken
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return err
	}
	
	g.accessToken = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		g.refreshToken = tokenResp.RefreshToken
	}
	if tokenResp.ExpiresIn > 0 {
		g.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}
	
	// Update the client with new token
	g.updateClient()
	
	// Note: The OAuth flow dialog will handle updating the config with the new token
	// This method just refreshes the in-memory client
	
	return nil
}

func (g *githubCopilotClient) ensureValidToken(ctx context.Context) error {
	// Check if token is expired
	if !g.tokenExpiry.IsZero() && time.Now().After(g.tokenExpiry) {
		slog.Info("GitHub Copilot token expired, refreshing...")
		if err := g.refreshAccessToken(ctx); err != nil {
			return fmt.Errorf("failed to refresh token: %w", err)
		}
	}
	return nil
}

// Implement the same methods as openaiClient but with token refresh logic

func (g *githubCopilotClient) convertMessages(messages []message.Message) []openai.ChatCompletionMessageParamUnion {
	// Same implementation as openaiClient
	systemMessage := g.providerOptions.systemMessage
	if g.providerOptions.systemPromptPrefix != "" {
		systemMessage = g.providerOptions.systemPromptPrefix + "\n" + systemMessage
	}
	
	var openaiMessages []openai.ChatCompletionMessageParamUnion
	openaiMessages = append(openaiMessages, openai.SystemMessage(systemMessage))
	
	for _, msg := range messages {
		switch msg.Role {
		case message.User:
			var content []openai.ChatCompletionContentPartUnionParam
			textBlock := openai.ChatCompletionContentPartTextParam{Text: msg.Content().String()}
			content = append(content, openai.ChatCompletionContentPartUnionParam{OfText: &textBlock})
			
			for _, binaryContent := range msg.BinaryContent() {
				imageURL := openai.ChatCompletionContentPartImageImageURLParam{
					URL: binaryContent.String(catwalk.InferenceProviderOpenAI),
				}
				imageBlock := openai.ChatCompletionContentPartImageParam{ImageURL: imageURL}
				content = append(content, openai.ChatCompletionContentPartUnionParam{OfImageURL: &imageBlock})
			}
			
			if len(content) > 1 {
				openaiMessages = append(openaiMessages, openai.UserMessage(content))
			} else {
				openaiMessages = append(openaiMessages, openai.UserMessage(msg.Content().String()))
			}
			
		case message.Assistant:
			assistantMsg := openai.ChatCompletionAssistantMessageParam{
				Role: "assistant",
			}
			
			hasContent := false
			if msg.Content().String() != "" {
				hasContent = true
				assistantMsg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: param.NewOpt(msg.Content().String()),
				}
			}
			
			if len(msg.ToolCalls()) > 0 {
				hasContent = true
				assistantMsg.ToolCalls = make([]openai.ChatCompletionMessageToolCallParam, len(msg.ToolCalls()))
				for i, call := range msg.ToolCalls() {
					assistantMsg.ToolCalls[i] = openai.ChatCompletionMessageToolCallParam{
						ID:   call.ID,
						Type: "function",
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      call.Name,
							Arguments: call.Input,
						},
					}
				}
			}
			
			if hasContent {
				openaiMessages = append(openaiMessages, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &assistantMsg,
				})
			}
			
		case message.Tool:
			for _, result := range msg.ToolResults() {
				openaiMessages = append(openaiMessages,
					openai.ToolMessage(result.Content, result.ToolCallID),
				)
			}
		}
	}
	
	return openaiMessages
}

func (g *githubCopilotClient) convertTools(tools []tools.BaseTool) []openai.ChatCompletionToolParam {
	openaiTools := make([]openai.ChatCompletionToolParam, len(tools))
	
	for i, tool := range tools {
		info := tool.Info()
		openaiTools[i] = openai.ChatCompletionToolParam{
			Function: openai.FunctionDefinitionParam{
				Name:        info.Name,
				Description: openai.String(info.Description),
				Parameters: openai.FunctionParameters{
					"type":       "object",
					"properties": info.Parameters,
					"required":   info.Required,
				},
			},
		}
	}
	
	return openaiTools
}

func (g *githubCopilotClient) finishReason(reason string) message.FinishReason {
	switch reason {
	case "stop":
		return message.FinishReasonEndTurn
	case "length":
		return message.FinishReasonMaxTokens
	case "tool_calls":
		return message.FinishReasonToolUse
	default:
		return message.FinishReasonUnknown
	}
}

func (g *githubCopilotClient) preparedParams(messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) openai.ChatCompletionNewParams {
	model := g.providerOptions.model(g.providerOptions.modelType)
	cfg := config.Get()
	
	modelConfig := cfg.Models[config.SelectedModelTypeLarge]
	if g.providerOptions.modelType == config.SelectedModelTypeSmall {
		modelConfig = cfg.Models[config.SelectedModelTypeSmall]
	}
	
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(model.ID),
		Messages: messages,
		Tools:    tools,
	}
	
	maxTokens := model.DefaultMaxTokens
	if modelConfig.MaxTokens > 0 {
		maxTokens = modelConfig.MaxTokens
	}
	
	if g.providerOptions.maxTokens > 0 {
		maxTokens = g.providerOptions.maxTokens
	}
	
	params.MaxTokens = openai.Int(maxTokens)
	
	return params
}

func (g *githubCopilotClient) send(ctx context.Context, messages []message.Message, tools []tools.BaseTool) (*ProviderResponse, error) {
	// Ensure token is valid before making request
	if err := g.ensureValidToken(ctx); err != nil {
		return nil, err
	}
	
	params := g.preparedParams(g.convertMessages(messages), g.convertTools(tools))
	
	attempts := 0
	for {
		attempts++
		openaiResponse, err := g.client.Chat.Completions.New(ctx, params)
		
		if err != nil {
			retry, after, retryErr := g.shouldRetry(attempts, err)
			if retryErr != nil {
				return nil, retryErr
			}
			if retry {
				slog.Warn("Retrying due to rate limit", "attempt", attempts, "max_retries", maxRetries)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(time.Duration(after) * time.Millisecond):
					continue
				}
			}
			return nil, retryErr
		}
		
		if len(openaiResponse.Choices) == 0 {
			return nil, fmt.Errorf("received empty response from GitHub Copilot API")
		}
		
		content := ""
		if openaiResponse.Choices[0].Message.Content != "" {
			content = openaiResponse.Choices[0].Message.Content
		}
		
		toolCalls := g.toolCalls(*openaiResponse)
		finishReason := g.finishReason(string(openaiResponse.Choices[0].FinishReason))
		
		if len(toolCalls) > 0 {
			finishReason = message.FinishReasonToolUse
		}
		
		return &ProviderResponse{
			Content:      content,
			ToolCalls:    toolCalls,
			Usage:        g.usage(*openaiResponse),
			FinishReason: finishReason,
		}, nil
	}
}

func (g *githubCopilotClient) stream(ctx context.Context, messages []message.Message, tools []tools.BaseTool) <-chan ProviderEvent {
	// Ensure token is valid before making request
	if err := g.ensureValidToken(ctx); err != nil {
		eventChan := make(chan ProviderEvent, 1)
		eventChan <- ProviderEvent{Type: EventError, Error: err}
		close(eventChan)
		return eventChan
	}
	
	params := g.preparedParams(g.convertMessages(messages), g.convertTools(tools))
	params.StreamOptions = openai.ChatCompletionStreamOptionsParam{
		IncludeUsage: openai.Bool(true),
	}
	
	attempts := 0
	eventChan := make(chan ProviderEvent)
	
	go func() {
		for {
			attempts++
			if len(params.Tools) == 0 {
				params.Tools = nil
			}
			
			openaiStream := g.client.Chat.Completions.NewStreaming(ctx, params)
			
			acc := openai.ChatCompletionAccumulator{}
			currentContent := ""
			toolCalls := make([]message.ToolCall, 0)
			var msgToolCalls []openai.ChatCompletionMessageToolCall
			
			for openaiStream.Next() {
				chunk := openaiStream.Current()
				acc.AddChunk(chunk)
				
				for _, choice := range chunk.Choices {
					if choice.Delta.Content != "" {
						eventChan <- ProviderEvent{
							Type:    EventContentDelta,
							Content: choice.Delta.Content,
						}
						currentContent += choice.Delta.Content
					} else if len(choice.Delta.ToolCalls) > 0 {
						toolCall := choice.Delta.ToolCalls[0]
						if len(msgToolCalls)-1 < int(toolCall.Index) {
							if toolCall.ID == "" {
								toolCall.ID = uuid.NewString()
							}
							eventChan <- ProviderEvent{
								Type: EventToolUseStart,
								ToolCall: &message.ToolCall{
									ID:       toolCall.ID,
									Name:     toolCall.Function.Name,
									Finished: false,
								},
							}
							msgToolCalls = append(msgToolCalls, openai.ChatCompletionMessageToolCall{
								ID:   toolCall.ID,
								Type: "function",
								Function: openai.ChatCompletionMessageToolCallFunction{
									Name:      toolCall.Function.Name,
									Arguments: toolCall.Function.Arguments,
								},
							})
						} else {
							msgToolCalls[toolCall.Index].Function.Arguments += toolCall.Function.Arguments
						}
					}
				}
			}
			
			err := openaiStream.Err()
			if err == nil || errors.Is(err, io.EOF) {
				if len(acc.Choices) == 0 {
					eventChan <- ProviderEvent{
						Type:  EventError,
						Error: fmt.Errorf("received empty streaming response from GitHub Copilot API"),
					}
					return
				}
				
				finishReason := g.finishReason(string(acc.Choices[0].FinishReason))
				if len(acc.Choices[0].Message.ToolCalls) > 0 {
					toolCalls = append(toolCalls, g.toolCalls(acc.ChatCompletion)...)
				}
				if len(toolCalls) > 0 {
					finishReason = message.FinishReasonToolUse
				}
				
				eventChan <- ProviderEvent{
					Type: EventComplete,
					Response: &ProviderResponse{
						Content:      currentContent,
						ToolCalls:    toolCalls,
						Usage:        g.usage(acc.ChatCompletion),
						FinishReason: finishReason,
					},
				}
				close(eventChan)
				return
			}
			
			retry, after, retryErr := g.shouldRetry(attempts, err)
			if retryErr != nil {
				eventChan <- ProviderEvent{Type: EventError, Error: retryErr}
				close(eventChan)
				return
			}
			if retry {
				slog.Warn("Retrying due to rate limit", "attempt", attempts, "max_retries", maxRetries)
				select {
				case <-ctx.Done():
					if ctx.Err() != nil {
						eventChan <- ProviderEvent{Type: EventError, Error: ctx.Err()}
					}
					close(eventChan)
					return
				case <-time.After(time.Duration(after) * time.Millisecond):
					continue
				}
			}
			eventChan <- ProviderEvent{Type: EventError, Error: retryErr}
			close(eventChan)
			return
		}
	}()
	
	return eventChan
}

func (g *githubCopilotClient) shouldRetry(attempts int, err error) (bool, int64, error) {
	if attempts > maxRetries {
		return false, 0, fmt.Errorf("maximum retry attempts reached: %d retries", maxRetries)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false, 0, err
	}
	
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		// Check for token expiration (401 Unauthorized)
		if apiErr.StatusCode == 401 {
			// Try to refresh token
			if err := g.refreshAccessToken(context.Background()); err != nil {
				return false, 0, fmt.Errorf("authentication failed: %w", err)
			}
			return true, 0, nil
		}
		
		if apiErr.StatusCode != 429 && apiErr.StatusCode != 500 {
			return false, 0, err
		}
		
		// Handle rate limiting
		retryAfterValues := apiErr.Response.Header.Values("Retry-After")
		retryMs := 2000 * (1 << (attempts - 1))
		if len(retryAfterValues) > 0 {
			if ms, err := fmt.Sscanf(retryAfterValues[0], "%d", &retryMs); err == nil && ms > 0 {
				retryMs = ms * 1000
			}
		}
		return true, int64(retryMs), nil
	}
	
	return false, 0, err
}

func (g *githubCopilotClient) toolCalls(completion openai.ChatCompletion) []message.ToolCall {
	var toolCalls []message.ToolCall
	
	if len(completion.Choices) > 0 && len(completion.Choices[0].Message.ToolCalls) > 0 {
		for _, call := range completion.Choices[0].Message.ToolCalls {
			toolCall := message.ToolCall{
				ID:       call.ID,
				Name:     call.Function.Name,
				Input:    call.Function.Arguments,
				Type:     "function",
				Finished: true,
			}
			toolCalls = append(toolCalls, toolCall)
		}
	}
	
	return toolCalls
}

func (g *githubCopilotClient) usage(completion openai.ChatCompletion) TokenUsage {
	return TokenUsage{
		InputTokens:  completion.Usage.PromptTokens,
		OutputTokens: completion.Usage.CompletionTokens,
	}
}

func (g *githubCopilotClient) Model() catwalk.Model {
	return g.providerOptions.model(g.providerOptions.modelType)
}
