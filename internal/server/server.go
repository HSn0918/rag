package server

import (
	"github.com/hsn0918/rag/internal/adapters"
	"github.com/hsn0918/rag/internal/clients/doc2x"
	"github.com/hsn0918/rag/internal/clients/embedding"
	"github.com/hsn0918/rag/internal/clients/openai"
	"github.com/hsn0918/rag/internal/clients/rerank"
	"github.com/hsn0918/rag/internal/config"
	"github.com/hsn0918/rag/internal/redis"
	"github.com/hsn0918/rag/internal/storage"
)

// RagServer 实现了 ragv1connect.RagServiceHandler 接口。
// 它包含了服务所需的所有依赖项。
type RagServer struct {
	DB        adapters.VectorDB
	Cache     *redis.CacheService
	Storage   *storage.MinIOClient
	Doc2X     *doc2x.Client
	Embedding *embedding.Client
	LLM       *openai.Client
	Reranker  *rerank.Client
	Config    config.Config
}

// NewRagServer 是 RagServer 的构造函数。
func NewRagServer(
	db adapters.VectorDB,
	cache *redis.CacheService,
	storageClient *storage.MinIOClient,
	doc2xClient *doc2x.Client,
	embeddingClient *embedding.Client,
	llmClient *openai.Client,
	rerankClient *rerank.Client,
	cfg config.Config,
) *RagServer {
	return &RagServer{
		DB:        db,
		Cache:     cache,
		Storage:   storageClient,
		Doc2X:     doc2xClient,
		Embedding: embeddingClient,
		LLM:       llmClient,
		Reranker:  rerankClient,
		Config:    cfg,
	}
}
