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

// Clients 包含所有外部服务客户端
type Clients struct {
	Doc2X     *doc2x.Client
	Embedding *embedding.Client
	LLM       *openai.Client
	Reranker  *rerank.Client
	Storage   *storage.MinIOClient
}

// NewClients 根据配置创建所有客户端
func NewClients(cfg *config.Config) (*Clients, error) {
	// 创建 MinIO 客户端
	minioClient, err := storage.NewMinIOClient(storage.MinIOConfig{
		Endpoint:        cfg.MinIO.Endpoint,
		AccessKeyID:     cfg.MinIO.AccessKeyID,
		SecretAccessKey: cfg.MinIO.SecretAccessKey,
		BucketName:      cfg.MinIO.BucketName,
		UseSSL:          cfg.MinIO.UseSSL,
	})
	if err != nil {
		return nil, err
	}

	return &Clients{
		Doc2X:     doc2x.NewClient(cfg.Services.Doc2X),
		Embedding: embedding.NewClient(cfg.Services.Embedding.ServiceConfig),
		LLM:       openai.NewClient(cfg.Services.LLM),
		Reranker:  rerank.NewClient(cfg.Services.Reranker),
		Storage:   minioClient,
	}, nil
}

// NewRagServerWithClients 使用预配置的客户端创建 RagServer
func NewRagServerWithClients(
	db adapters.VectorDB,
	cache *redis.CacheService,
	clients *Clients,
	cfg config.Config,
) (*RagServer, error) {
	server := &RagServer{
		DB:              db,
		Cache:           cache,
		Storage:         clients.Storage,
		Doc2X:           clients.Doc2X,
		Embedding:       clients.Embedding,
		LLM:             clients.LLM,
		Reranker:        clients.Reranker,
		Config:          &cfg,
		EmbeddingClient: clients.Embedding,
	}

	// Initialize SearchOptimizer with proper configuration
	searchOptimizer, err := NewSearchOptimizer(
		server,
		20, // Initial candidates
		5,  // Final results
		WithMinSimilarity(0.25),
		WithParallelScoring(true),
	)
	if err != nil {
		return nil, err
	}

	server.SearchOptimizer = searchOptimizer
	return server, nil
}
