package search

import (
	"context"
	"sort"
	"strings"

	"github.com/hsn0918/rag/internal/adapters"
	"github.com/hsn0918/rag/pkg/logger"
	"go.uber.org/zap"
)

type Optimizer struct {
	cfg Config
	db  adapters.VectorDB
}

type Config struct {
	VectorWeight  float64 `default:"0.4"`
	KeywordWeight float64 `default:"0.3"`
	PhraseWeight  float64 `default:"0.2"`
	QualityWeight float64 `default:"0.1"`

	CandidateLimit int     `default:"20"`
	ResultLimit    int     `default:"5"`
	MinScore       float64 `default:"0.25"`
}

func NewOptimizer(cfg Config, db adapters.VectorDB) *Optimizer {
	cfg.setDefaults()
	return &Optimizer{cfg: cfg, db: db}
}

func (o *Optimizer) Search(ctx context.Context, query string, vector []float32) ([]adapters.ChunkSearchResult, error) {
	results, err := o.db.SearchSimilarChunks(ctx, vector, o.cfg.CandidateLimit, float32(o.cfg.MinScore))
	if err != nil {
		return nil, err
	}
	terms := o.extractTerms(query)
	scored := o.scoreResults(results, query, terms)
	return o.filterResults(scored), nil
}

func (o *Optimizer) extractTerms(query string) []string {
	words := strings.Fields(strings.ToLower(query))
	stopWords := map[string]bool{"的": true, "了": true, "在": true, "是": true, "the": true, "a": true, "an": true, "is": true}
	terms := make([]string, 0, len(words))
	for _, w := range words {
		if !stopWords[w] && len(w) > 1 {
			terms = append(terms, w)
		}
	}
	return terms
}

func (o *Optimizer) scoreResults(results []adapters.ChunkSearchResult, query string, terms []string) []adapters.ChunkSearchResult {
	for i := range results {
		results[i] = o.scoreResult(results[i], query, terms)
	}
	return results
}

func (o *Optimizer) scoreResult(result adapters.ChunkSearchResult, query string, terms []string) adapters.ChunkSearchResult {
	content := strings.ToLower(result.Content)
	vectorScore := float64(result.Similarity)
	keywordScore := o.keywordScore(content, terms)
	phraseScore := 0.0
	if strings.Contains(content, strings.ToLower(query)) {
		phraseScore = 1.0
	}
	qualityScore := o.qualityScore(result)
	hybrid := vectorScore*o.cfg.VectorWeight + keywordScore*o.cfg.KeywordWeight + phraseScore*o.cfg.PhraseWeight + qualityScore*o.cfg.QualityWeight
	if result.Metadata == nil {
		result.Metadata = make(map[string]interface{})
	}
	result.Metadata["score"] = hybrid
	return result
}

func (o *Optimizer) keywordScore(content string, terms []string) float64 {
	if len(terms) == 0 {
		return 0
	}
	matches := 0
	for _, t := range terms {
		if strings.Contains(content, t) {
			matches++
		}
	}
	return float64(matches) / float64(len(terms))
}

func (o *Optimizer) qualityScore(result adapters.ChunkSearchResult) float64 {
	length := len(result.Content)
	if length >= 100 && length <= 1500 {
		return 1.0
	}
	if length >= 50 && length <= 2000 {
		return 0.5
	}
	return 0.2
}

func (o *Optimizer) filterResults(results []adapters.ChunkSearchResult) []adapters.ChunkSearchResult {
	sort.Slice(results, func(i, j int) bool {
		si, _ := results[i].Metadata["score"].(float64)
		sj, _ := results[j].Metadata["score"].(float64)
		return si > sj
	})
	filtered := make([]adapters.ChunkSearchResult, 0, o.cfg.ResultLimit)
	for _, r := range results {
		s, _ := r.Metadata["score"].(float64)
		if s < o.cfg.MinScore {
			break
		}
		filtered = append(filtered, r)
		if len(filtered) >= o.cfg.ResultLimit {
			break
		}
	}
	logger.Get().Debug("Search results filtered", zap.Int("candidates", len(results)), zap.Int("final", len(filtered)))
	return filtered
}

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

// Package search provides optimized search functionality for RAG systems.
