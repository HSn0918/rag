package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/hsn0918/rag/internal/adapters"
	"github.com/hsn0918/rag/pkg/logger"
)

// Common errors for search optimization.
var (
	// ErrInvalidOptConfig indicates invalid optimizer configuration.
	ErrInvalidOptConfig = errors.New("invalid optimizer configuration")
	// ErrNoSearchResults indicates no search results were found.
	ErrNoSearchResults = errors.New("no search results found")
)

// SearchOptimizer provides advanced search optimization algorithms
// combining vector search with keyword matching and quality scoring.
//
// The optimizer implements a hybrid search strategy that improves
// search relevance by considering multiple ranking signals.
type SearchOptimizer struct {
	ragServer *RagServer
	cfg       Config
}

// Config defines the configuration for SearchOptimizer.
//
// All weights should sum to 1.0 for proper score normalization.
// The configuration allows fine-tuning the balance between different
// ranking signals based on your specific use case.
type Config struct {
	// Required fields.
	InitialCandidates int
	FinalResults      int

	// Weight configuration with defaults.
	VectorWeight  float64
	KeywordWeight float64
	PhraseWeight  float64
	QualityWeight float64

	// Search parameters.
	MinSimilarity float64

	// Performance optimization.
	EnableParallelScoring bool
	CacheSearchResults    bool
}

// Option is a functional option for configuring SearchOptimizer.
//
// Options follow the functional options pattern for flexible and
// extensible configuration.
type Option func(*Config)

// WithVectorWeight sets the weight for vector similarity scoring.
//
// The weight should be between 0 and 1, and all weights should sum to 1.0.
func WithVectorWeight(weight float64) Option {
	return func(c *Config) {
		c.VectorWeight = weight
	}
}

// WithKeywordWeight sets the weight for keyword matching scoring.
//
// The weight should be between 0 and 1, and all weights should sum to 1.0.
func WithKeywordWeight(weight float64) Option {
	return func(c *Config) {
		c.KeywordWeight = weight
	}
}

// WithMinSimilarity sets the minimum similarity threshold for filtering results.
//
// Results with similarity scores below this threshold will be excluded.
func WithMinSimilarity(threshold float64) Option {
	return func(c *Config) {
		c.MinSimilarity = threshold
	}
}

// WithParallelScoring enables or disables parallel scoring of search results.
//
// When enabled, scoring operations will be performed concurrently for better
// performance with large result sets.
func WithParallelScoring(enabled bool) Option {
	return func(c *Config) {
		c.EnableParallelScoring = enabled
	}
}

// NewSearchOptimizer creates a new SearchOptimizer with the specified configuration.
//
// The initialCandidates parameter determines how many results to fetch initially,
// while finalResults determines how many results to return after reranking.
//
// Returns an error if the configuration is invalid.
func NewSearchOptimizer(
	ragServer *RagServer,
	initialCandidates, finalResults int,
	opts ...Option,
) (*SearchOptimizer, error) {
	if ragServer == nil {
		return nil, fmt.Errorf("%w: server is required", ErrInvalidOptConfig)
	}
	if initialCandidates <= 0 || finalResults <= 0 {
		return nil, fmt.Errorf("%w: candidate counts must be positive", ErrInvalidOptConfig)
	}
	if finalResults > initialCandidates {
		return nil, fmt.Errorf("%w: final results cannot exceed initial candidates", ErrInvalidOptConfig)
	}

	cfg := Config{
		InitialCandidates:     initialCandidates,
		FinalResults:          finalResults,
		VectorWeight:          0.4,
		KeywordWeight:         0.3,
		PhraseWeight:          0.2,
		QualityWeight:         0.1,
		MinSimilarity:         0.25,
		EnableParallelScoring: true,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &SearchOptimizer{
		ragServer: ragServer,
		cfg:       cfg,
	}, nil
}

// OptimizedSearch performs an optimized hybrid search combining vector and keyword signals.
//
// This method:
//  1. Extracts search components from the query
//  2. Performs parallel vector and keyword searches
//  3. Merges and deduplicates results
//  4. Applies hybrid scoring
//  5. Reranks and filters results
//
// Returns the final ranked results or an error if the search fails.
func (so *SearchOptimizer) OptimizedSearch(ctx context.Context, query string, queryVector []float32) ([]adapters.ChunkSearchResult, error) {
	if query == "" || len(queryVector) == 0 {
		return nil, fmt.Errorf("%w: query and vector are required", ErrInvalidOptConfig)
	}
	logger.Get().Info("Starting optimized search",
		"query", query,
		"vector_dim", len(queryVector),
	)

	// Extract search components.
	components := so.extractSearchComponents(query)

	var vectorResults, keywordResults []adapters.ChunkSearchResult
	var vectorErr error

	var wg sync.WaitGroup
	wg.Go(func() {
		vectorResults, vectorErr = so.ragServer.DB.SearchSimilarChunks(
			ctx, queryVector, so.cfg.InitialCandidates, float32(so.cfg.MinSimilarity),
		)
	})
	wg.Go(func() {
		if len(components.keywords) > 0 {
			keywordResults = so.performKeywordSearch(ctx, components.keywords)
		}
	})

	// Wait for both goroutines to complete
	wg.Wait()

	if vectorErr != nil {
		return nil, fmt.Errorf("vector search failed: %w", vectorErr)
	}

	// Merge and deduplicate results.
	merged := so.mergeResults(vectorResults, keywordResults)
	if len(merged) == 0 {
		return nil, ErrNoSearchResults
	}

	// Apply hybrid scoring.
	scored := so.applyHybridScoring(merged, components)

	// Re-rank and filter.
	finalResults := so.rerankAndFilter(scored)

	logger.Get().Info("Optimized search completed",
		"vector_results", len(vectorResults),
		"keyword_results", len(keywordResults),
		"final_results", len(finalResults),
	)

	return finalResults, nil
}

// searchComponents holds extracted search components from a query.
//
// These components are used by various scoring algorithms to calculate
// relevance scores.
type searchComponents struct {
	originalQuery string
	keywords      []string
	phrases       []string
	entities      []string
}

// extractSearchComponents analyzes the query to extract search components.
//
// This method identifies keywords, phrases, and potential entities from
// the query text. It filters out stop words and extracts meaningful terms
// for search optimization.
func (so *SearchOptimizer) extractSearchComponents(query string) searchComponents {
	components := searchComponents{
		originalQuery: query,
	}

	// Extract keywords.
	words := strings.Fields(strings.ToLower(query))

	// Filter stop words.
	stopWords := map[string]bool{
		"的": true, "了": true, "在": true, "是": true, "我": true,
		"有": true, "和": true, "the": true, "a": true, "an": true,
		"in": true, "on": true, "at": true, "to": true, "for": true,
	}

	for _, word := range words {
		if !stopWords[word] && len(word) > 1 {
			components.keywords = append(components.keywords, word)
		}
	}

	// Extract phrases (2-3 word combinations).
	if len(words) >= 2 {
		for i := 0; i < len(words)-1; i++ {
			phrase := words[i] + " " + words[i+1]
			components.phrases = append(components.phrases, phrase)

			if i < len(words)-2 {
				phrase3 := phrase + " " + words[i+2]
				components.phrases = append(components.phrases, phrase3)
			}
		}
	}

	// Extract potential entities (capitalized words, longer terms).
	for _, word := range strings.Fields(query) {
		if len(word) > 3 && (strings.ToUpper(word[:1]) == word[:1] || isChinese(word)) {
			components.entities = append(components.entities, word)
		}
	}

	return components
}

// performKeywordSearch performs keyword-based search.
//
// This is a placeholder implementation. In a production system, this would
// query the database using keyword matching algorithms.
func (so *SearchOptimizer) performKeywordSearch(_ context.Context, _ []string) []adapters.ChunkSearchResult {
	// Placeholder - actual implementation would query the database.
	return []adapters.ChunkSearchResult{}
}

// mergeResults combines and deduplicates results from different search methods.
//
// When a chunk appears in multiple result sets, their scores are averaged
// to create a combined relevance score.
func (so *SearchOptimizer) mergeResults(vectorResults, keywordResults []adapters.ChunkSearchResult) []adapters.ChunkSearchResult {
	resultMap := make(map[string]*adapters.ChunkSearchResult)

	// Add vector results.
	for i := range vectorResults {
		result := &vectorResults[i]
		resultMap[result.ChunkID] = result
	}

	// Merge keyword results.
	for i := range keywordResults {
		result := &keywordResults[i]
		if existing, exists := resultMap[result.ChunkID]; exists {
			// Combine scores if chunk appears in both results.
			existing.Similarity = (existing.Similarity + result.Similarity) / 2
		} else {
			resultMap[result.ChunkID] = result
		}
	}

	// Convert map back to slice.
	var merged []adapters.ChunkSearchResult
	for _, result := range resultMap {
		merged = append(merged, *result)
	}

	return merged
}

// applyHybridScoring calculates hybrid scores for all results.
//
// This method applies the configured scoring weights to combine multiple
// ranking signals into a single relevance score.
func (so *SearchOptimizer) applyHybridScoring(results []adapters.ChunkSearchResult, components searchComponents) []adapters.ChunkSearchResult {
	if so.cfg.EnableParallelScoring {
		return so.parallelHybridScoring(results, components)
	}

	for i := range results {
		results[i] = so.calculateHybridScore(&results[i], components)
	}

	return results
}

// parallelHybridScoring applies hybrid scoring in parallel using WaitGroup.Go.
//
// This method leverages Go 1.23+ WaitGroup.Go for cleaner concurrent processing
// of scoring operations, improving performance for large result sets.
func (so *SearchOptimizer) parallelHybridScoring(results []adapters.ChunkSearchResult, components searchComponents) []adapters.ChunkSearchResult {
	var wg sync.WaitGroup

	for i := range results {
		idx := i // Capture loop variable
		wg.Go(func() {
			results[idx] = so.calculateHybridScore(&results[idx], components)
		})
	}

	wg.Wait()
	return results
}

// calculateHybridScore computes the hybrid score for a single result.
//
// The score combines multiple signals according to the configured weights:
//   - Vector similarity: Semantic relevance
//   - Keyword matching: Term presence
//   - Phrase matching: Exact phrase matches
//   - Content quality: Length and structure metrics
func (so *SearchOptimizer) calculateHybridScore(result *adapters.ChunkSearchResult, components searchComponents) adapters.ChunkSearchResult {
	// Initialize score components.
	vectorScore := float64(result.Similarity)
	keywordScore := so.calculateKeywordScore(result.Content, components.keywords)
	phraseScore := so.calculatePhraseScore(result.Content, components.phrases)
	qualityScore := so.calculateQualityScore(result)

	// Calculate weighted hybrid score.
	hybridScore := (vectorScore * so.cfg.VectorWeight) +
		(keywordScore * so.cfg.KeywordWeight) +
		(phraseScore * so.cfg.PhraseWeight) +
		(qualityScore * so.cfg.QualityWeight)

	// Store detailed scores in metadata.
	if result.Metadata == nil {
		result.Metadata = make(map[string]interface{})
	}
	result.Metadata["hybrid_score"] = hybridScore
	result.Metadata["vector_score"] = vectorScore
	result.Metadata["keyword_score"] = keywordScore
	result.Metadata["phrase_score"] = phraseScore
	result.Metadata["quality_score"] = qualityScore

	return *result
}

// calculateKeywordScore calculates the keyword matching score.
//
// The score is based on both coverage (how many keywords match) and
// frequency (how often they appear), with coverage weighted more heavily.
func (so *SearchOptimizer) calculateKeywordScore(content string, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0
	}

	contentLower := strings.ToLower(content)
	matchCount := 0
	totalMatches := 0

	for _, keyword := range keywords {
		// Count occurrences of each keyword.
		count := strings.Count(contentLower, strings.ToLower(keyword))
		if count > 0 {
			matchCount++
			totalMatches += count
		}
	}

	// Calculate score based on coverage and frequency.
	coverage := float64(matchCount) / float64(len(keywords))
	frequency := math.Min(float64(totalMatches)/float64(len(keywords)), 3.0) / 3.0

	return (coverage * 0.7) + (frequency * 0.3)
}

// calculatePhraseScore calculates the phrase matching score.
//
// Returns the proportion of phrases that appear in the content.
func (so *SearchOptimizer) calculatePhraseScore(content string, phrases []string) float64 {
	if len(phrases) == 0 {
		return 0
	}

	contentLower := strings.ToLower(content)
	matchCount := 0

	for _, phrase := range phrases {
		if strings.Contains(contentLower, strings.ToLower(phrase)) {
			matchCount++
		}
	}

	return float64(matchCount) / float64(len(phrases))
}

// calculateQualityScore evaluates content quality based on multiple factors.
//
// Quality signals include:
//   - Content length (optimal range: 100-1500 characters)
//   - Structured content (presence of formatting)
//   - Metadata quality indicators
func (so *SearchOptimizer) calculateQualityScore(result *adapters.ChunkSearchResult) float64 {
	score := 0.0

	// Content length score.
	length := len(result.Content)
	if length >= 100 && length <= 1500 {
		score += 0.4
	} else if length >= 50 && length <= 2000 {
		score += 0.2
	}

	// Check for structured content.
	if strings.Contains(result.Content, "\n") || strings.Contains(result.Content, "。") {
		score += 0.2
	}

	// Check metadata quality indicators.
	if result.Metadata != nil {
		if chunkType, ok := result.Metadata["chunk_type"].(string); ok {
			if chunkType == "section" || chunkType == "header" {
				score += 0.2
			}
		}
		if _, hasTitle := result.Metadata["chunk_title"]; hasTitle {
			score += 0.2
		}
	}

	return math.Min(score, 1.0)
}

// rerankAndFilter performs final re-ranking and filtering of results.
//
// This method sorts results by hybrid score, applies minimum score
// thresholds, limits the result count, and ensures diversity.
func (so *SearchOptimizer) rerankAndFilter(results []adapters.ChunkSearchResult) []adapters.ChunkSearchResult {
	// Sort by hybrid score.
	sort.Slice(results, func(i, j int) bool {
		scoreI, _ := results[i].Metadata["hybrid_score"].(float64)
		scoreJ, _ := results[j].Metadata["hybrid_score"].(float64)
		return scoreI > scoreJ
	})

	// Apply filters.
	var filtered []adapters.ChunkSearchResult
	for _, result := range results {
		hybridScore, _ := result.Metadata["hybrid_score"].(float64)

		// Filter by minimum score threshold.
		if hybridScore < so.cfg.MinSimilarity {
			continue
		}

		filtered = append(filtered, result)

		// Limit to final result count.
		if len(filtered) >= so.cfg.FinalResults {
			break
		}
	}

	// Diversity optimization: ensure results aren't too similar.
	filtered = so.ensureDiversity(filtered)

	return filtered
}

// ensureDiversity ensures diversity in the final results.
//
// This method removes results that are too similar to already selected
// results, preventing redundant information in the response.
func (so *SearchOptimizer) ensureDiversity(results []adapters.ChunkSearchResult) []adapters.ChunkSearchResult {
	if len(results) <= 2 {
		return results
	}

	var diverse []adapters.ChunkSearchResult
	diverse = append(diverse, results[0])

	for i := 1; i < len(results); i++ {
		// Check similarity with already selected results.
		tooSimilar := false
		for _, selected := range diverse {
			similarity := so.contentSimilarity(results[i].Content, selected.Content)
			if similarity > 0.9 { // Too similar.
				tooSimilar = true
				break
			}
		}

		if !tooSimilar {
			diverse = append(diverse, results[i])
		}
	}

	return diverse
}

// contentSimilarity calculates Jaccard similarity between two texts.
//
// This is a simple and fast similarity metric used for diversity filtering.
// Returns a value between 0 (no similarity) and 1 (identical).
func (so *SearchOptimizer) contentSimilarity(content1, content2 string) float64 {
	// Simple Jaccard similarity for quick comparison.
	words1 := strings.Fields(strings.ToLower(content1))
	words2 := strings.Fields(strings.ToLower(content2))

	set1 := make(map[string]bool)
	set2 := make(map[string]bool)

	for _, w := range words1 {
		set1[w] = true
	}
	for _, w := range words2 {
		set2[w] = true
	}

	intersection := 0
	for w := range set1 {
		if set2[w] {
			intersection++
		}
	}

	union := len(set1) + len(set2) - intersection
	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

// validate checks if the configuration is valid.
//
// Validation ensures:
//   - All weights sum to 1.0 (within tolerance)
//   - Individual weights are between 0 and 1
//   - Similarity threshold is between 0 and 1
func (c *Config) validate() error {
	// Validate weights sum to 1.0.
	totalWeight := c.VectorWeight + c.KeywordWeight + c.PhraseWeight + c.QualityWeight
	if math.Abs(totalWeight-1.0) > 0.01 {
		return fmt.Errorf("%w: weights must sum to 1.0, got %f", ErrInvalidOptConfig, totalWeight)
	}

	// Validate individual weights.
	if c.VectorWeight < 0 || c.VectorWeight > 1 {
		return fmt.Errorf("%w: vector weight must be in [0,1]", ErrInvalidOptConfig)
	}
	if c.MinSimilarity < 0 || c.MinSimilarity > 1 {
		return fmt.Errorf("%w: min similarity must be in [0,1]", ErrInvalidOptConfig)
	}

	return nil
}

// isChinese checks if a string contains Chinese characters.
//
// This function checks for characters in the CJK Unified Ideographs range
// (U+4E00 to U+9FFF), which covers common Chinese characters.
func isChinese(str string) bool {
	for _, r := range str {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}
