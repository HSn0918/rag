package chunking_test

import (
	"context"
	"testing"

	"github.com/hsn0918/rag/internal/chunking"
	"github.com/hsn0918/rag/internal/clients/embedding"
)

// mockEmbedder implements a mock embedding client for testing.
type mockEmbedder struct{}

func (m *mockEmbedder) CreateEmbedding(req embedding.Request) (*embedding.Response, error) {
	// Return deterministic embeddings based on text length
	size := 768 // Standard embedding size
	vec := make([]float64, size)
	// Use length of string input for deterministic output
	textLen := 10 // Default length
	if str, ok := req.Input.(string); ok {
		textLen = len(str)
	}
	for i := range vec {
		vec[i] = float64(textLen%10) / 10.0
	}
	return &embedding.Response{
		Data: []embedding.Data{
			{Embedding: vec},
		},
	}, nil
}

func (m *mockEmbedder) CreateEmbeddingWithDefaults(model, text string) (*embedding.Response, error) {
	// Return deterministic embeddings based on text length
	size := 768 // Standard embedding size
	vec := make([]float64, size)
	for i := range vec {
		vec[i] = float64(len(text)%10) / 10.0
	}
	return &embedding.Response{
		Data: []embedding.Data{
			{Embedding: vec},
		},
	}, nil
}

func (m *mockEmbedder) CreateBatchEmbedding(model string, texts []string) (*embedding.Response, error) {
	var data []embedding.Data
	for i, text := range texts {
		size := 768 // Standard embedding size
		vec := make([]float64, size)
		for j := range vec {
			vec[j] = float64(len(text)%10) / 10.0
		}
		data = append(data, embedding.Data{
			Embedding: vec,
			Index:     i,
		})
	}
	return &embedding.Response{
		Data: data,
	}, nil
}

func TestSemanticChunker_ChunkText(t *testing.T) {
	tests := []struct {
		name                string
		text                string
		maxChunkSize        int
		minChunkSize        int
		similarityThreshold float64
		wantMin             int
		wantMax             int
	}{
		{
			name: "simple text",
			text: `# Title
			This is paragraph one with some content.

			This is paragraph two with more content.`,
			maxChunkSize:        500,
			minChunkSize:        50,
			similarityThreshold: 0.7,
			wantMin:             1,
			wantMax:             2,
		},
		{
			name:         "empty text",
			text:         "",
			maxChunkSize: 500,
			minChunkSize: 50,
			wantMin:      -1, // Expect error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			embedder := &mockEmbedder{}
			chunker, err := chunking.NewSemanticChunker(
				tt.maxChunkSize,
				tt.minChunkSize,
				embedder,
				chunking.WithSimilarityThreshold(tt.similarityThreshold),
			)
			if err != nil {
				t.Fatalf("Failed to create chunker: %v", err)
			}

			chunks, err := chunker.ChunkText(context.Background(), tt.text)

			if tt.wantMin == -1 {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("ChunkText failed: %v", err)
			}

			if len(chunks) < tt.wantMin || len(chunks) > tt.wantMax {
				t.Errorf("Got %d chunks, want between %d and %d",
					len(chunks), tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSemanticChunker_Validation(t *testing.T) {
	tests := []struct {
		name         string
		maxChunkSize int
		minChunkSize int
		threshold    float64
		wantErr      bool
	}{
		{
			name:         "valid config",
			maxChunkSize: 1000,
			minChunkSize: 100,
			threshold:    0.75,
			wantErr:      false,
		},
		{
			name:         "invalid sizes",
			maxChunkSize: 100,
			minChunkSize: 200, // Min > Max
			wantErr:      true,
		},
		{
			name:         "invalid threshold",
			maxChunkSize: 1000,
			minChunkSize: 100,
			threshold:    1.5, // > 1.0
			wantErr:      true,
		},
		{
			name:         "zero max size",
			maxChunkSize: 0,
			minChunkSize: 100,
			threshold:    0.75,
			wantErr:      true,
		},
		{
			name:         "negative min size",
			maxChunkSize: 1000,
			minChunkSize: -100,
			threshold:    0.75,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			embedder := &mockEmbedder{}
			_, err := chunking.NewSemanticChunker(
				tt.maxChunkSize,
				tt.minChunkSize,
				embedder,
				chunking.WithSimilarityThreshold(tt.threshold),
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSemanticChunker() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func BenchmarkSemanticChunker(b *testing.B) {
	embedder := &mockEmbedder{}
	chunker, _ := chunking.NewSemanticChunker(
		2000,
		200,
		embedder,
		chunking.WithSimilarityThreshold(0.75),
		chunking.WithParallelProcessing(true),
	)

	// Sample document
	text := `# Machine Learning Guide

	Machine learning is a subset of artificial intelligence...

	## Supervised Learning

	Supervised learning uses labeled data...

	## Unsupervised Learning

	Unsupervised learning finds patterns...`

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chunker.ChunkText(ctx, text)
	}
}
