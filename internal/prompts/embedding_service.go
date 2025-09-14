// Package prompts provides prompt management and embedding services.
package prompts

import (
	"context"
	"fmt"
	"sync"

	"github.com/hsn0918/rag/internal/logger"
	"go.uber.org/zap"
)

// EmbeddingGenerator defines the interface for generating embeddings.
type EmbeddingGenerator interface {
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)
}

// PromptEmbeddingService manages prompt embeddings with caching.
type PromptEmbeddingService struct {
	promptManager      *PromptManager
	embeddingGenerator EmbeddingGenerator
	cache              map[string][]float32
	cacheMu            sync.RWMutex
}

// NewPromptEmbeddingService creates a new prompt embedding service.
func NewPromptEmbeddingService(generator EmbeddingGenerator) *PromptEmbeddingService {
	return &PromptEmbeddingService{
		promptManager:      NewPromptManager(),
		embeddingGenerator: generator,
		cache:              make(map[string][]float32),
	}
}

// InitializeEmbeddings generates embeddings for all prompts.
func (pes *PromptEmbeddingService) InitializeEmbeddings(ctx context.Context) error {
	logger.Get().Info("Initializing prompt embeddings")

	promptTypes := pes.promptManager.ListPromptTypes()
	for _, promptType := range promptTypes {
		prompt, err := pes.promptManager.GetPrompt(promptType)
		if err != nil {
			logger.Get().Error("Failed to get prompt",
				zap.String("type", string(promptType)),
				zap.Error(err),
			)
			continue
		}

		// Generate embedding for the system prompt
		embedding, err := pes.generateAndCacheEmbedding(ctx, prompt.System, string(promptType))
		if err != nil {
			logger.Get().Error("Failed to generate embedding",
				zap.String("type", string(promptType)),
				zap.Error(err),
			)
			continue
		}

		// Store the embedding in the prompt
		if err := pes.promptManager.SetPromptEmbedding(promptType, embedding); err != nil {
			logger.Get().Error("Failed to set prompt embedding",
				zap.String("type", string(promptType)),
				zap.Error(err),
			)
		}

		logger.Get().Debug("Generated embedding for prompt",
			zap.String("type", string(promptType)),
			zap.Int("embedding_dim", len(embedding)),
		)
	}

	logger.Get().Info("Prompt embeddings initialized",
		zap.Int("total_prompts", len(promptTypes)),
	)

	return nil
}

// generateAndCacheEmbedding generates and caches an embedding.
func (pes *PromptEmbeddingService) generateAndCacheEmbedding(ctx context.Context, text string, cacheKey string) ([]float32, error) {
	// Check cache first
	pes.cacheMu.RLock()
	if cached, exists := pes.cache[cacheKey]; exists {
		pes.cacheMu.RUnlock()
		return cached, nil
	}
	pes.cacheMu.RUnlock()

	// Generate new embedding
	embedding, err := pes.embeddingGenerator.GenerateEmbedding(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	// Cache the result
	pes.cacheMu.Lock()
	pes.cache[cacheKey] = embedding
	pes.cacheMu.Unlock()

	return embedding, nil
}

// GetPromptWithEmbedding returns a prompt with its embedding.
func (pes *PromptEmbeddingService) GetPromptWithEmbedding(promptType PromptType) (*Prompt, []float32, error) {
	prompt, err := pes.promptManager.GetPrompt(promptType)
	if err != nil {
		return nil, nil, err
	}

	embedding, err := pes.promptManager.GetPromptEmbedding(promptType)
	if err != nil {
		return prompt, nil, err
	}

	return prompt, embedding, nil
}

// FindSimilarPrompt finds the most similar prompt based on query embedding.
func (pes *PromptEmbeddingService) FindSimilarPrompt(queryEmbedding []float32) (*Prompt, float32, error) {
	var bestPrompt *Prompt
	var bestSimilarity float32 = -1

	promptTypes := pes.promptManager.ListPromptTypes()
	for _, promptType := range promptTypes {
		prompt, err := pes.promptManager.GetPrompt(promptType)
		if err != nil {
			continue
		}

		promptEmbedding, err := pes.promptManager.GetPromptEmbedding(promptType)
		if err != nil || len(promptEmbedding) == 0 {
			continue
		}

		similarity := cosineSimilarity(queryEmbedding, promptEmbedding)
		if similarity > bestSimilarity {
			bestSimilarity = similarity
			bestPrompt = prompt
		}
	}

	if bestPrompt == nil {
		return nil, 0, fmt.Errorf("no prompts available for similarity matching")
	}

	return bestPrompt, bestSimilarity, nil
}

// cosineSimilarity calculates cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrt(normA) * sqrt(normB))
}

// sqrt calculates square root for float32.
func sqrt(x float32) float32 {
	if x < 0 {
		return 0
	}

	// Newton's method for square root
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// ClearCache clears the embedding cache.
func (pes *PromptEmbeddingService) ClearCache() {
	pes.cacheMu.Lock()
	defer pes.cacheMu.Unlock()
	pes.cache = make(map[string][]float32)
}

// GetPromptManager returns the underlying prompt manager.
func (pes *PromptEmbeddingService) GetPromptManager() *PromptManager {
	return pes.promptManager
}
