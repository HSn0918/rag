// Package openai provides a client for OpenAI-compatible API operations.
// It supports chat completions with various model configurations and parameters.
package openai

import (
	"time"

	"github.com/hsn0918/rag/internal/clients/base"
	"github.com/hsn0918/rag/internal/config"
)

// Default configuration constants
const (
	DefaultTimeout     = 60 * time.Second
	DefaultMaxTokens   = 4096
	DefaultTemperature = 0.7
	DefaultTopP        = 0.7
	ServiceName        = "openai"
)

// ChatCompleter defines the interface for chat completion operations.
type ChatCompleter interface {
	CreateChatCompletion(req ChatRequest) (*ChatResponse, error)
	CreateChatCompletionWithDefaults(model string, messages []Message) (*ChatResponse, error)
}

// Client provides OpenAI API operations using standardized base client.
// It handles chat completions and maintains service configuration.
type Client struct {
	httpClient *base.HTTPClient
	config     config.ServiceConfig
}

// Compile-time check to ensure Client implements ChatCompleter interface
var _ ChatCompleter = (*Client)(nil)

// NewClient creates a new OpenAI client with standardized configuration.
// It uses the base HTTP client for consistent error handling and retry logic.
func NewClient(cfg config.ServiceConfig) *Client {
	httpClient := base.NewHTTPClient(ServiceName, cfg, DefaultTimeout)

	return &Client{
		httpClient: httpClient,
		config:     cfg,
	}
}

// Message represents a single chat message with role and content.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Tool represents a function tool that can be called by the model.
type Tool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string      `json:"name"`
		Description string      `json:"description"`
		Parameters  interface{} `json:"parameters"`
	} `json:"function"`
}

// ResponseFormat defines the format constraints for model responses.
type ResponseFormat struct {
	Type   string      `json:"type"`
	Schema interface{} `json:"schema,omitempty"`
}

// ChatRequest represents a chat completion request with all parameters.
type ChatRequest struct {
	// Required fields
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`

	// Optional behavior settings
	Stream         bool            `json:"stream,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	EnableThinking bool            `json:"enable_thinking,omitempty"`
	ThinkingBudget int             `json:"thinking_budget,omitempty"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
	Tools          []Tool          `json:"tools,omitempty"`

	// Sampling parameters
	MinP             float64     `json:"min_p,omitempty"`
	Stop             interface{} `json:"stop,omitempty"`
	Temperature      float64     `json:"temperature,omitempty"`
	TopP             float64     `json:"top_p,omitempty"`
	TopK             float64     `json:"top_k,omitempty"`
	FrequencyPenalty float64     `json:"frequency_penalty,omitempty"`
	N                int         `json:"n,omitempty"`
}

// Choice represents a single completion choice from the model.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage represents token usage information for the request.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatResponse represents the complete chat completion API response.
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// CreateChatCompletion generates a chat completion for the given request.
// It returns the complete response with choices and usage information.
func (c *Client) CreateChatCompletion(req ChatRequest) (*ChatResponse, error) {
	var result ChatResponse
	if err := c.httpClient.Post("/chat/completions", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateChatCompletionWithDefaults creates a chat completion with sensible defaults.
// It uses standard parameters for temperature, tokens, and streaming settings.
func (c *Client) CreateChatCompletionWithDefaults(model string, messages []Message) (*ChatResponse, error) {
	req := ChatRequest{
		Model:       model,
		Messages:    messages,
		Stream:      false,
		MaxTokens:   DefaultMaxTokens,
		Temperature: DefaultTemperature,
		TopP:        DefaultTopP,
	}

	return c.CreateChatCompletion(req)
}
