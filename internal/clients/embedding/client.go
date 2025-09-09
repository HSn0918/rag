// Package embedding provides a client for embedding service operations.
// It supports multiple embedding models and handles batch operations efficiently.
package embedding

import (
	"time"

	"github.com/hsn0918/rag/internal/clients/base"
	"github.com/hsn0918/rag/internal/config"
)

// Default configuration constants
const (
	DefaultTimeout = 30 * time.Second
	ServiceName    = "embedding"
)

// Embedder defines the interface for embedding operations.
type Embedder interface {
	CreateEmbedding(req Request) (*Response, error)
	CreateEmbeddingWithDefaults(model, text string) (*Response, error)
	CreateBatchEmbedding(model string, texts []string) (*Response, error)
}

// Client provides embedding API operations using standardized base client.
// It handles text embedding generation with various model configurations.
type Client struct {
	httpClient *base.HTTPClient
	config     config.ServiceConfig
}

// Compile-time check to ensure Client implements Embedder interface
var _ Embedder = (*Client)(nil)

// NewClient creates a new embedding client with standardized configuration.
// It uses the base HTTP client for consistent error handling and retry logic.
func NewClient(cfg config.ServiceConfig) *Client {
	httpClient := base.NewHTTPClient(ServiceName, cfg, DefaultTimeout)

	return &Client{
		httpClient: httpClient,
		config:     cfg,
	}
}

// Request represents an embedding generation request.
type Request struct {
	Model          string      `json:"model"`
	Input          interface{} `json:"input"`
	EncodingFormat string      `json:"encoding_format,omitempty"`
	Dimensions     int         `json:"dimensions,omitempty"`
}

// Data represents a single embedding result.
type Data struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

// Usage represents token usage information.
type Usage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// Response represents the complete embedding API response.
type Response struct {
	Object string `json:"object"`
	Model  string `json:"model"`
	Data   []Data `json:"data"`
	Usage  Usage  `json:"usage"`
}

// CreateEmbedding generates embeddings for the given request.
// It returns the complete embedding response with usage information.
func (c *Client) CreateEmbedding(req Request) (*Response, error) {
	var result Response
	if err := c.httpClient.Post("/embeddings", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateEmbeddingWithDefaults generates embeddings with standard settings.
// It uses default encoding format and model-specific configurations.
func (c *Client) CreateEmbeddingWithDefaults(model, text string) (*Response, error) {
	req := Request{
		Model:          model,
		Input:          text,
		EncodingFormat: "float",
	}

	return c.CreateEmbedding(req)
}

// CreateBatchEmbedding generates embeddings for multiple texts efficiently.
// It processes all texts in a single API call for better performance.
func (c *Client) CreateBatchEmbedding(model string, texts []string) (*Response, error) {
	req := Request{
		Model:          model,
		Input:          texts,
		EncodingFormat: "float",
	}

	return c.CreateEmbedding(req)
}

// Supported embedding models organized by provider
const (
	// BGE models - Bilingual General Embedding
	ModelBGELargeZhV15 = "BAAI/bge-large-zh-v1.5"
	ModelBGELargeEnV15 = "BAAI/bge-large-en-v1.5"
	ModelBGEM3         = "BAAI/bge-m3"
	ModelProBGEM3      = "Pro/BAAI/bge-m3"

	// BCE models - Bilingual Contextual Embedding
	ModelBCEEmbeddingBaseV1 = "netease-youdao/bce-embedding-base_v1"

	// Qwen models - Qwen embedding series
	ModelQwen3Embedding8B  = "Qwen/Qwen3-Embedding-8B"
	ModelQwen3Embedding4B  = "Qwen/Qwen3-Embedding-4B"
	ModelQwen3Embedding06B = "Qwen/Qwen3-Embedding-0.6B"
)

// Model token limits for context window management
const (
	MaxTokensBGELarge = 512
	MaxTokensBGEM3    = 8192
	MaxTokensQwen3    = 32768
)

// GetMaxTokens returns the maximum token limit for the specified model.
// This helps with input text chunking and validation.
func GetMaxTokens(model string) int {
	switch model {
	case ModelBGELargeZhV15, ModelBGELargeEnV15, ModelBCEEmbeddingBaseV1:
		return MaxTokensBGELarge
	case ModelBGEM3, ModelProBGEM3:
		return MaxTokensBGEM3
	case ModelQwen3Embedding8B, ModelQwen3Embedding4B, ModelQwen3Embedding06B:
		return MaxTokensQwen3
	default:
		return MaxTokensBGELarge
	}
}

// GetSupportedDimensions returns available embedding dimensions for the model.
// Returns nil if the model doesn't support custom dimensions.
func GetSupportedDimensions(model string) []int {
	switch model {
	case ModelQwen3Embedding8B:
		return []int{64, 128, 256, 512, 768, 1024, 2048, 4096}
	case ModelQwen3Embedding4B:
		return []int{64, 128, 256, 512, 768, 1024, 2048}
	case ModelQwen3Embedding06B:
		return []int{64, 128, 256, 512, 768, 1024}
	default:
		return nil
	}
}

// GetDefaultDimensions returns the default embedding dimension for the model.
// This is typically the highest quality dimension setting available.
func GetDefaultDimensions(model string) int {
	switch model {
	case ModelQwen3Embedding8B:
		return 4096 // Maximum dimension for best quality
	case ModelQwen3Embedding4B:
		return 2048 // Maximum dimension for best quality
	case ModelQwen3Embedding06B:
		return 1024 // Maximum dimension for best quality
	case ModelBGELargeZhV15, ModelBGELargeEnV15:
		return 1024 // Standard BGE-Large dimension
	case ModelBCEEmbeddingBaseV1:
		return 768 // Standard BCE dimension
	case ModelBGEM3, ModelProBGEM3:
		return 1024 // Standard BGE-M3 dimension
	default:
		return 1536 // Conservative fallback dimension
	}
}
