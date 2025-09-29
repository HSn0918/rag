package server

import (
	"github.com/hsn0918/rag/internal/adapters"
	pkgdoc2x "github.com/hsn0918/rag/pkg/clients/doc2x"
	pkgembedding "github.com/hsn0918/rag/pkg/clients/embedding"
	pkgopenai "github.com/hsn0918/rag/pkg/clients/openai"
	pkgrerank "github.com/hsn0918/rag/pkg/clients/rerank"
	"github.com/hsn0918/rag/pkg/config"
	"github.com/hsn0918/rag/pkg/prompts"
	"github.com/hsn0918/rag/pkg/redis"
	"github.com/hsn0918/rag/pkg/storage"
)

// ExternalClients 外部服务客户端集合
type ExternalClients struct {
	Doc2X     *pkgdoc2x.Client     // 文档转换服务客户端
	Embedding *pkgembedding.Client // 向量嵌入服务客户端
	LLM       *pkgopenai.Client    // 大语言模型服务客户端
	Reranker  *pkgrerank.Client    // 文档重排序服务客户端
	Storage   *storage.MinIOClient // 对象存储服务客户端
}

// RagServer RAG服务主体
type RagServer struct {
	// 核心存储
	DB    adapters.VectorDB   // 向量数据库
	Cache *redis.CacheService // 缓存服务

	// 外部客户端
	Storage   *storage.MinIOClient // 对象存储
	Doc2X     *pkgdoc2x.Client     // 文档转换客户端
	Embedding *pkgembedding.Client // 向量嵌入客户端
	LLM       *pkgopenai.Client    // 大语言模型客户端
	Reranker  *pkgrerank.Client    // 重排序客户端

	// 配置和服务
	Config                 *config.Config                  // 配置
	SearchOptimizer        *SearchOptimizer                // 搜索优化器
	promptEmbeddingService *prompts.PromptEmbeddingService // 提示向量化服务
}
