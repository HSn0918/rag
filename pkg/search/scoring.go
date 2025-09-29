package search

import (
	"math"
	"strings"

	"github.com/hsn0918/rag/internal/adapters"
	"github.com/hsn0918/rag/pkg/logger"
	"go.uber.org/zap"
)

func CalculateAdvancedScore(chunk adapters.ChunkSearchResult, query string, keywords []string) float64 {
	score := float64(chunk.Similarity) * 0.4
	contentLower := strings.ToLower(chunk.Content)
	keywordScore := calculateKeywordScore(contentLower, keywords)
	score += keywordScore * 0.3
	queryLower := strings.ToLower(query)
	if strings.Contains(contentLower, queryLower) {
		score += 0.2
	}
	contentLength := len(chunk.Content)
	if contentLength > 100 && contentLength < 1500 {
		score += 0.1
	} else if contentLength > 50 {
		score += 0.05
	}
	return score
}

func calculateKeywordScore(contentLower string, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0
	}
	matchCount := 0
	for _, k := range keywords {
		if strings.Contains(contentLower, strings.ToLower(k)) {
			matchCount++
		}
	}
	return float64(matchCount) / float64(len(keywords))
}

func RerankChunksWithKeywords(chunks []adapters.ChunkSearchResult, query string, keywords []string, maxChunks int, minSimilarity float32) []adapters.ChunkSearchResult {
	for i := range chunks {
		score := CalculateAdvancedScore(chunks[i], query, keywords)
		if chunks[i].Metadata == nil {
			chunks[i].Metadata = make(map[string]interface{})
		}
		chunks[i].Metadata["advanced_score"] = score
	}
	sortByAdvancedScore(chunks)
	if len(chunks) > maxChunks {
		chunks = chunks[:maxChunks]
	}
	var filtered []adapters.ChunkSearchResult
	for _, c := range chunks {
		if c.Similarity > minSimilarity {
			filtered = append(filtered, c)
		}
	}
	logger.Get().Debug("Advanced reranking completed", zap.Int("original_count", len(chunks)), zap.Int("filtered_count", len(filtered)), zap.Strings("keywords", keywords))
	return filtered
}

func sortByAdvancedScore(chunks []adapters.ChunkSearchResult) {
	n := len(chunks)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			sj, _ := chunks[j].Metadata["advanced_score"].(float64)
			sj1, _ := chunks[j+1].Metadata["advanced_score"].(float64)
			if sj < sj1 {
				chunks[j], chunks[j+1] = chunks[j+1], chunks[j]
			}
		}
	}
}

func CalculateAverageSimilarity(chunks []adapters.ChunkSearchResult) float64 {
	if len(chunks) == 0 {
		return 0
	}
	sum := float64(0)
	for _, c := range chunks {
		sum += float64(c.Similarity)
	}
	return math.Round(sum/float64(len(chunks))*1000) / 1000
}
