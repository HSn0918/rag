package server

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/hsn0918/rag/internal/adapters"
	"github.com/hsn0918/rag/internal/clients/openai"
	ragv1 "github.com/hsn0918/rag/internal/gen/rag/v1"
	"github.com/hsn0918/rag/internal/logger"
	"go.uber.org/zap"
)

// GetContext 实现智能文档检索和问答功能
//
// 完整的RAG(Retrieval-Augmented Generation)流程:
// 1. 使用大模型提取用户查询中的关键词
// 2. 生成查询向量进行语义搜索
// 3. 对搜索结果进行智能重排序
// 4. 使用大模型生成个性化回答
func (s *RagServer) GetContext(
	ctx context.Context,
	req *connect.Request[ragv1.GetContextRequest],
) (*connect.Response[ragv1.GetContextResponse], error) {
	query := req.Msg.GetQuery()

	if query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("query is required"))
	}

	logger.GetLogger().Info("开始处理智能文档检索请求",
		zap.String("query", query),
		zap.Int("query_length", len(query)),
	)

	// 第一步：使用大模型进行智能分词和关键词提取
	keywords, err := s.generateKeywords(ctx, query)
	if err != nil {
		logger.GetLogger().Error("大模型关键词提取失败", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate keywords: %w", err))
	}
	logger.GetLogger().Debug("大模型关键词提取完成",
		zap.Strings("keywords", keywords),
	)

	// 第二步：生成语义向量进行相似性搜索
	// 优先使用提取的关键词，回退到原始查询
	queryText := strings.Join(keywords, " ")
	if queryText == "" {
		queryText = query
	}
	queryVector, err := s.generateEmbedding(ctx, queryText)
	if err != nil {
		logger.GetLogger().Error("查询向量生成失败", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate query embedding: %w", err))
	}

	// 第三步：执行向量相似性搜索，获取候选文档块
	similarChunks, err := s.searchSimilarChunks(ctx, queryVector, 15) // 获取更多候选用于重排
	if err != nil {
		logger.GetLogger().Error("向量相似性搜索失败", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to search similar chunks: %w", err))
	}

	if len(similarChunks) == 0 {
		logger.GetLogger().Warn("未找到相关文档", zap.String("query", query))
		return connect.NewResponse(&ragv1.GetContextResponse{
			Context: fmt.Sprintf("未找到与查询 '%s' 相关的内容。请尝试使用不同的关键词。", query),
		}), nil
	}

	// 第四步：智能重排序 - 综合向量相似度和关键词匹配
	rankedChunks := s.rerankChunksWithKeywords(similarChunks, query, keywords)

	// 第五步：使用大模型生成个性化总结回答
	contextContent, err := s.generateContextSummary(ctx, rankedChunks, query)
	if err != nil {
		logger.GetLogger().Error("大模型总结生成失败", zap.Error(err))
		// 降级到模板回答
		contextContent = s.buildContextResponse(rankedChunks, query)
	}

	logger.GetLogger().Info("智能文档检索完成",
		zap.String("query", query),
		zap.Int("chunks_found", len(similarChunks)),
		zap.Int("chunks_used", len(rankedChunks)),
		zap.Int("response_length", len(contextContent)),
	)

	return connect.NewResponse(&ragv1.GetContextResponse{
		Context: contextContent,
	}), nil
}

// searchSimilarChunks 使用pgvector执行语义向量搜索
//
// 基于查询向量在PostgreSQL数据库中搜索相似的文档块，
// 使用余弦相似度算法计算相关性，返回最相关的文档片段
func (s *RagServer) searchSimilarChunks(ctx context.Context, queryVector []float32, limit int) ([]adapters.ChunkSearchResult, error) {
	// 使用数据库的向量搜索功能
	results, err := s.DB.SearchSimilarChunks(ctx, queryVector, limit, 0.3) // 0.3是相似度阈值
	if err != nil {
		return nil, fmt.Errorf("database search failed: %w", err)
	}

	logger.GetLogger().Debug("Vector search completed",
		zap.Int("results_count", len(results)),
		zap.Int("query_vector_dim", len(queryVector)),
	)

	return results, nil
}

// rerankChunks 对搜索结果进行重排序和过滤
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
		if chunk.
			Similarity > 0.4 { // 相似度阈值
			filteredChunks = append(filteredChunks, chunk)
		}
	}

	logger.GetLogger().Debug("Chunks reranked and filtered",
		zap.Int("original_count", len(chunks)),
		zap.Int("filtered_count", len(filteredChunks)),
	)

	return filteredChunks
}

// calculateChunkScore 计算分块的综合评分
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

// buildContextResponse 构建结构化的上下文响应
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

// cleanAndFormatChunkContent 清理和格式化分块内容
func (s *RagServer) cleanAndFormatChunkContent(content string) string {
	// 基本清理
	content = strings.TrimSpace(content)

	// 移除多余的换行符
	lines := strings.Split(content, "\n")
	var cleanedLines []string

	lastWasEmpty := false
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		if trimmedLine == "" {
			if !lastWasEmpty {
				cleanedLines = append(cleanedLines, "")
			}
			lastWasEmpty = true
		} else {
			cleanedLines = append(cleanedLines, trimmedLine)
			lastWasEmpty = false
		}
	}

	// 确保内容不会太长
	result := strings.Join(cleanedLines, "\n")
	if len(result) > 2000 {
		result = s.safeUTF8Truncate(result, 1000) + "..."
	}

	// 确保结果是有效的UTF-8
	result = s.sanitizeUTF8(result)

	return result
}

// generateKeywords 使用大模型智能提取查询关键词
//
// 调用配置的LLM服务(如DeepSeek)进行中文分词和关键词提取，
// 自动过滤停用词，保留专业术语和实体名词，
// 如果LLM调用失败会降级到本地简单分词
func (s *RagServer) generateKeywords(ctx context.Context, query string) ([]string, error) {
	messages := []openai.Message{
		{
			Role: "system",
			Content: `你是一个中文关键词提取专家。请从用户输入的查询中提取最重要的关键词。

要求：
1. 提取3-8个最相关的关键词
2. 忽略停词（如：的、了、在、是、我、有、和等）
3. 保留专业术语和实体名词
4. 每行一个关键词，不要编号
5. 不要添加任何解释或其他内容

示例：
输入："请帮我找一下关于机器学习算法的资料"
输出：
机器学习
算法
资料`,
		},
		{
			Role:    "user",
			Content: query,
		},
	}

	resp, err := s.LLM.CreateChatCompletionWithDefaults(s.Config.Services.LLM.Model, messages)
	if err != nil {
		logger.GetLogger().Error("LLM关键词提取失败", zap.Error(err))
		// 降级为简单分词
		return s.fallbackKeywords(query), nil
	}

	if len(resp.Choices) == 0 {
		return s.fallbackKeywords(query), nil
	}

	// 解析LLM返回的关键词
	content := resp.Choices[0].Message.Content
	lines := strings.Split(strings.TrimSpace(content), "\n")
	var keywords []string

	for _, line := range lines {
		keyword := strings.TrimSpace(line)
		if keyword != "" && len(keyword) > 1 {
			keywords = append(keywords, keyword)
		}
	}

	if len(keywords) == 0 {
		return s.fallbackKeywords(query), nil
	}

	return keywords, nil
}

// fallbackKeywords 本地降级分词实现
//
// 当LLM服务不可用时的备用方案，使用基于字符规则的简单中文分词，
// 包含基础停用词过滤，确保服务的可用性
func (s *RagServer) fallbackKeywords(query string) []string {
	// 简单的中文分词作为降级方案
	stopWords := map[string]bool{
		"的": true, "了": true, "在": true, "是": true, "我": true, "有": true, "和": true,
		"就": true, "不": true, "人": true, "都": true, "一": true, "一个": true, "上": true,
		"也": true, "很": true, "到": true, "说": true, "要": true, "去": true, "你": true,
		"会": true, "着": true, "没有": true, "看": true, "好": true, "自己": true, "这": true,
	}

	var keywords []string
	runes := []rune(query)
	var currentWord []rune

	for _, r := range runes {
		if (r >= 0x4e00 && r <= 0x9fff) || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			currentWord = append(currentWord, r)
		} else {
			if len(currentWord) > 0 {
				word := string(currentWord)
				if len(word) > 1 && !stopWords[word] {
					keywords = append(keywords, word)
				}
				currentWord = nil
			}
		}
	}

	if len(currentWord) > 0 {
		word := string(currentWord)
		if len(word) > 1 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// rerankChunksWithKeywords 混合算法智能重排序
//
// 综合向量相似度(40%)、关键词匹配(30%)、短语匹配(20%)和内容质量(10%)
// 对搜索结果进行重新排序，提升相关性和准确性
func (s *RagServer) rerankChunksWithKeywords(chunks []adapters.ChunkSearchResult, query string, keywords []string) []adapters.ChunkSearchResult {
	// 为每个chunk计算综合评分
	for i := range chunks {
		score := s.calculateAdvancedChunkScore(chunks[i], query, keywords)
		// 将score存储在metadata中（临时方案）
		if chunks[i].Metadata == nil {
			chunks[i].Metadata = make(map[string]interface{})
		}
		chunks[i].Metadata["advanced_score"] = score
	}

	// 按综合评分重排序
	sort.Slice(chunks, func(i, j int) bool {
		scoreI, _ := chunks[i].Metadata["advanced_score"].(float64)
		scoreJ, _ := chunks[j].Metadata["advanced_score"].(float64)
		return scoreI > scoreJ
	})

	// 过滤低质量结果
	maxChunks := 5
	if len(chunks) > maxChunks {
		chunks = chunks[:maxChunks]
	}

	var filteredChunks []adapters.ChunkSearchResult
	for _, chunk := range chunks {
		if chunk.Similarity > 0.25 { // 降低阈值以获得更多候选
			filteredChunks = append(filteredChunks, chunk)
		}
	}

	logger.GetLogger().Debug("Advanced reranking completed",
		zap.Int("original_count", len(chunks)),
		zap.Int("filtered_count", len(filteredChunks)),
		zap.Strings("keywords", keywords),
	)

	return filteredChunks
}

// calculateAdvancedChunkScore 多维度评分算法
//
// 权重分配: 向量相似度40% + 关键词匹配30% + 短语匹配20% + 内容质量10%
// 确保既考虑语义相似性，又兼顾精确匹配和内容可读性
func (s *RagServer) calculateAdvancedChunkScore(chunk adapters.ChunkSearchResult, query string, keywords []string) float64 {
	// 基础向量相似度 (40%)
	score := float64(chunk.Similarity) * 0.4

	contentLower := strings.ToLower(chunk.Content)

	// 精确关键词匹配 (30%)
	keywordScore := 0.0
	if len(keywords) > 0 {
		matchCount := 0
		for _, keyword := range keywords {
			if strings.Contains(contentLower, strings.ToLower(keyword)) {
				matchCount++
			}
		}
		keywordScore = float64(matchCount) / float64(len(keywords))
	}
	score += keywordScore * 0.3

	// 短语匹配 (20%)
	queryLower := strings.ToLower(query)
	if strings.Contains(contentLower, queryLower) {
		score += 0.2 // 完整查询短语匹配奖励
	}

	// 内容质量评分 (10%)
	contentLength := len(chunk.Content)
	if contentLength > 100 && contentLength < 1500 {
		score += 0.1
	} else if contentLength > 50 {
		score += 0.05
	}

	return score
}

// generateContextSummary 使用LLM智能生成上下文总结
//
// 基于检索到的文档片段，使用大模型进行深度分析和智能总结，
// 生成针对用户查询的高质量、结构化回答
func (s *RagServer) generateContextSummary(ctx context.Context, chunks []adapters.ChunkSearchResult, query string) (string, error) {
	if len(chunks) == 0 {
		return "", fmt.Errorf("no chunks to summarize")
	}

	// 首先构建原始上下文信息供LLM分析
	rawContextBuilder := strings.Builder{}
	rawContextBuilder.WriteString("以下是从知识库检索到的相关信息：\n\n")

	for i, chunk := range chunks {
		cleanContent := s.cleanAndFormatChunkContent(chunk.Content)
		rawContextBuilder.WriteString(fmt.Sprintf("**信息片段%d (相似度: %.3f):**\n", i+1, chunk.Similarity))
		rawContextBuilder.WriteString(cleanContent)

		// 添加元数据信息
		if chunk.Metadata != nil {
			if chunkType, ok := chunk.Metadata["chunk_type"].(string); ok && chunkType != "" {
				rawContextBuilder.WriteString(fmt.Sprintf("\n*[类型: %s]*", chunkType))
			}
		}
		rawContextBuilder.WriteString("\n\n")
	}

	// 构建LLM总结提示词
	messages := []openai.Message{
		{
			Role: "system",
			Content: `你是一个专业的信息分析师，擅长根据用户查询对多个信息源进行智能总结和分析。

你的任务是：
1. 仔细阅读用户的查询和检索到的相关信息
2. 分析信息之间的关联性和互补性
3. 生成一个结构化、准确、有用的回答
4. 如果信息不足以完全回答查询，请诚实说明
5. 适当引用具体的信息来源以增加可信度

回答格式要求：
- 使用清晰的Markdown格式
- 重点信息用**粗体**突出
- 分点说明时使用有序或无序列表
- 保持逻辑清晰、语言简洁
- 长度控制在800字以内`,
		},
		{
			Role: "user",
			Content: fmt.Sprintf(`用户查询：%s

相关信息：
%s

请基于以上信息，为用户提供准确、有用的回答。如果检索到的信息不足以完全回答用户的查询，请明确指出信息不足的地方，并建议用户如何获得更完整的答案。`, query, rawContextBuilder.String()),
		},
	}

	// 调用LLM进行智能总结
	resp, err := s.LLM.CreateChatCompletionWithDefaults(s.Config.Services.LLM.Model, messages)
	if err != nil {
		logger.GetLogger().Error("LLM智能总结失败，回退到基础模板", zap.Error(err))
		// 降级到基础模板方案
		return s.generateBasicContextSummary(chunks, query), nil
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		logger.GetLogger().Warn("LLM返回空内容，回退到基础模板")
		return s.generateBasicContextSummary(chunks, query), nil
	}

	intelligentSummary := resp.Choices[0].Message.Content

	// 添加系统标识和使用说明
	var finalSummary strings.Builder
	finalSummary.WriteString(intelligentSummary)
	finalSummary.WriteString("\n\n---\n\n")
	finalSummary.WriteString("💡 **提示**: 以上回答基于知识库检索结果生成，如需了解更详细信息，可以尝试调整查询关键词或提出更具体的问题。")

	logger.GetLogger().Info("LLM智能总结生成成功",
		zap.String("query", query),
		zap.Int("chunks_count", len(chunks)),
		zap.Int("summary_length", len(intelligentSummary)),
	)

	return finalSummary.String(), nil
}

// generateBasicContextSummary 基础模板总结（降级方案）
//
// 当LLM服务不可用时的降级方案，提供基础的信息整理和格式化
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

// analyzeQueryType 分析查询类型
func (s *RagServer) analyzeQueryType(query string) string {
	queryLower := strings.ToLower(query)

	// 定义查询类型的关键词模式
	patterns := map[string][]string{
		"how_to":     {"怎么", "如何", "怎样", "怎么办", "how to", "how do"},
		"what_is":    {"什么是", "是什么", "what is", "define", "定义"},
		"why":        {"为什么", "为啥", "原因", "why", "because"},
		"comparison": {"比较", "对比", "区别", "差异", "vs", "versus", "compare"},
		"list":       {"有哪些", "包括", "种类", "类型", "list", "types"},
		"experience": {"经验", "心得", "体会", "感受", "experience"},
		"technical":  {"技术", "算法", "架构", "实现", "技术栈", "technical"},
		"project":    {"项目", "工程", "系统", "应用", "project"},
	}

	for queryType, keywords := range patterns {
		for _, keyword := range keywords {
			if strings.Contains(queryLower, keyword) {
				return queryType
			}
		}
	}

	return "general"
}

// generateQueryTypeGuidance 根据查询类型生成引导信息
func (s *RagServer) generateQueryTypeGuidance(queryType string) string {
	guidanceMap := map[string]string{
		"how_to":     "以下信息将帮助您了解具体的操作方法和步骤：\n\n",
		"what_is":    "以下信息将帮助您理解相关概念和定义：\n\n",
		"why":        "以下信息将帮助您了解相关的原因和背景：\n\n",
		"comparison": "以下信息将帮助您进行比较和分析：\n\n",
		"list":       "以下信息列出了相关的项目和分类：\n\n",
		"experience": "以下是相关的经验分享和实践心得：\n\n",
		"technical":  "以下是相关的技术信息和实现细节：\n\n",
		"project":    "以下是相关的项目信息和实践案例：\n\n",
		"general":    "以下是与您查询相关的信息：\n\n",
	}

	return guidanceMap[queryType]
}

// generateQuerySpecificSummary 生成针对特定查询的总结
func (s *RagServer) generateQuerySpecificSummary(query string, chunks []adapters.ChunkSearchResult) string {
	queryType := s.analyzeQueryType(query)

	summaryMap := map[string]string{
		"how_to":     "从操作方法的角度来看，文档中提到的步骤和建议可以为您提供实用的指导。",
		"what_is":    "从概念定义的角度分析，相关的解释和说明在文档中有详细描述。",
		"why":        "从原因分析的角度来看，文档中提供了相关的背景信息和解释。",
		"comparison": "从比较分析的角度来看，不同方案的特点和差异在文档中有所体现。",
		"list":       "从分类整理的角度来看，相关项目的列举和说明在文档中比较全面。",
		"experience": "从实践经验的角度来看，文档中分享的经验和心得具有参考价值。",
		"technical":  "从技术角度分析，相关的技术栈、架构和实现方案在文档中有详细说明。",
		"project":    "从项目实施的角度来看，相关的项目经验和实践案例为您提供了有价值的参考。",
		"general":    "这些信息从多个角度为您的查询提供了相关的背景知识。",
	}

	return summaryMap[queryType]
}

// generateSmartResponse 模板化降级回答生成
//
// 当LLM服务不可用时的降级方案，基于预设模板和规则
// 生成结构化回答，确保用户始终能获得有用的响应
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

// safeUTF8Truncate 安全地截断UTF-8字符串，避免在多字节字符中间截断
func (s *RagServer) safeUTF8Truncate(str string, maxBytes int) string {
	if len(str) <= maxBytes {
		return str
	}

	// 确保不在多字节字符中间截断
	for i := maxBytes; i >= 0 && i > maxBytes-4; i-- {
		if utf8.ValidString(str[:i]) {
			return str[:i]
		}
	}

	// 如果找不到合适的截断点，使用rune级别截断
	runes := []rune(str)
	result := ""
	for _, r := range runes {
		test := result + string(r)
		if len(test) > maxBytes {
			break
		}
		result = test
	}

	return result
}

// sanitizeUTF8 清理并确保字符串包含有效的UTF-8字符
func (s *RagServer) sanitizeUTF8(str string) string {
	if utf8.ValidString(str) {
		return str
	}

	// 移除或替换无效的UTF-8字符
	var buf strings.Builder
	buf.Grow(len(str))

	for len(str) > 0 {
		r, size := utf8.DecodeRuneInString(str)
		if r == utf8.RuneError && size == 1 {
			// 跳过无效字节
			str = str[1:]
		} else {
			buf.WriteRune(r)
			str = str[size:]
		}
	}

	return buf.String()
}
