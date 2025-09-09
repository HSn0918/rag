// Package rerank provides a client for reranking service operations.
// It supports document reranking with various model configurations.
package rerank

import (
	"time"

	"github.com/hsn0918/rag/internal/clients/base"
	"github.com/hsn0918/rag/internal/config"
)

// Default configuration constants
const (
	DefaultTimeout = 30 * time.Second
	ServiceName    = "rerank"
)

// Reranker defines the interface for reranking operations.
type Reranker interface {
	CreateRerank(req Request) (*Response, error)
	CreateRerankWithDefaults(model, query string, documents []string, topN int) (*Response, error)
}

// Client provides reranking API operations using standardized base client.
// It handles document reranking and maintains service configuration.
type Client struct {
	httpClient *base.HTTPClient
	config     config.ServiceConfig
}

// Compile-time check to ensure Client implements Reranker interface
var _ Reranker = (*Client)(nil)

// NewClient creates a new reranking client with standardized configuration.
// It uses the base HTTP client for consistent error handling and retry logic.
func NewClient(cfg config.ServiceConfig) *Client {
	httpClient := base.NewHTTPClient(ServiceName, cfg, DefaultTimeout)

	return &Client{
		httpClient: httpClient,
		config:     cfg,
	}
}

// Request represents a document reranking request.
type Request struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	Instruction     string   `json:"instruction,omitempty"`
	TopN            int      `json:"top_n,omitempty"`
	ReturnDocuments bool     `json:"return_documents,omitempty"`
	MaxChunksPerDoc int      `json:"max_chunks_per_doc,omitempty"`
	OverlapTokens   int      `json:"overlap_tokens,omitempty"`
}

// Result represents a single reranking result.
type Result struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
	Document       *string `json:"document,omitempty"`
}

// Response represents the complete reranking API response.
type Response struct {
	ID      string   `json:"id"`
	Results []Result `json:"results"`
}

// CreateRerank performs document reranking based on query relevance.
// It returns reranked documents with relevance scores.
func (c *Client) CreateRerank(req Request) (*Response, error) {
	var result Response
	if err := c.httpClient.Post("/rerank", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateRerankWithDefaults creates a rerank request with sensible defaults.
// It enables document return and uses the specified parameters.
func (c *Client) CreateRerankWithDefaults(model, query string, documents []string, topN int) (*Response, error) {
	req := Request{
		Model:           model,
		Query:           query,
		Documents:       documents,
		TopN:            topN,
		ReturnDocuments: true,
	}

	return c.CreateRerank(req)
}

// Supported reranking models organized by provider
const (
	// Qwen reranker models
	ModelQwen3Reranker8B  = "Qwen/Qwen3-Reranker-8B"
	ModelQwen3Reranker4B  = "Qwen/Qwen3-Reranker-4B"
	ModelQwen3Reranker06B = "Qwen/Qwen3-Reranker-0.6B"

	// BGE reranker models
	ModelBGERerankerV2M3    = "BAAI/bge-reranker-v2-m3"
	ModelProBGERerankerV2M3 = "Pro/BAAI/bge-reranker-v2-m3"

	// BCE reranker models
	ModelBCERerankerBaseV1 = "netease-youdao/bce-reranker-base_v1"
)

// Configuration limits
const (
	MaxOverlapTokens = 80
)

// SupportsInstruction reports whether the model supports custom instructions.
// Qwen models support instruction-based reranking.
func SupportsInstruction(model string) bool {
	switch model {
	case ModelQwen3Reranker8B, ModelQwen3Reranker4B, ModelQwen3Reranker06B:
		return true
	default:
		return false
	}
}

// SupportsChunking reports whether the model supports document chunking.
// BGE and BCE models support automatic document chunking.
func SupportsChunking(model string) bool {
	switch model {
	case ModelBGERerankerV2M3, ModelProBGERerankerV2M3, ModelBCERerankerBaseV1:
		return true
	default:
		return false
	}
}
