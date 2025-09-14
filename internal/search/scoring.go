package search

import (
	"math"
	"strings"

	"github.com/hsn0918/rag/internal/adapters"
	"github.com/hsn0918/rag/internal/logger"
	"go.uber.org/zap"
)

// CalculateAdvancedScore calculates a multi-dimensional relevance score
// for a search result chunk.
//
// The scoring algorithm combines multiple signals with the following weights:
//   - Vector similarity: 40% - Semantic similarity from embeddings
//   - Keyword matching: 30% - Exact keyword presence in content
//   - Phrase matching: 20% - Full query phrase presence
//   - Content quality: 10% - Length and structure quality metrics
//
// The function returns a score between 0 and 1, where higher scores
// indicate better relevance to the query.
func CalculateAdvancedScore(chunk adapters.ChunkSearchResult, query string, keywords []string) float64 {
	// Base vector similarity (40%)
	score := float64(chunk.Similarity) * 0.4

	contentLower := strings.ToLower(chunk.Content)

	// Exact keyword matching (30%)
	keywordScore := calculateKeywordScore(contentLower, keywords)
	score += keywordScore * 0.3

	// Phrase matching (20%)
	queryLower := strings.ToLower(query)
	if strings.Contains(contentLower, queryLower) {
		score += 0.2 // Full query phrase match bonus
	}

	// Content quality scoring (10%)
	contentLength := len(chunk.Content)
	if contentLength > 100 && contentLength < 1500 {
		score += 0.1
	} else if contentLength > 50 {
		score += 0.05
	}

	return score
}

// calculateKeywordScore calculates the keyword matching score based on
// the proportion of query keywords found in the content.
//
// Returns a score between 0 and 1, where 1 means all keywords are present.
func calculateKeywordScore(contentLower string, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0
	}

	matchCount := 0
	for _, keyword := range keywords {
		if strings.Contains(contentLower, strings.ToLower(keyword)) {
			matchCount++
		}
	}

	return float64(matchCount) / float64(len(keywords))
}

// RerankChunksWithKeywords performs intelligent reranking of search results
// using a hybrid scoring algorithm.
//
// This function:
//  1. Calculates comprehensive scores for each chunk
//  2. Sorts chunks by their advanced scores
//  3. Filters results based on maxChunks and minSimilarity thresholds
//  4. Stores scoring metadata for debugging and analysis
//
// Parameters:
//   - chunks: The search results to rerank
//   - query: The original user query
//   - keywords: Extracted keywords from the query
//   - maxChunks: Maximum number of chunks to return
//   - minSimilarity: Minimum similarity threshold for filtering
//
// The function modifies chunks in place by adding scoring metadata.
func RerankChunksWithKeywords(chunks []adapters.ChunkSearchResult, query string, keywords []string, maxChunks int, minSimilarity float32) []adapters.ChunkSearchResult {
	// Calculate comprehensive score for each chunk
	for i := range chunks {
		score := CalculateAdvancedScore(chunks[i], query, keywords)
		// Store score in metadata (temporary solution)
		if chunks[i].Metadata == nil {
			chunks[i].Metadata = make(map[string]interface{})
		}
		chunks[i].Metadata["advanced_score"] = score
	}

	// Sort by comprehensive score
	sortByAdvancedScore(chunks)

	// Filter low quality results
	if len(chunks) > maxChunks {
		chunks = chunks[:maxChunks]
	}

	var filteredChunks []adapters.ChunkSearchResult
	for _, chunk := range chunks {
		if chunk.Similarity > minSimilarity {
			filteredChunks = append(filteredChunks, chunk)
		}
	}

	logger.Get().Debug("Advanced reranking completed",
		zap.Int("original_count", len(chunks)),
		zap.Int("filtered_count", len(filteredChunks)),
		zap.Strings("keywords", keywords),
	)

	return filteredChunks
}

// sortByAdvancedScore sorts chunks by their advanced score in descending order.
//
// This function uses a simple bubble sort algorithm, which is acceptable
// for small datasets (typically <100 chunks). For larger datasets, consider
// using sort.Slice for better performance.
func sortByAdvancedScore(chunks []adapters.ChunkSearchResult) {
	// Simple bubble sort for small datasets
	n := len(chunks)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			scoreJ, _ := chunks[j].Metadata["advanced_score"].(float64)
			scoreJ1, _ := chunks[j+1].Metadata["advanced_score"].(float64)
			if scoreJ < scoreJ1 {
				chunks[j], chunks[j+1] = chunks[j+1], chunks[j]
			}
		}
	}
}

// CalculateAverageSimilarity calculates the average similarity score across
// all provided chunks.
//
// The result is rounded to 3 decimal places for cleaner display in logs
// and metrics. Returns 0 if the chunks slice is empty.
func CalculateAverageSimilarity(chunks []adapters.ChunkSearchResult) float64 {
	if len(chunks) == 0 {
		return 0
	}

	sum := float64(0)
	for _, chunk := range chunks {
		sum += float64(chunk.Similarity)
	}

	return math.Round(sum/float64(len(chunks))*1000) / 1000
}
