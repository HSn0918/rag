package server

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/hsn0918/rag/internal/adapters"
	"github.com/hsn0918/rag/internal/clients/openai"
	ragv1 "github.com/hsn0918/rag/internal/gen/rag/v1"
	"github.com/hsn0918/rag/internal/logger"
	"github.com/hsn0918/rag/internal/prompts"
	"github.com/hsn0918/rag/internal/search"
	"github.com/hsn0918/rag/internal/utils"
	"go.uber.org/zap"
)

// GetContext implements intelligent document retrieval and question-answering
// functionality using RAG (Retrieval-Augmented Generation).
//
// The complete RAG pipeline consists of:
//  1. Extract keywords from user query using LLM
//  2. Generate query embeddings for semantic search
//  3. Perform intelligent reranking of search results
//  4. Generate personalized responses using LLM
//
// This method handles the entire request lifecycle including logging,
// error handling, and fallback strategies when services are unavailable.
func (s *RagServer) GetContext(
	ctx context.Context,
	req *connect.Request[ragv1.GetContextRequest],
) (*connect.Response[ragv1.GetContextResponse], error) {
	startTime := time.Now()
	query := req.Msg.GetQuery()

	if query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("query is required"))
	}

	logger.Get().Info("开始处理智能文档检索请求",
		zap.String("query", query),
		zap.Int("query_length", len(query)),
		zap.String("request_id", req.Header().Get("X-Request-ID")),
		zap.Time("start_time", startTime),
	)

	// 第一步：使用大模型进行智能分词和关键词提取
	logger.Get().Debug("开始提取关键词", zap.String("query", query))
	keywordsStart := time.Now()
	keywords, err := s.generateKeywords(ctx, query)
	keywordsDuration := time.Since(keywordsStart)

	if err != nil {
		logger.Get().Error("大模型关键词提取失败",
			zap.Error(err),
			zap.Duration("duration", keywordsDuration),
		)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate keywords: %w", err))
	}
	logger.Get().Info("大模型关键词提取完成",
		zap.Strings("keywords", keywords),
		zap.Int("keywords_count", len(keywords)),
		zap.Duration("duration", keywordsDuration),
	)

	// 第二步：生成语义向量进行相似性搜索
	// 优先使用提取的关键词，回退到原始查询
	queryText := strings.Join(keywords, " ")
	if queryText == "" {
		logger.Get().Debug("关键词为空，使用原始查询生成向量")
		queryText = query
	}

	logger.Get().Debug("开始生成查询向量", zap.String("query_text", queryText))
	embeddingStart := time.Now()
	queryVector, err := s.generateEmbedding(ctx, queryText)
	embeddingDuration := time.Since(embeddingStart)

	if err != nil {
		logger.Get().Error("查询向量生成失败",
			zap.Error(err),
			zap.Duration("duration", embeddingDuration),
		)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate query embedding: %w", err))
	}
	logger.Get().Debug("查询向量生成完成",
		zap.Int("vector_dim", len(queryVector)),
		zap.Duration("duration", embeddingDuration),
	)

	// 第三步：使用优化的搜索策略
	var similarChunks []adapters.ChunkSearchResult
	searchStart := time.Now()

	// Check if search optimizer is available
	if s.SearchOptimizer != nil {
		logger.Get().Debug("使用优化搜索策略")
		// Use optimized hybrid search
		similarChunks, err = s.SearchOptimizer.OptimizedSearch(ctx, query, queryVector)
		if err != nil {
			logger.Get().Error("优化搜索失败，回退到标准搜索",
				zap.Error(err),
				zap.Duration("failed_duration", time.Since(searchStart)),
			)
			// Fallback to standard search
			similarChunks, err = s.searchSimilarChunks(ctx, queryVector, 15)
			if err != nil {
				logger.Get().Error("向量相似性搜索失败",
					zap.Error(err),
					zap.Duration("total_search_duration", time.Since(searchStart)),
				)
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to search similar chunks: %w", err))
			}
		}
	} else {
		logger.Get().Debug("使用标准向量搜索")
		// Standard vector similarity search
		similarChunks, err = s.searchSimilarChunks(ctx, queryVector, 15) // 获取更多候选用于重排
		if err != nil {
			logger.Get().Error("向量相似性搜索失败",
				zap.Error(err),
				zap.Duration("search_duration", time.Since(searchStart)),
			)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to search similar chunks: %w", err))
		}
	}

	searchDuration := time.Since(searchStart)
	logger.Get().Info("搜索完成",
		zap.Int("chunks_found", len(similarChunks)),
		zap.Duration("search_duration", searchDuration),
	)

	if logger.Get().Core().Enabled(zap.DebugLevel) {
		for i, chunk := range similarChunks {
			if i < 5 { // Only log top 5 for brevity
				logger.Get().Debug("搜索结果详情",
					zap.Int("rank", i+1),
					zap.String("chunk_id", chunk.ChunkID),
					zap.Float32("similarity", chunk.Similarity),
					zap.Int("content_length", len(chunk.Content)),
				)
			}
		}
	}
	if len(similarChunks) == 0 {
		logger.Get().Warn("未找到相关文档",
			zap.String("query", query),
			zap.Strings("keywords", keywords),
			zap.Duration("total_duration", time.Since(startTime)),
		)
		return connect.NewResponse(&ragv1.GetContextResponse{
			Context: fmt.Sprintf("未找到与查询 '%s' 相关的内容。请尝试使用不同的关键词。", query),
		}), nil
	}

	// 第四步：智能重排序 - 综合向量相似度和关键词匹配
	logger.Get().Debug("开始智能重排序",
		zap.Int("chunks_before", len(similarChunks)),
	)
	rerankStart := time.Now()
	rankedChunks := s.rerankChunksWithKeywords(similarChunks, query, keywords)
	rerankDuration := time.Since(rerankStart)

	logger.Get().Info("重排序完成",
		zap.Int("chunks_after", len(rankedChunks)),
		zap.Duration("rerank_duration", rerankDuration),
	)

	if logger.Get().Core().Enabled(zap.DebugLevel) && len(rankedChunks) > 0 {
		for i, chunk := range rankedChunks {
			if i < 3 { // Log top 3 reranked results
				logger.Get().Debug("重排序结果",
					zap.Int("final_rank", i+1),
					zap.String("chunk_id", chunk.ChunkID),
					zap.Float32("similarity", chunk.Similarity),
					zap.Any("advanced_score", chunk.Metadata["advanced_score"]),
				)
			}
		}
	}
	// 第五步：使用大模型生成个性化总结回答
	logger.Get().Debug("开始生成个性化回答",
		zap.String("query", query),
		zap.Int("chunks_count", len(rankedChunks)),
	)
	summaryStart := time.Now()
	contextContent, err := s.generateContextSummary(ctx, rankedChunks, query)
	summaryDuration := time.Since(summaryStart)

	if err != nil {
		logger.Get().Error("大模型总结生成失败，回退到模板回答",
			zap.Error(err),
			zap.Duration("failed_duration", summaryDuration),
		)
		// 降级到模板回答
		contextContent = s.buildContextResponse(rankedChunks, query)
	} else {
		logger.Get().Info("个性化回答生成成功",
			zap.Duration("summary_duration", summaryDuration),
			zap.Int("summary_length", len(contextContent)),
		)
	}

	// 计算处理时间
	processingTime := time.Since(startTime).Milliseconds()

	totalDuration := time.Since(startTime)
	logger.Get().Info("智能文档检索完成",
		zap.String("query", query),
		zap.Int("chunks_found", len(similarChunks)),
		zap.Int("chunks_used", len(rankedChunks)),
		zap.Int("response_length", len(contextContent)),
		zap.Int64("processing_time_ms", processingTime),
		zap.Duration("total_duration", totalDuration),
	)

	return connect.NewResponse(&ragv1.GetContextResponse{
		Context: contextContent,
	}), nil
}

// searchSimilarChunks performs semantic vector search using pgvector.
//
// This function searches for similar document chunks in PostgreSQL based on
// the query vector, using cosine similarity to calculate relevance.
// It returns the most relevant document fragments up to the specified limit.
//
// The similarity threshold of 0.3 filters out low-relevance results.
func (s *RagServer) searchSimilarChunks(ctx context.Context, queryVector []float32, limit int) ([]adapters.ChunkSearchResult, error) {
	// 使用数据库的向量搜索功能
	results, err := s.DB.SearchSimilarChunks(ctx, queryVector, limit, 0.3) // 0.3是相似度阈值
	if err != nil {
		return nil, fmt.Errorf("database search failed: %w", err)
	}

	logger.Get().Debug("Vector search completed",
		zap.Int("results_count", len(results)),
		zap.Int("query_vector_dim", len(queryVector)),
	)

	return results, nil
}

// rerankChunks reranks and filters search results based on multiple factors.
//
// This function is deprecated in favor of rerankChunksWithKeywords which
// provides more sophisticated ranking algorithms.
func (s *RagServer) rerankChunks(chunks []adapters.ChunkSearchResult, query string) []adapters.ChunkSearchResult {
	// 基于多个因素进行重排序
	sort.Slice(chunks, func(i, j int) bool {
		// 综合评分：相似度 + 内容长度 + 关键词匹配
		scoreI := s.calculateChunkScore(chunks[i], query)
		scoreJ := s.calculateChunkScore(chunks[j], query)
		return scoreI > scoreJ
	})

	// 过滤低质量结果，最多返回5个最相关的块
	maxChunks := 5
	if len(chunks) > maxChunks {
		chunks = chunks[:maxChunks]
	}

	// 进一步过滤：移除相似度过低的结果
	var filteredChunks []adapters.ChunkSearchResult
	for _, chunk := range chunks {
		if chunk.Similarity > 0.4 { // 相似度阈值
			filteredChunks = append(filteredChunks, chunk)
		}
	}

	logger.Get().Debug("Chunks reranked and filtered",
		zap.Int("original_count", len(chunks)),
		zap.Int("filtered_count", len(filteredChunks)),
	)

	return filteredChunks
}

// calculateChunkScore calculates a comprehensive score for a chunk.
//
// The scoring combines:
//   - Base similarity score (70% weight)
//   - Content length bonus for optimal lengths (10% weight)
//   - Keyword matching score (20% weight)
//
// This function is deprecated in favor of search.CalculateAdvancedScore.
func (s *RagServer) calculateChunkScore(chunk adapters.ChunkSearchResult, query string) float64 {
	// 基础相似度权重
	score := float64(chunk.Similarity) * 0.7

	// 内容长度加分（适中长度更好）
	contentLength := len(chunk.Content)
	if contentLength > 100 && contentLength < 1000 {
		score += 0.1
	}

	// 关键词匹配加分
	queryLower := strings.ToLower(query)
	contentLower := strings.ToLower(chunk.Content)

	// 简单的关键词匹配评分
	queryWords := strings.Fields(queryLower)
	matchCount := 0
	for _, word := range queryWords {
		if strings.Contains(contentLower, word) {
			matchCount++
		}
	}

	if len(queryWords) > 0 {
		keywordScore := float64(matchCount) / float64(len(queryWords))
		score += keywordScore * 0.2
	}

	return score
}

// buildContextResponse constructs a structured context response from chunks.
//
// This function formats the search results into a human-readable response
// with proper structure, including similarity scores and metadata.
// It serves as a fallback when LLM-based summarization is unavailable.
func (s *RagServer) buildContextResponse(chunks []adapters.ChunkSearchResult, query string) string {
	if len(chunks) == 0 {
		return fmt.Sprintf("未找到与查询 '%s' 相关的内容。", query)
	}

	var contextBuilder strings.Builder

	// 添加查询总结
	contextBuilder.WriteString(fmt.Sprintf("基于查询「%s」找到以下相关内容：\n\n", query))

	// 添加相关内容块
	for i, chunk := range chunks {
		contextBuilder.WriteString(fmt.Sprintf("## 相关内容 %d (相似度: %.2f)\n", i+1, chunk.Similarity))

		// 清理和格式化内容
		cleanContent := s.cleanAndFormatChunkContent(chunk.Content)
		contextBuilder.WriteString(cleanContent)
		contextBuilder.WriteString("\n\n")

		// 添加元数据信息
		if chunk.Metadata != nil {
			if chunkType, ok := chunk.Metadata["chunk_type"].(string); ok && chunkType != "" {
				contextBuilder.WriteString(fmt.Sprintf("*[内容类型: %s]*\n\n", chunkType))
			}
		}
	}

	// 添加使用说明
	contextBuilder.WriteString("---\n")
	contextBuilder.WriteString("💡 提示：以上内容按相关性排序，您可以基于这些信息进行进一步的询问。")

	return contextBuilder.String()
}

// cleanAndFormatChunkContent cleans and formats chunk content for display.
//
// This is a wrapper around utils.CleanAndFormatContent with a fixed
// maximum length of 2000 bytes.
func (s *RagServer) cleanAndFormatChunkContent(content string) string {
	return utils.CleanAndFormatContent(content, 2000)
}

// generateKeywords extracts keywords from the query using LLM.
//
// This function calls the configured LLM service (e.g., DeepSeek) to perform
// intelligent keyword extraction with Chinese word segmentation.
// It automatically filters stop words and preserves technical terms and entities.
//
// If the LLM call fails, it falls back to basic local tokenization using
// utils.ExtractBasicKeywords.
//
// The function expects XML-formatted output from the LLM for structured parsing.
func (s *RagServer) generateKeywords(_ context.Context, query string) ([]string, error) {
	// Use prompt manager to get the keyword extraction prompt
	if s.promptEmbeddingService != nil {
		prompt, _, err := s.promptEmbeddingService.GetPromptWithEmbedding(prompts.PromptTypeKeywordExtraction)
		if err == nil {
			// Render the user prompt with the query
			userContent, err := s.promptEmbeddingService.GetPromptManager().RenderUserPrompt(
				prompts.PromptTypeKeywordExtraction,
				map[string]string{"query": query},
			)
			if err == nil {
				messages := []openai.Message{
					{
						Role:    "system",
						Content: prompt.System,
					},
					{
						Role:    "user",
						Content: userContent,
					},
				}

				resp, err := s.LLM.CreateChatCompletionWithDefaults(s.Config.Services.LLM.Model, messages)
				if err != nil {
					logger.Get().Error("LLM关键词提取失败", zap.Error(err))
					return utils.ExtractBasicKeywords(query), nil
				}

				if len(resp.Choices) == 0 {
					return utils.ExtractBasicKeywords(query), nil
				}

				logger.Get().Info("关键词 LLM", zap.Any("resp", resp))
				content := resp.Choices[0].Message.Content
				keywords := s.parseKeywordsXML(content)

				if len(keywords) == 0 {
					// Fallback to line-based parsing if XML parsing fails
					lines := strings.Split(strings.TrimSpace(content), "\n")
					for _, line := range lines {
						keyword := strings.TrimSpace(line)
						if strings.HasPrefix(keyword, "<") && strings.HasSuffix(keyword, ">") {
							continue
						}
						if keyword != "" && len(keyword) > 1 {
							keywords = append(keywords, keyword)
						}
					}
				}

				if len(keywords) == 0 {
					return utils.ExtractBasicKeywords(query), nil
				}

				return keywords, nil
			}
		}
	}

	// Fallback to direct prompt manager if prompt embedding service is not available
	if pm := prompts.NewPromptManager(); pm != nil {
		prompt, err := pm.GetPrompt(prompts.PromptTypeKeywordExtraction)
		if err == nil {
			userContent, err := pm.RenderUserPrompt(
				prompts.PromptTypeKeywordExtraction,
				map[string]string{"query": query},
			)
			if err == nil {
				messages := []openai.Message{
					{
						Role:    "system",
						Content: prompt.System,
					},
					{
						Role:    "user",
						Content: userContent,
					},
				}

				resp, err := s.LLM.CreateChatCompletionWithDefaults(s.Config.Services.LLM.Model, messages)

				if err != nil {
					logger.Get().Error("LLM关键词提取失败", zap.Error(err))
					// 降级为简单分词
					return utils.ExtractBasicKeywords(query), nil
				}

				if len(resp.Choices) == 0 {
					return utils.ExtractBasicKeywords(query), nil
				}

				logger.Get().Info("关键词 LLM", zap.Any("resp", resp))
				// 解析LLM返回的XML格式关键词
				content := resp.Choices[0].Message.Content
				keywords := s.parseKeywordsXML(content)

				if len(keywords) == 0 {
					// 如果XML解析失败，尝试按行解析（兼容旧格式）
					lines := strings.Split(strings.TrimSpace(content), "\n")
					for _, line := range lines {
						keyword := strings.TrimSpace(line)
						// 跳过XML标签
						if strings.HasPrefix(keyword, "<") && strings.HasSuffix(keyword, ">") {
							continue
						}
						if keyword != "" && len(keyword) > 1 {
							keywords = append(keywords, keyword)
						}
					}
				}

				if len(keywords) == 0 {
					return utils.ExtractBasicKeywords(query), nil
				}

				return keywords, nil
			}
		}
	}

	// Final fallback to basic keywords if all else fails
	return utils.ExtractBasicKeywords(query), nil

}

// parseKeywordsXML parses XML-formatted keyword response from LLM.
//
// This function extracts keywords from XML tags in the format:
//
//	<keyword>word</keyword>
//
// It handles both well-formed XML and loosely formatted responses,
// providing robust parsing even when the LLM output is imperfect.
func (s *RagServer) parseKeywordsXML(xmlContent string) []string {
	var keywords []string

	// 简单的XML解析，提取<keyword>标签内容
	lines := strings.Split(xmlContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 查找 <keyword>xxx</keyword> 模式
		if strings.HasPrefix(line, "<keyword>") && strings.HasSuffix(line, "</keyword>") {
			// 提取标签之间的内容
			start := len("<keyword>")
			end := len(line) - len("</keyword>")
			if end > start {
				keyword := strings.TrimSpace(line[start:end])
				// 验证关键词有效性
				if keyword != "" && len(keyword) > 1 && !strings.Contains(keyword, "<") {
					keywords = append(keywords, keyword)
				}
			}
		}
	}

	// 如果没有找到标准格式，尝试更宽松的解析
	if len(keywords) == 0 {
		// 使用正则表达式查找所有<keyword>内容</keyword>模式
		content := strings.ReplaceAll(xmlContent, "\n", " ")
		parts := strings.Split(content, "<keyword>")
		for _, part := range parts {
			if idx := strings.Index(part, "</keyword>"); idx > 0 {
				keyword := strings.TrimSpace(part[:idx])
				if keyword != "" && len(keyword) > 1 && !strings.Contains(keyword, "<") {
					keywords = append(keywords, keyword)
				}
			}
		}
	}

	return keywords
}

// rerankChunksWithKeywords performs intelligent reranking using a hybrid algorithm.
//
// This function delegates to search.RerankChunksWithKeywords which combines:
//   - Vector similarity (40% weight)
//   - Keyword matching (30% weight)
//   - Phrase matching (20% weight)
//   - Content quality (10% weight)
//
// Parameters are configured with maxChunks=5 and minSimilarity=0.25.
func (s *RagServer) rerankChunksWithKeywords(chunks []adapters.ChunkSearchResult, query string, keywords []string) []adapters.ChunkSearchResult {
	return search.RerankChunksWithKeywords(chunks, query, keywords, 5, 0.25)
}

// calculateAdvancedChunkScore calculates multi-dimensional scoring for a chunk.
//
// This function delegates to search.CalculateAdvancedScore for the actual
// scoring implementation.
func (s *RagServer) calculateAdvancedChunkScore(chunk adapters.ChunkSearchResult, query string, keywords []string) float64 {
	return search.CalculateAdvancedScore(chunk, query, keywords)
}

// generateContextSummary generates an intelligent context summary using LLM.
//
// Based on retrieved document chunks, this function uses the LLM to perform
// deep analysis and intelligent summarization, generating high-quality,
// structured responses tailored to the user's query.
//
// The function expects XML-formatted output from the LLM and falls back to
// generateBasicContextSummary if the LLM is unavailable or returns an error.
func (s *RagServer) generateContextSummary(ctx context.Context, chunks []adapters.ChunkSearchResult, query string) (string, error) {
	if len(chunks) == 0 {
		return "", fmt.Errorf("no chunks to summarize")
	}

	// Build raw context for LLM analysis
	rawContextBuilder := strings.Builder{}
	rawContextBuilder.WriteString("以下是从知识库检索到的相关信息：\n\n")

	for i, chunk := range chunks {
		cleanContent := s.cleanAndFormatChunkContent(chunk.Content)
		rawContextBuilder.WriteString(fmt.Sprintf("**信息片段%d (相似度: %.3f):**\n", i+1, chunk.Similarity))
		rawContextBuilder.WriteString(cleanContent)

		if chunk.Metadata != nil {
			if chunkType, ok := chunk.Metadata["chunk_type"].(string); ok && chunkType != "" {
				rawContextBuilder.WriteString(fmt.Sprintf("\n*[类型: %s]*", chunkType))
			}
		}
		rawContextBuilder.WriteString("\n\n")
	}

	var messages []openai.Message

	// Try to use prompt manager for context summary
	if s.promptEmbeddingService != nil {
		prompt, _, err := s.promptEmbeddingService.GetPromptWithEmbedding(prompts.PromptTypeContextSummary)
		if err == nil {
			userContent, err := s.promptEmbeddingService.GetPromptManager().RenderUserPrompt(
				prompts.PromptTypeContextSummary,
				map[string]string{
					"query":   query,
					"context": rawContextBuilder.String(),
				},
			)
			if err == nil {
				messages = []openai.Message{
					{
						Role:    "system",
						Content: prompt.System,
					},
					{
						Role:    "user",
						Content: userContent,
					},
				}
			}
		}
	}

	// Fallback to direct prompt manager if prompt service is not available
	if len(messages) == 0 {
		if pm := prompts.NewPromptManager(); pm != nil {
			prompt, err := pm.GetPrompt(prompts.PromptTypeContextSummary)
			if err == nil {
				userContent, err := pm.RenderUserPrompt(
					prompts.PromptTypeContextSummary,
					map[string]string{
						"query":   query,
						"context": rawContextBuilder.String(),
					},
				)
				if err == nil {
					messages = []openai.Message{
						{
							Role:    "system",
							Content: prompt.System,
						},
						{
							Role:    "user",
							Content: userContent,
						},
					}
				}
			}
		}
		// If prompt manager fails, return error
		if len(messages) == 0 {
			return s.generateBasicContextSummary(chunks, query), nil
		}
	}

	// 调用LLM进行智能总结
	resp, err := s.LLM.CreateChatCompletionWithDefaults(s.Config.Services.LLM.Model, messages)
	if err != nil {
		logger.Get().Error("LLM智能总结失败，回退到基础模板", zap.Error(err))
		// 降级到基础模板方案
		return s.generateBasicContextSummary(chunks, query), nil
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		logger.Get().Warn("LLM返回空内容，回退到基础模板")
		return s.generateBasicContextSummary(chunks, query), nil
	}

	intelligentSummary := resp.Choices[0].Message.Content

	// 添加系统标识和使用说明
	var finalSummary strings.Builder
	finalSummary.WriteString(intelligentSummary)
	finalSummary.WriteString("\n\n---\n\n")
	finalSummary.WriteString("提示: 以上回答基于知识库检索结果生成，如需了解更详细信息，可以尝试调整查询关键词或提出更具体的问题。")

	logger.Get().Info("LLM智能总结生成成功",
		zap.String("query", query),
		zap.Int("chunks_count", len(chunks)),
		zap.Int("summary_length", len(intelligentSummary)),
	)

	return finalSummary.String(), nil
}

// generateBasicContextSummary provides basic template-based summarization.
//
// This is a fallback solution when the LLM service is unavailable.
// It provides basic information organization and formatting using
// predefined templates and query type analysis.
func (s *RagServer) generateBasicContextSummary(chunks []adapters.ChunkSearchResult, query string) string {
	var contextBuilder strings.Builder

	contextBuilder.WriteString(fmt.Sprintf("## 关于「%s」的相关信息\n\n", query))

	// 智能分析查询类型，提供针对性的引导
	queryType := s.analyzeQueryType(query)
	contextBuilder.WriteString(s.generateQueryTypeGuidance(queryType))

	// 添加检索到的信息
	contextBuilder.WriteString("### 📚 检索到的相关内容\n\n")

	for i, chunk := range chunks {
		cleanContent := s.cleanAndFormatChunkContent(chunk.Content)

		contextBuilder.WriteString(fmt.Sprintf("**%d. 相关信息** (相似度: %.2f)\n\n", i+1, chunk.Similarity))
		contextBuilder.WriteString(cleanContent)
		contextBuilder.WriteString("\n\n")

		// 添加简单的元数据
		if chunk.Metadata != nil {
			if chunkType, ok := chunk.Metadata["chunk_type"].(string); ok && chunkType != "" {
				contextBuilder.WriteString(fmt.Sprintf("*信息类型: %s*\n\n", chunkType))
			}
		}
	}

	// 添加智能总结
	contextBuilder.WriteString("### 💡 信息总结\n\n")
	contextBuilder.WriteString("基于以上检索结果，这些信息涵盖了您查询的相关方面。")
	contextBuilder.WriteString(s.generateQuerySpecificSummary(query, chunks))
	contextBuilder.WriteString("\n\n")

	contextBuilder.WriteString("如需了解更详细的信息，建议您：\n")
	contextBuilder.WriteString("- 查看上述具体的信息片段\n")
	contextBuilder.WriteString("- 尝试使用更具体的关键词重新查询\n")
	contextBuilder.WriteString("- 提出更详细的问题以获得精准答案\n")

	return contextBuilder.String()
}

// analyzeQueryType analyzes the semantic type of a query.
//
// This function delegates to utils.AnalyzeQueryType for the actual analysis.
func (s *RagServer) analyzeQueryType(query string) utils.QueryType {
	return utils.AnalyzeQueryType(query)
}

// generateQueryTypeGuidance generates guidance text based on query type.
//
// This function delegates to utils.GetQueryTypeGuidance for the actual
// guidance generation.
func (s *RagServer) generateQueryTypeGuidance(queryType utils.QueryType) string {
	return utils.GetQueryTypeGuidance(queryType)
}

// generateQuerySpecificSummary generates a query-specific summary.
//
// This function delegates to utils.GetQuerySpecificSummary for the actual
// summary generation based on the analyzed query type.
func (s *RagServer) generateQuerySpecificSummary(query string, _ []adapters.ChunkSearchResult) string {
	queryType := utils.AnalyzeQueryType(query)
	return utils.GetQuerySpecificSummary(queryType)
}

// generateSmartResponse generates template-based fallback responses.
//
// This function provides a fallback response generation mechanism when
// the LLM service is unavailable. It uses predefined templates and rules
// to generate structured responses, ensuring users always receive useful
// information even when advanced services are down.
// generateEmbedding generates embeddings for the given text using the configured embedding service.
//
// This function uses the embedding client to convert text into vector representations
// that can be used for semantic similarity searches. It handles the conversion from
// the embedding response format to the expected []float32 format.
//
// Returns an error if the embedding service is unavailable or if the embedding
// generation fails.

func (s *RagServer) generateSmartResponse(query string, chunks []adapters.ChunkSearchResult) string {
	var responseBuilder strings.Builder

	// 添加问题理解
	responseBuilder.WriteString(fmt.Sprintf("关于「%s」，我在相关文档中找到以下信息：\n\n", query))

	// 分析和总结内容
	for i, chunk := range chunks {
		content := s.cleanAndFormatChunkContent(chunk.Content)
		responseBuilder.WriteString(fmt.Sprintf("**%d. 相关信息 (相似度: %.2f)**\n", i+1, chunk.Similarity))
		responseBuilder.WriteString(content)
		responseBuilder.WriteString("\n\n")
	}

	// 添加智能总结
	responseBuilder.WriteString("**总结：**\n")
	responseBuilder.WriteString("基于以上文档内容，这些信息涵盖了您询问的主要方面。")

	// 根据查询类型添加不同的建议
	queryLower := strings.ToLower(query)
	if strings.Contains(queryLower, "经验") || strings.Contains(queryLower, "工作") {
		responseBuilder.WriteString("从工作经验的角度来看，文档中提到的相关背景和实践经验可以为您提供参考。")
	} else if strings.Contains(queryLower, "技术") || strings.Contains(queryLower, "开发") {
		responseBuilder.WriteString("从技术角度分析，相关的技术栈和开发经验在文档中有详细描述。")
	} else if strings.Contains(queryLower, "项目") {
		responseBuilder.WriteString("项目相关的信息显示了具体的实施经验和成果。")
	}

	responseBuilder.WriteString("\n\n💡 如需了解更具体的信息，建议您查看上述相关内容或提出更详细的问题。")

	return responseBuilder.String()
}
