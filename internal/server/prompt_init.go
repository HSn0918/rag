// Package server provides initialization for prompt embedding service.
package server

import (
	"context"
	"fmt"

	"github.com/hsn0918/rag/internal/clients/embedding"
	"github.com/hsn0918/rag/internal/logger"
	"github.com/hsn0918/rag/internal/prompts"
	"go.uber.org/zap"
)

// embeddingAdapter adapts the embedding client to the EmbeddingGenerator interface.
type embeddingAdapter struct {
	client *embedding.Client
}

// GenerateEmbedding implements the EmbeddingGenerator interface.
func (ea *embeddingAdapter) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// Use the embedding client's CreateEmbeddingWithDefaults method
	resp, err := ea.client.CreateEmbeddingWithDefaults("default", text)
	if err != nil {
		return nil, err
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding data returned")
	}

	// Convert []float64 to []float32
	embedding64 := resp.Data[0].Embedding
	embedding32 := make([]float32, len(embedding64))
	for i, v := range embedding64 {
		embedding32[i] = float32(v)
	}

	return embedding32, nil
}

// InitializePromptService initializes the prompt embedding service for the server.
//
// This function creates a new prompt embedding service and initializes embeddings
// for all configured prompts. If initialization fails, the server will continue
// to work with fallback prompts.
func (s *RagServer) InitializePromptService(ctx context.Context) error {
	if s.Embedding == nil {
		logger.Get().Warn("Embedding client not available, skipping prompt service initialization")
		return nil
	}

	// Create embedding adapter
	adapter := &embeddingAdapter{client: s.Embedding}

	// Create prompt embedding service
	s.promptEmbeddingService = prompts.NewPromptEmbeddingService(adapter)

	// Initialize embeddings for all prompts
	if err := s.promptEmbeddingService.InitializeEmbeddings(ctx); err != nil {
		logger.Get().Error("Failed to initialize prompt embeddings",
			zap.Error(err),
		)
		// Don't fail server startup, just log the error
		// The system will work with fallback prompts
	}

	logger.Get().Info("Prompt embedding service initialized successfully")
	return nil
}

// GetPromptService returns the prompt embedding service.
//
// Returns nil if the service is not initialized.
func (s *RagServer) GetPromptService() *prompts.PromptEmbeddingService {
	return s.promptEmbeddingService
}
