package openai

import (
	"time"

	"github.com/hsn0918/rag/pkg/clients/base"
	"github.com/hsn0918/rag/pkg/config"
)

const (
	DefaultTimeout     = 60 * time.Second
	DefaultMaxTokens   = 4096
	DefaultTemperature = 0.7
	DefaultTopP        = 0.7
	ServiceName        = "openai"
)

type ChatCompleter interface {
	CreateChatCompletion(req ChatRequest) (*ChatResponse, error)
	CreateChatCompletionWithDefaults(model string, messages []Message) (*ChatResponse, error)
}

type Client struct {
	httpClient *base.HTTPClient
	config     config.ServiceConfig
}

var _ ChatCompleter = (*Client)(nil)

func NewClient(cfg config.ServiceConfig) *Client {
	httpClient := base.NewHTTPClient(ServiceName, cfg, DefaultTimeout)
	return &Client{httpClient: httpClient, config: cfg}
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Tool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string      `json:"name"`
		Description string      `json:"description"`
		Parameters  interface{} `json:"parameters"`
	} `json:"function"`
}

type ResponseFormat struct {
	Type   string      `json:"type"`
	Schema interface{} `json:"schema,omitempty"`
}

type ChatRequest struct {
	Model            string          `json:"model"`
	Messages         []Message       `json:"messages"`
	Stream           bool            `json:"stream,omitempty"`
	MaxTokens        int             `json:"max_tokens,omitempty"`
	EnableThinking   bool            `json:"enable_thinking,omitempty"`
	ThinkingBudget   int             `json:"thinking_budget,omitempty"`
	ResponseFormat   *ResponseFormat `json:"response_format,omitempty"`
	Tools            []Tool          `json:"tools,omitempty"`
	MinP             float64         `json:"min_p,omitempty"`
	Stop             interface{}     `json:"stop,omitempty"`
	Temperature      float64         `json:"temperature,omitempty"`
	TopP             float64         `json:"top_p,omitempty"`
	TopK             float64         `json:"top_k,omitempty"`
	FrequencyPenalty float64         `json:"frequency_penalty,omitempty"`
	N                int             `json:"n,omitempty"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

func (c *Client) CreateChatCompletion(req ChatRequest) (*ChatResponse, error) {
	var result ChatResponse
	if err := c.httpClient.Post("/chat/completions", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CreateChatCompletionWithDefaults(model string, messages []Message) (*ChatResponse, error) {
	req := ChatRequest{Model: model, Messages: messages, Stream: false, MaxTokens: DefaultMaxTokens, Temperature: DefaultTemperature, TopP: DefaultTopP}
	return c.CreateChatCompletion(req)
}
