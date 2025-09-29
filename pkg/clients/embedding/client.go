package embedding

import (
	"time"

	"github.com/hsn0918/rag/pkg/clients/base"
	"github.com/hsn0918/rag/pkg/config"
)

const (
	DefaultTimeout = 30 * time.Second
	ServiceName    = "embedding"
)

type Embedder interface {
	CreateEmbedding(req Request) (*Response, error)
	CreateEmbeddingWithDefaults(model, text string) (*Response, error)
	CreateBatchEmbedding(model string, texts []string) (*Response, error)
}

type Client struct {
	httpClient *base.HTTPClient
	config     config.ServiceConfig
}

var _ Embedder = (*Client)(nil)

func NewClient(cfg config.ServiceConfig) *Client {
	httpClient := base.NewHTTPClient(ServiceName, cfg, DefaultTimeout)
	return &Client{httpClient: httpClient, config: cfg}
}

type Request struct {
	Model          string      `json:"model"`
	Input          interface{} `json:"input"`
	EncodingFormat string      `json:"encoding_format,omitempty"`
	Dimensions     int         `json:"dimensions,omitempty"`
}
type Data struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}
type Usage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}
type Response struct {
	Object string `json:"object"`
	Model  string `json:"model"`
	Data   []Data `json:"data"`
	Usage  Usage  `json:"usage"`
}

func (c *Client) CreateEmbedding(req Request) (*Response, error) {
	var result Response
	if err := c.httpClient.Post("/embeddings", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CreateEmbeddingWithDefaults(model, text string) (*Response, error) {
	req := Request{Model: model, Input: text, EncodingFormat: "float"}
	return c.CreateEmbedding(req)
}

func (c *Client) CreateBatchEmbedding(model string, texts []string) (*Response, error) {
	req := Request{Model: model, Input: texts, EncodingFormat: "float"}
	return c.CreateEmbedding(req)
}

const (
	ModelBGELargeZhV15      = "BAAI/bge-large-zh-v1.5"
	ModelBGELargeEnV15      = "BAAI/bge-large-en-v1.5"
	ModelBGEM3              = "BAAI/bge-m3"
	ModelProBGEM3           = "Pro/BAAI/bge-m3"
	ModelBCEEmbeddingBaseV1 = "netease-youdao/bce-embedding-base_v1"
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
		return 4096
	case ModelQwen3Embedding4B:
		return 2048
	case ModelQwen3Embedding06B:
		return 1024
	case ModelBGELargeZhV15, ModelBGELargeEnV15:
		return 1024
	case ModelBCEEmbeddingBaseV1:
		return 768
	case ModelBGEM3, ModelProBGEM3:
		return 1024
	default:
		return 1536
	}
}
