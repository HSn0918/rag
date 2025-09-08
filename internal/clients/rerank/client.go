package rerank

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
		SetTimeout(30 * time.Second)

	return &Client{
		client: client,
		config: cfg,
	}
}

type RerankRequest struct {
	Model              string   `json:"model"`
	Query              string   `json:"query"`
	Documents          []string `json:"documents"`
	Instruction        string   `json:"instruction,omitempty"`
	TopN               int      `json:"top_n,omitempty"`
	ReturnDocuments    bool     `json:"return_documents,omitempty"`
	MaxChunksPerDoc    int      `json:"max_chunks_per_doc,omitempty"`
	OverlapTokens      int      `json:"overlap_tokens,omitempty"`
}

type RerankResult struct {
	Index           int     `json:"index"`
	RelevanceScore  float64 `json:"relevance_score"`
	Document        *string `json:"document,omitempty"`
}

type RerankResponse struct {
	ID      string         `json:"id"`
	Results []RerankResult `json:"results"`
}

func (c *Client) CreateRerank(req RerankRequest) (*RerankResponse, error) {
	var result RerankResponse
	resp, err := c.client.R().
		SetBody(req).
		SetResult(&result).
		Post("/rerank")

	if err != nil {
		return nil, fmt.Errorf("create rerank failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("create rerank failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

func (c *Client) CreateRerankWithDefaults(model, query string, documents []string, topN int) (*RerankResponse, error) {
	req := RerankRequest{
		Model:           model,
		Query:           query,
		Documents:       documents,
		TopN:            topN,
		ReturnDocuments: true,
	}

	return c.CreateRerank(req)
}

const (
	ModelQwen3Reranker8B       = "Qwen/Qwen3-Reranker-8B"
	ModelQwen3Reranker4B       = "Qwen/Qwen3-Reranker-4B"
	ModelQwen3Reranker06B      = "Qwen/Qwen3-Reranker-0.6B"
	ModelBGERerankerV2M3       = "BAAI/bge-reranker-v2-m3"
	ModelProBGERerankerV2M3    = "Pro/BAAI/bge-reranker-v2-m3"
	ModelBCERerankerBaseV1     = "netease-youdao/bce-reranker-base_v1"
)

func SupportsInstruction(model string) bool {
	switch model {
	case ModelQwen3Reranker8B, ModelQwen3Reranker4B, ModelQwen3Reranker06B:
		return true
	default:
		return false
	}
}

func SupportsChunking(model string) bool {
	switch model {
	case ModelBGERerankerV2M3, ModelProBGERerankerV2M3, ModelBCERerankerBaseV1:
		return true
	default:
		return false
	}
}

const (
	MaxOverlapTokens = 80
)
