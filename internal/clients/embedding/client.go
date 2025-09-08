package embedding

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

type EmbeddingRequest struct {
	Model          string      `json:"model"`
	Input          interface{} `json:"input"`
	EncodingFormat string      `json:"encoding_format,omitempty"`
	Dimensions     int         `json:"dimensions,omitempty"`
}

type EmbeddingData struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type EmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type EmbeddingResponse struct {
	Object string          `json:"object"`
	Model  string          `json:"model"`
	Data   []EmbeddingData `json:"data"`
	Usage  EmbeddingUsage  `json:"usage"`
}

func (c *Client) CreateEmbedding(req EmbeddingRequest) (*EmbeddingResponse, error) {
	var result EmbeddingResponse
	resp, err := c.client.R().
		SetBody(req).
		SetResult(&result).
		Post("/embeddings")

	if err != nil {
		return nil, fmt.Errorf("create embedding failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("create embedding failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

func (c *Client) CreateEmbeddingWithDefaults(model, text string) (*EmbeddingResponse, error) {
	req := EmbeddingRequest{
		Model:          model,
		Input:          text,
		EncodingFormat: "float",
	}

	return c.CreateEmbedding(req)
}

func (c *Client) CreateBatchEmbedding(model string, texts []string) (*EmbeddingResponse, error) {
	req := EmbeddingRequest{
		Model:          model,
		Input:          texts,
		EncodingFormat: "float",
	}

	return c.CreateEmbedding(req)
}

const (
	ModelBGELargeZhV15      = "BAAI/bge-large-zh-v1.5"
	ModelBGELargeEnV15      = "BAAI/bge-large-en-v1.5"
	ModelBCEEmbeddingBaseV1 = "netease-youdao/bce-embedding-base_v1"
	ModelBGEM3              = "BAAI/bge-m3"
	ModelProBGEM3           = "Pro/BAAI/bge-m3"
	ModelQwen3Embedding8B   = "Qwen/Qwen3-Embedding-8B"
	ModelQwen3Embedding4B   = "Qwen/Qwen3-Embedding-4B"
	ModelQwen3Embedding06B  = "Qwen/Qwen3-Embedding-0.6B"
)

const (
	MaxTokensBGELarge = 512
	MaxTokensBGEM3    = 8192
	MaxTokensQwen3    = 32768
)

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

func GetDefaultDimensions(model string) int {
	switch model {
	case ModelQwen3Embedding8B:
		return 4096 // 默认最大维度
	case ModelQwen3Embedding4B:
		return 2048 // 默认最大维度
	case ModelQwen3Embedding06B:
		return 1024 // 默认最大维度
	case ModelBGELargeZhV15, ModelBGELargeEnV15:
		return 1024 // BGE-Large 系列
	case ModelBCEEmbeddingBaseV1:
		return 768 // BCE 系列
	case ModelBGEM3, ModelProBGEM3:
		return 1024 // BGE-M3 系列
	default:
		return 1536 // 默认回退维度
	}
}
