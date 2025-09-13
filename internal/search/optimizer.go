// Package search provides optimized search functionality for RAG systems.
package search

import (
	"context"
	"sort"
	"strings"

	"github.com/hsn0918/rag/internal/adapters"
	"github.com/hsn0918/rag/internal/logger"
	"go.uber.org/zap"
)

// Optimizer provides hybrid search optimization.
type Optimizer struct {
	// Immutable configuration
	cfg Config

	// Dependencies
	db adapters.VectorDB
}

// Config defines search optimization parameters.
type Config struct {
	// Weights for hybrid scoring (must sum to 1.0)
	VectorWeight  float64 `default:"0.4"`
	KeywordWeight float64 `default:"0.3"`
	PhraseWeight  float64 `default:"0.2"`
	QualityWeight float64 `default:"0.1"`

	// Search parameters
	CandidateLimit int     `default:"20"`
	ResultLimit    int     `default:"5"`
	MinScore       float64 `default:"0.25"`
}

// NewOptimizer creates a new search optimizer.
func NewOptimizer(cfg Config, db adapters.VectorDB) *Optimizer {
	cfg.setDefaults()
	return &Optimizer{
		cfg: cfg,
		db:  db,
	}
}

// Search performs optimized hybrid search.
func (o *Optimizer) Search(ctx context.Context, query string, vector []float32) ([]adapters.ChunkSearchResult, error) {
	// Perform vector search
	results, err := o.db.SearchSimilarChunks(ctx, vector, o.cfg.CandidateLimit, float32(o.cfg.MinScore))
	if err != nil {
		return nil, err
	}

	// Extract search terms
	terms := o.extractTerms(query)

	// Score and rank results
	scored := o.scoreResults(results, query, terms)

	// Filter and limit
	return o.filterResults(scored), nil
}

// extractTerms extracts search terms from query.
func (o *Optimizer) extractTerms(query string) []string {
	// Simple tokenization - could be enhanced with NLP
	words := strings.Fields(strings.ToLower(query))

	// Remove stop words
	stopWords := map[string]bool{
		"的": true, "了": true, "在": true, "是": true,
		"the": true, "a": true, "an": true, "is": true,
	}

	terms := make([]string, 0, len(words))
	for _, word := range words {
		if !stopWords[word] && len(word) > 1 {
			terms = append(terms, word)
		}
	}

	return terms
}

// scoreResults calculates hybrid scores for results.
func (o *Optimizer) scoreResults(results []adapters.ChunkSearchResult, query string, terms []string) []adapters.ChunkSearchResult {
	for i := range results {
		results[i] = o.scoreResult(results[i], query, terms)
	}
	return results
}

// scoreResult calculates hybrid score for a single result.
func (o *Optimizer) scoreResult(result adapters.ChunkSearchResult, query string, terms []string) adapters.ChunkSearchResult {
	content := strings.ToLower(result.Content)

	// Vector score (already computed)
	vectorScore := float64(result.Similarity)

	// Keyword score
	keywordScore := o.keywordScore(content, terms)

	// Phrase score
	phraseScore := 0.0
	if strings.Contains(content, strings.ToLower(query)) {
		phraseScore = 1.0
	}

	// Quality score
	qualityScore := o.qualityScore(result)

	// Calculate weighted score
	hybridScore := vectorScore*o.cfg.VectorWeight +
		keywordScore*o.cfg.KeywordWeight +
		phraseScore*o.cfg.PhraseWeight +
		qualityScore*o.cfg.QualityWeight

	// Store in metadata
	if result.Metadata == nil {
		result.Metadata = make(map[string]interface{})
	}
	result.Metadata["score"] = hybridScore

	return result
}

// keywordScore calculates keyword matching score.
func (o *Optimizer) keywordScore(content string, terms []string) float64 {
	if len(terms) == 0 {
		return 0
	}

	matches := 0
	for _, term := range terms {
		if strings.Contains(content, term) {
			matches++
		}
	}

	return float64(matches) / float64(len(terms))
}

// qualityScore evaluates content quality.
func (o *Optimizer) qualityScore(result adapters.ChunkSearchResult) float64 {
	length := len(result.Content)

	// Prefer medium-length content
	if length >= 100 && length <= 1500 {
		return 1.0
	}
	if length >= 50 && length <= 2000 {
		return 0.5
	}
	return 0.2
}

// filterResults filters and sorts results by score.
func (o *Optimizer) filterResults(results []adapters.ChunkSearchResult) []adapters.ChunkSearchResult {
	// Sort by hybrid score
	sort.Slice(results, func(i, j int) bool {
		scoreI, _ := results[i].Metadata["score"].(float64)
		scoreJ, _ := results[j].Metadata["score"].(float64)
		return scoreI > scoreJ
	})

	// Filter by minimum score and limit
	filtered := make([]adapters.ChunkSearchResult, 0, o.cfg.ResultLimit)
	for _, result := range results {
		score, _ := result.Metadata["score"].(float64)
		if score < o.cfg.MinScore {
			break
		}

		filtered = append(filtered, result)
		if len(filtered) >= o.cfg.ResultLimit {
			break
		}
	}

	logger.Get().Debug("Search results filtered",
		zap.Int("candidates", len(results)),
		zap.Int("final", len(filtered)),
	)

	return filtered
}

// setDefaults applies default values to config.
func (c *Config) setDefaults() {
	if c.VectorWeight == 0 {
		c.VectorWeight = 0.4
	}
	if c.KeywordWeight == 0 {
		c.KeywordWeight = 0.3
	}
	if c.PhraseWeight == 0 {
		c.PhraseWeight = 0.2
	}
	if c.QualityWeight == 0 {
		c.QualityWeight = 0.1
	}
	if c.CandidateLimit == 0 {
		c.CandidateLimit = 20
	}
	if c.ResultLimit == 0 {
		c.ResultLimit = 5
	}
	if c.MinScore == 0 {
		c.MinScore = 0.25
	}
}
