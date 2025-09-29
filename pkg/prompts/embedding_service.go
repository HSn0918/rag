package prompts

import (
	"context"
	"fmt"
	"sync"

	"github.com/hsn0918/rag/pkg/logger"
	"go.uber.org/zap"
)

type EmbeddingGenerator interface {
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)
}

type PromptEmbeddingService struct {
	promptManager      *PromptManager
	embeddingGenerator EmbeddingGenerator
	cache              map[string][]float32
	cacheMu            sync.RWMutex
}

func NewPromptEmbeddingService(generator EmbeddingGenerator) *PromptEmbeddingService {
	return &PromptEmbeddingService{promptManager: NewPromptManager(), embeddingGenerator: generator, cache: make(map[string][]float32)}
}

func (pes *PromptEmbeddingService) InitializeEmbeddings(ctx context.Context) error {
	logger.Get().Info("Initializing prompt embeddings")
	promptTypes := pes.promptManager.ListPromptTypes()
	for _, t := range promptTypes {
		prompt, err := pes.promptManager.GetPrompt(t)
		if err != nil {
			logger.Get().Error("Failed to get prompt", zap.String("type", string(t)), zap.Error(err))
			continue
		}
		emb, err := pes.generateAndCacheEmbedding(ctx, prompt.System, string(t))
		if err != nil {
			logger.Get().Error("Failed to generate embedding", zap.String("type", string(t)), zap.Error(err))
			continue
		}
		if err := pes.promptManager.SetPromptEmbedding(t, emb); err != nil {
			logger.Get().Error("Failed to set prompt embedding", zap.String("type", string(t)), zap.Error(err))
		}
		logger.Get().Debug("Generated embedding for prompt", zap.String("type", string(t)), zap.Int("embedding_dim", len(emb)))
	}
	logger.Get().Info("Prompt embeddings initialized", zap.Int("total_prompts", len(promptTypes)))
	return nil
}

func (pes *PromptEmbeddingService) generateAndCacheEmbedding(ctx context.Context, text string, cacheKey string) ([]float32, error) {
	pes.cacheMu.RLock()
	if cached, ok := pes.cache[cacheKey]; ok {
		pes.cacheMu.RUnlock()
		return cached, nil
	}
	pes.cacheMu.RUnlock()
	embedding, err := pes.embeddingGenerator.GenerateEmbedding(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}
	pes.cacheMu.Lock()
	pes.cache[cacheKey] = embedding
	pes.cacheMu.Unlock()
	return embedding, nil
}

func (pes *PromptEmbeddingService) GetPromptWithEmbedding(t PromptType) (*Prompt, []float32, error) {
	prompt, err := pes.promptManager.GetPrompt(t)
	if err != nil {
		return nil, nil, err
	}
	emb, err := pes.promptManager.GetPromptEmbedding(t)
	if err != nil {
		return prompt, nil, err
	}
	return prompt, emb, nil
}

func (pes *PromptEmbeddingService) FindSimilarPrompt(queryEmbedding []float32) (*Prompt, float32, error) {
	var best *Prompt
	var bestSim float32 = -1
	types := pes.promptManager.ListPromptTypes()
	for _, t := range types {
		prompt, err := pes.promptManager.GetPrompt(t)
		if err != nil {
			continue
		}
		pe, err := pes.promptManager.GetPromptEmbedding(t)
		if err != nil || len(pe) == 0 {
			continue
		}
		sim := cosineSimilarity(queryEmbedding, pe)
		if sim > bestSim {
			bestSim = sim
			best = prompt
		}
	}
	if best == nil {
		return nil, 0, fmt.Errorf("no prompts available for similarity matching")
	}
	return best, bestSim, nil
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (sqrt(normA) * sqrt(normB))
}

func sqrt(x float32) float32 {
	if x < 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

func (pes *PromptEmbeddingService) ClearCache() {
	pes.cacheMu.Lock()
	defer pes.cacheMu.Unlock()
	pes.cache = make(map[string][]float32)
}
func (pes *PromptEmbeddingService) GetPromptManager() *PromptManager { return pes.promptManager }
