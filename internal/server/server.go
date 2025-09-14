package server

import (
	"github.com/hsn0918/rag/internal/adapters"
	"github.com/hsn0918/rag/internal/clients/doc2x"
	"github.com/hsn0918/rag/internal/clients/embedding"
	"github.com/hsn0918/rag/internal/clients/openai"
	"github.com/hsn0918/rag/internal/clients/rerank"
	"github.com/hsn0918/rag/internal/config"
	"github.com/hsn0918/rag/internal/prompts"
	"github.com/hsn0918/rag/internal/redis"
	"github.com/hsn0918/rag/internal/storage"
)

// RagServer implements the RAG service with all necessary dependencies.
type RagServer struct {
	DB                     adapters.VectorDB
	Cache                  *redis.CacheService
	Storage                *storage.MinIOClient
	Doc2X                  *doc2x.Client
	Embedding              *embedding.Client
	EmbeddingClient        *embedding.Client
	LLM                    *openai.Client
	Reranker               *rerank.Client
	Config                 *config.Config
	SearchOptimizer        *SearchOptimizer
	promptEmbeddingService *prompts.PromptEmbeddingService
}
