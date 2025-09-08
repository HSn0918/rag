package openai

import (
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/hsn0918/rag/internal/config"
)

type Client struct {
	client *resty.Client
	config config.ServiceConfig
}

func NewClient(cfg config.ServiceConfig) *Client {
	client := resty.New().
		SetBaseURL(cfg.BaseURL).
		SetHeader("Authorization", "Bearer "+cfg.APIKey).
		SetTimeout(60 * time.Second)

	return &Client{
		client: client,
		config: cfg,
	}
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
	MinP             float64         `json:"min_p,omitempty"`
	Stop             interface{}     `json:"stop,omitempty"`
	Temperature      float64         `json:"temperature,omitempty"`
	TopP             float64         `json:"top_p,omitempty"`
	TopK             float64         `json:"top_k,omitempty"`
	FrequencyPenalty float64         `json:"frequency_penalty,omitempty"`
	N                int             `json:"n,omitempty"`
	ResponseFormat   *ResponseFormat `json:"response_format,omitempty"`
	Tools            []Tool          `json:"tools,omitempty"`
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
	resp, err := c.client.R().
		SetBody(req).
		SetResult(&result).
		Post("/chat/completions")

	if err != nil {
		return nil, fmt.Errorf("create chat completion failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("create chat completion failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

func (c *Client) CreateChatCompletionWithDefaults(model string, messages []Message) (*ChatResponse, error) {
	req := ChatRequest{
		Model:       model,
		Messages:    messages,
		Stream:      false,
		MaxTokens:   4096,
		Temperature: 0.7,
		TopP:        0.7,
	}

	return c.CreateChatCompletion(req)
}
