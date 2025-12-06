// Package chunking provides text chunking functionality for RAG systems.
package chunking

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/hsn0918/rag/pkg/clients/embedding"
	"github.com/hsn0918/rag/pkg/logger"
	"log/slog"
)

// Common errors.
var (
	ErrInvalidConfig   = errors.New("invalid configuration")
	ErrNoEmbeddingData = errors.New("no embedding data in response")
	ErrChunkingFailed  = errors.New("chunking failed")
)

// SemanticChunker implements semantic-aware text chunking.
type SemanticChunker struct {
	cfg      Config
	embedder embedding.Embedder
	base     *OptimizedMarkdownChunker
	cache    *embeddingCache
}

// Config defines chunking configuration.
type Config struct {
	// Required fields.
	MaxChunkSize int
	MinChunkSize int
	Model        string

	// Optional fields with defaults.
	OverlapSize         int
	SimilarityThreshold float64
	MaxMergeChunks      int
	EnableParallel      bool
}

// Option configures a SemanticChunker.
type Option func(*Config)

// WithModel sets the embedding model.
func WithModel(model string) Option {
	return func(c *Config) {
		c.Model = model
	}
}

// WithSimilarityThreshold sets the similarity threshold for merging.
func WithSimilarityThreshold(threshold float64) Option {
	return func(c *Config) {
		c.SimilarityThreshold = threshold
	}
}

// WithParallelProcessing enables parallel embedding generation.
func WithParallelProcessing(enabled bool) Option {
	return func(c *Config) {
		c.EnableParallel = enabled
	}
}

// WithMaxMergeChunks sets the maximum number of chunks to merge.
func WithMaxMergeChunks(max int) Option {
	return func(c *Config) {
		c.MaxMergeChunks = max
	}
}

// NewSemanticChunker creates a new semantic chunker.
func NewSemanticChunker(
	maxChunkSize, minChunkSize int,
	embedder embedding.Embedder,
	opts ...Option,
) (*SemanticChunker, error) {
	if maxChunkSize <= 0 || minChunkSize <= 0 {
		return nil, fmt.Errorf("%w: chunk sizes must be positive", ErrInvalidConfig)
	}
	if maxChunkSize <= minChunkSize {
		return nil, fmt.Errorf("%w: max must be greater than min", ErrInvalidConfig)
	}
	if embedder == nil {
		return nil, fmt.Errorf("%w: embedder is required", ErrInvalidConfig)
	}

	cfg := Config{
		MaxChunkSize:        maxChunkSize,
		MinChunkSize:        minChunkSize,
		Model:               "text-embedding-ada-002",
		OverlapSize:         200,
		SimilarityThreshold: 0.75,
		MaxMergeChunks:      3,
		EnableParallel:      true,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	base, err := NewOptimizedMarkdownChunker(ChunkerConfig{
		MaxChunkSize:       cfg.MaxChunkSize,
		MinChunkSize:       cfg.MinChunkSize,
		OverlapSize:        cfg.OverlapSize,
		PreserveStructure:  true,
		EnableSemantic:     true,
		MergeSparseParents: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create base chunker: %w", err)
	}

	return &SemanticChunker{
		cfg:      cfg,
		embedder: embedder,
		base:     base,
		cache:    newEmbeddingCache(1000),
	}, nil
}

// ChunkText performs semantic-aware text chunking.
func (sc *SemanticChunker) ChunkText(ctx context.Context, text string) ([]Chunk, error) {
	if text == "" {
		return nil, ErrEmptyContent
	}

	chunks, err := sc.base.ChunkMarkdown(text)
	if err != nil {
		return nil, fmt.Errorf("base chunking: %w", err)
	}

	// Skip semantic processing for single chunk.
	if len(chunks) <= 1 {
		return chunks, nil
	}

	embeddings, err := sc.generateEmbeddings(ctx, chunks)
	if err != nil {
		// Log warning but don't fail - fallback to base chunks.
		logger.Get().Warn("Failed to generate embeddings, using base chunks",
			slog.Any("error", err),
		)
		return chunks, nil
	}

	merged := sc.mergeBySemantics(chunks, embeddings)
	return sc.postProcess(merged), nil
}

// generateEmbeddings creates embeddings for all chunks.
func (sc *SemanticChunker) generateEmbeddings(ctx context.Context, chunks []Chunk) ([][]float32, error) {
	if !sc.cfg.EnableParallel {
		return sc.sequentialEmbeddings(ctx, chunks)
	}
	return sc.parallelEmbeddings(ctx, chunks)
}

// sequentialEmbeddings generates embeddings one by one.
func (sc *SemanticChunker) sequentialEmbeddings(ctx context.Context, chunks []Chunk) ([][]float32, error) {
	embeddings := make([][]float32, len(chunks))

	for i, chunk := range chunks {
		embed, err := sc.getEmbedding(ctx, chunk.Content)
		if err != nil {
			return nil, fmt.Errorf("chunk %d: %w", i, err)
		}
		embeddings[i] = embed
	}

	return embeddings, nil
}

// parallelEmbeddings generates embeddings concurrently.
func (sc *SemanticChunker) parallelEmbeddings(ctx context.Context, chunks []Chunk) ([][]float32, error) {
	const maxWorkers = 5

	type result struct {
		idx   int
		embed []float32
		err   error
	}

	// Use buffered channel for results.
	results := make(chan result, len(chunks))

	// Create worker pool using WaitGroup.Go (Go 1.23+).
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxWorkers)

	for i, chunk := range chunks {
		idx, content := i, chunk.Content // Capture loop variables
		wg.Go(func() {
			// Acquire semaphore.
			sem <- struct{}{}
			defer func() { <-sem }()

			embed, err := sc.getEmbedding(ctx, content)
			results <- result{idx: idx, embed: embed, err: err}
		})
	}

	// Wait for all workers and close results.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results.
	embeddings := make([][]float32, len(chunks))
	for res := range results {
		if res.err != nil {
			return nil, fmt.Errorf("chunk %d: %w", res.idx, res.err)
		}
		embeddings[res.idx] = res.embed
	}

	return embeddings, nil
}

// getEmbedding retrieves or generates an embedding.
func (sc *SemanticChunker) getEmbedding(ctx context.Context, text string) ([]float32, error) {
	// Check cache first.
	if cached := sc.cache.get(text); cached != nil {
		return cached, nil
	}

	resp, err := sc.embedder.CreateEmbeddingWithDefaults(sc.cfg.Model, text)
	if err != nil {
		return nil, fmt.Errorf("create embedding: %w", err)
	}

	if resp == nil || len(resp.Data) == 0 {
		return nil, ErrNoEmbeddingData
	}

	// Convert float64 to float32.
	embed := make([]float32, len(resp.Data[0].Embedding))
	for i, v := range resp.Data[0].Embedding {
		embed[i] = float32(v)
	}

	sc.cache.set(text, embed)
	return embed, nil
}

// mergeBySemantics merges chunks based on semantic similarity.
func (sc *SemanticChunker) mergeBySemantics(chunks []Chunk, embeddings [][]float32) []Chunk {
	if len(chunks) == 0 {
		return chunks
	}

	var merged []Chunk
	i := 0

	for i < len(chunks) {
		group := []int{i}
		currentSize := len(chunks[i].Content)

		// Try to merge with subsequent chunks.
		for j := i + 1; j < len(chunks) && len(group) < sc.cfg.MaxMergeChunks; j++ {
			nextSize := currentSize + len(chunks[j].Content)
			if nextSize > sc.cfg.MaxChunkSize {
				break
			}

			similarity := cosineSimilarity(embeddings[i], embeddings[j])
			if similarity < sc.cfg.SimilarityThreshold {
				break
			}

			group = append(group, j)
			currentSize = nextSize
		}

		merged = append(merged, sc.mergeChunks(chunks, group))
		i = group[len(group)-1] + 1
	}

	return merged
}

// mergeChunks combines multiple chunks into one.
func (sc *SemanticChunker) mergeChunks(chunks []Chunk, indices []int) Chunk {
	if len(indices) == 1 {
		return chunks[indices[0]]
	}

	// Start with first chunk as base.
	merged := chunks[indices[0]]

	// Build combined content.
	var contents []string
	for _, idx := range indices {
		contents = append(contents, chunks[idx].Content)
	}

	// Join with double newline.
	for i := 1; i < len(contents); i++ {
		merged.Content += "\n\n" + contents[i]
	}

	// Update span.
	merged.EndIndex = chunks[indices[len(indices)-1]].EndIndex

	// Add metadata.
	if merged.Metadata == nil {
		merged.Metadata = make(map[string]string)
	}
	merged.Metadata["merged_count"] = fmt.Sprintf("%d", len(indices))

	return merged
}

// postProcess performs final optimization on chunks.
func (sc *SemanticChunker) postProcess(chunks []Chunk) []Chunk {
	if len(chunks) == 0 {
		return chunks
	}

	processed := make([]Chunk, 0, len(chunks))

	for i, chunk := range chunks {
		// Skip chunks that are too small (except the last one).
		if len(chunk.Content) < sc.cfg.MinChunkSize && i < len(chunks)-1 {
			continue
		}

		// Update chunk metadata.
		chunk.ID = fmt.Sprintf("chunk_%d", len(processed))
		chunk.TokenCount = sc.base.estimateTokenCount(chunk.Content)

		processed = append(processed, chunk)
	}

	return processed
}

// validate checks if the configuration is valid.
func (c *Config) validate() error {
	if c.SimilarityThreshold < 0 || c.SimilarityThreshold > 1 {
		return fmt.Errorf("%w: similarity threshold must be in [0,1]", ErrInvalidConfig)
	}
	if c.MaxMergeChunks <= 0 {
		return fmt.Errorf("%w: max merge chunks must be positive", ErrInvalidConfig)
	}
	return nil
}

// embeddingCache provides thread-safe caching for embeddings.
type embeddingCache struct {
	mu    sync.RWMutex
	items map[string][]float32
	order []string
	size  int
}

// newEmbeddingCache creates a new cache with the given capacity.
func newEmbeddingCache(size int) *embeddingCache {
	return &embeddingCache{
		items: make(map[string][]float32, size),
		order: make([]string, 0, size),
		size:  size,
	}
}

// get retrieves an embedding from cache.
func (c *embeddingCache) get(key string) []float32 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.items[hashKey(key)]
}

// set stores an embedding in cache.
func (c *embeddingCache) set(key string, value []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	hashed := hashKey(key)

	// Evict oldest if at capacity.
	if len(c.items) >= c.size && c.items[hashed] == nil {
		if len(c.order) > 0 {
			delete(c.items, c.order[0])
			c.order = c.order[1:]
		}
	}

	c.items[hashed] = value
	c.order = append(c.order, hashed)
}

// hashKey generates a cache key from text.
func hashKey(text string) string {
	const maxLen = 32
	if len(text) > maxLen {
		text = text[:maxLen]
	}
	return fmt.Sprintf("%x", text)
}

// cosineSimilarity calculates the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
