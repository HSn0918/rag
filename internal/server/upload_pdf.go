package server

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/hsn0918/rag/internal/chunking"
	"github.com/hsn0918/rag/internal/config"
	ragv1 "github.com/hsn0918/rag/internal/gen/rag/v1"
	"github.com/hsn0918/rag/internal/logger"
	"go.uber.org/zap"
)

// UploadPdf 接口的实现。
func (s *RagServer) UploadPdf(
	ctx context.Context,
	req *connect.Request[ragv1.UploadPdfRequest],
) (*connect.Response[ragv1.UploadPdfResponse], error) {
	// 获取文件键和文件名
	fileKey := req.Msg.GetFileKey()
	filename := req.Msg.GetFilename()

	if fileKey == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("file_key is required"))
	}

	if filename == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("filename is required"))
	}

	// 检查文件是否存在于 MinIO
	exists, err := s.Storage.CheckFileExists(ctx, fileKey)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check file existence: %w", err))
	}

	if !exists {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("file not found in storage: %s", fileKey))
	}

	// 从 MinIO 下载文件
	object, err := s.Storage.DownloadFile(ctx, fileKey)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to download file: %w", err))
	}
	defer func() {
		err := object.Close()
		if err != nil {
			logger.GetLogger().Error("failed to close file", zap.Error(err))
		}
	}()

	// 读取 PDF 数据
	pdfData, err := io.ReadAll(object)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read PDF data: %w", err))
	}

	if len(pdfData) == 0 {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("PDF file is empty"))
	}

	// 处理PDF并获取文本内容
	textContent, pageCount, err := s.processPDFWithCaching(ctx, pdfData)
	if err != nil {
		return nil, err
	}

	// 验证提取的文本内容
	if textContent == "" {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("no text extracted from PDF"))
	}

	// 清理文档中的空行
	textContent = s.cleanEmptyLines(textContent)

	// 获取处理UID，用于后续的元数据存储
	md5Hash := fmt.Sprintf("%x", md5.Sum(pdfData))
	doc2xUID := fmt.Sprintf("processed_%s", md5Hash)

	// 将文档存储到数据库
	docID, err := s.DB.StoreDocument(ctx, "uploaded_pdf", textContent, map[string]interface{}{
		"source":     "pdf_upload",
		"pages":      pageCount,
		"doc2x_uid":  doc2xUID,
		"md5_hash":   md5Hash,
		"created_at": time.Now(),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to store document: %w", err))
	}

	// 使用智能分块处理文本
	chunks, err := s.chunkTextContent(textContent)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to chunk text: %w", err))
	}

	successfulChunks := 0
	for i, chunk := range chunks {
		cleanContent := s.cleanText(chunk.Content)

		embeddingVec, err := s.generateEmbedding(ctx, cleanContent)
		if err != nil {
			logger.GetLogger().Error("Failed to generate embedding for chunk", zap.Int("chunk_id", i), zap.Error(err))
			continue
		}

		// 存储文本块和向量
		metadata := make(map[string]interface{})
		for k, v := range chunk.Metadata {
			metadata[k] = v
		}
		metadata["chunk_length"] = len(cleanContent)
		metadata["chunk_type"] = chunk.Type
		metadata["chunk_title"] = chunk.Title

		err = s.DB.StoreChunk(ctx, docID, i, cleanContent, embeddingVec, metadata)
		if err != nil {
			logger.GetLogger().Error("Failed to store chunk", zap.Int("chunk_id", i), zap.Error(err))
			continue
		}
		successfulChunks++
	}

	// 缓存文档信息
	err = s.Cache.CacheDocument(ctx, docID, map[string]interface{}{
		"title":     "uploaded_pdf",
		"content":   textContent,
		"doc2x_uid": doc2xUID,
		"md5_hash":  md5Hash,
		"chunks":    len(chunks),
	})
	if err != nil {
		logger.GetLogger().Error("Failed to cache document", zap.String("doc_id", docID), zap.Error(err))
	}

	return connect.NewResponse(&ragv1.UploadPdfResponse{
		Success:    true,
		Message:    fmt.Sprintf("PDF processed successfully. Document ID: %s, Chunks: %d", docID, successfulChunks),
		DocumentId: docID,
	}), nil
}

// chunkTextContent uses intelligent adaptive chunking
func (s *RagServer) chunkTextContent(content string) ([]chunking.Chunk, error) {
	chunkConfig := s.Config.Chunking

	// 检测内容类型并选择合适的分块策略
	if s.detectMarkdownContent(content) {
		// Markdown内容，使用AST-based chunking
		logger.GetLogger().Debug("Detected Markdown content, using AST-based chunking")
		chunker := chunking.NewMarkdownChunker(
			chunkConfig.MaxChunkSize,
			chunkConfig.OverlapSize,
			true,
		)
		chunks, err := chunker.ChunkMarkdown(content)
		if err != nil {
			// AST分块失败时，回退到智能文本分块
			logger.GetLogger().Warn("AST-based chunking failed, falling back to intelligent text chunking", zap.Error(err))
			return s.intelligentTextChunking(content), nil
		}
		return chunks, nil
	}

	// 非-Markdown内容，使用智能文本分块
	return s.intelligentTextChunking(content), nil
}

// intelligentTextChunking provides semantic-aware text chunking
func (s *RagServer) intelligentTextChunking(content string) []chunking.Chunk {
	chunkConfig := s.Config.Chunking

	// 自适应分块大小
	chunkSize := s.calculateOptimalChunkSize(content, chunkConfig)
	overlapSize := s.calculateOptimalOverlapSize(chunkSize, chunkConfig)

	if len(content) <= chunkSize {
		return []chunking.Chunk{{
			Content: content,
			Type:    "text",
			Level:   0,
			Title:   "Document",
			Metadata: map[string]string{
				"chunk_size": fmt.Sprintf("%d", len(content)),
				"adaptive":   fmt.Sprintf("%t", chunkConfig.AdaptiveSize),
			},
		}}
	}

	// 优先按段落分割
	if chunkConfig.ParagraphBoundary {
		return s.chunkByParagraphs(content, chunkSize, overlapSize)
	}

	// 优先按句子分割
	if chunkConfig.SentenceBoundary {
		return s.chunkBySentences(content, chunkSize, overlapSize)
	}

	// 简单按字符数分割（回退方案）
	return s.chunkByCharacters(content, chunkSize, overlapSize)
}

// calculateOptimalChunkSize calculates adaptive chunk size based on content
func (s *RagServer) calculateOptimalChunkSize(content string, config config.ChunkingConfig) int {
	if !config.AdaptiveSize {
		return config.MaxChunkSize
	}

	// 根据内容特征调整分块大小
	contentLength := len(content)
	baseSize := config.MaxChunkSize

	// 对于短文本，使用较小的分块
	if contentLength < baseSize*3 {
		return int(float64(baseSize) * 0.8)
	}

	// 对于结构化文本（包含多个段落），使用较大的分块
	paragraphCount := strings.Count(content, "\n\n") + 1
	if paragraphCount > 10 {
		return int(float64(baseSize) * config.SizeMultiplier)
	}

	return baseSize
}

// calculateOptimalOverlapSize calculates adaptive overlap size
func (s *RagServer) calculateOptimalOverlapSize(chunkSize int, config config.ChunkingConfig) int {
	// 重叠大小不超过分块大小的20%
	maxOverlap := chunkSize / 5
	if config.OverlapSize > maxOverlap {
		return maxOverlap
	}
	return config.OverlapSize
}

// chunkByParagraphs chunks content by paragraph boundaries
func (s *RagServer) chunkByParagraphs(content string, chunkSize, overlapSize int) []chunking.Chunk {
	paragraphs := strings.Split(content, "\n\n")
	var chunks []chunking.Chunk
	var currentChunk strings.Builder
	chunkIndex := 0

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		potentialContent := currentChunk.String()
		if potentialContent != "" {
			potentialContent += "\n\n"
		}
		potentialContent += para

		// 检查是否超过分块大小
		if len(potentialContent) > chunkSize && currentChunk.Len() > 0 {
			// 创建当前分块
			chunk := s.createChunk(currentChunk.String(), chunkIndex, "paragraph")
			chunks = append(chunks, chunk)

			// 开始新分块，带有重叠
			currentChunk.Reset()
			if overlapSize > 0 && len(chunks) > 0 {
				overlap := s.getLastNChars(chunks[len(chunks)-1].Content, overlapSize)
				if overlap != "" {
					currentChunk.WriteString(overlap)
					currentChunk.WriteString("\n\n")
				}
			}
			chunkIndex++
		}

		// 添加当前段落
		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n\n")
		}
		currentChunk.WriteString(para)
	}

	// 处理最后一个分块
	if currentChunk.Len() > 0 {
		chunk := s.createChunk(currentChunk.String(), chunkIndex, "paragraph")
		chunks = append(chunks, chunk)
	}

	return chunks
}

// chunkBySentences chunks content by sentence boundaries
func (s *RagServer) chunkBySentences(content string, chunkSize, overlapSize int) []chunking.Chunk {
	// 简单的句子分割（可以进一步优化）
	sentences := s.splitIntoSentences(content)
	var chunks []chunking.Chunk
	var currentChunk strings.Builder
	chunkIndex := 0

	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}

		potentialContent := currentChunk.String()
		if potentialContent != "" {
			potentialContent += " "
		}
		potentialContent += sentence

		if len(potentialContent) > chunkSize && currentChunk.Len() > 0 {
			chunk := s.createChunk(currentChunk.String(), chunkIndex, "sentence")
			chunks = append(chunks, chunk)

			currentChunk.Reset()
			if overlapSize > 0 && len(chunks) > 0 {
				overlap := s.getLastNChars(chunks[len(chunks)-1].Content, overlapSize)
				if overlap != "" {
					currentChunk.WriteString(overlap + " ")
				}
			}
			chunkIndex++
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString(" ")
		}
		currentChunk.WriteString(sentence)
	}

	if currentChunk.Len() > 0 {
		chunk := s.createChunk(currentChunk.String(), chunkIndex, "sentence")
		chunks = append(chunks, chunk)
	}

	return chunks
}

// chunkByCharacters provides fallback character-based chunking
func (s *RagServer) chunkByCharacters(content string, chunkSize, overlapSize int) []chunking.Chunk {
	var chunks []chunking.Chunk
	start := 0
	chunkIndex := 0

	for start < len(content) {
		end := start + chunkSize
		if end > len(content) {
			end = len(content)
		}

		chunk := s.createChunk(content[start:end], chunkIndex, "character")
		chunk.StartIndex = start
		chunk.EndIndex = end
		chunks = append(chunks, chunk)

		if end == len(content) {
			break
		}

		start = end - overlapSize
		if start <= 0 {
			start = end
		}
		chunkIndex++
	}

	return chunks
}

// splitIntoSentences splits text into sentences
func (s *RagServer) splitIntoSentences(text string) []string {
	// 简单的句子分割器（可以使用更复杂的NLP工具）
	text = strings.ReplaceAll(text, ". ", ".\n")
	text = strings.ReplaceAll(text, "! ", "!\n")
	text = strings.ReplaceAll(text, "? ", "?\n")
	text = strings.ReplaceAll(text, "。", "。\n")
	text = strings.ReplaceAll(text, "！", "！\n")
	text = strings.ReplaceAll(text, "？", "？\n")

	sentences := strings.Split(text, "\n")
	var result []string
	for _, sentence := range sentences {
		if trimmed := strings.TrimSpace(sentence); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// createChunk creates a standardized chunk
func (s *RagServer) createChunk(content string, index int, chunkType string) chunking.Chunk {
	return chunking.Chunk{
		Content: strings.TrimSpace(content),
		Type:    chunkType,
		Level:   0,
		Title:   fmt.Sprintf("%s chunk %d", strings.ToTitle(chunkType), index+1),
		Metadata: map[string]string{
			"chunk_index":  fmt.Sprintf("%d", index),
			"chunk_type":   chunkType,
			"chunk_length": fmt.Sprintf("%d", len(content)),
		},
	}
}

// getLastNChars gets the last N characters from text for overlap
func (s *RagServer) getLastNChars(text string, n int) string {
	if len(text) <= n {
		return text
	}

	// 尝试在单词边界截断
	start := len(text) - n
	for start > 0 && start < len(text) {
		if text[start] == ' ' || text[start] == '\n' {
			break
		}
		start--
	}

	return strings.TrimSpace(text[start:])
}

// cleanEmptyLines removes excessive consecutive empty lines while preserving content structure
func (s *RagServer) cleanEmptyLines(content string) string {
	lines := strings.Split(content, "\n")
	var cleanedLines []string
	consecutiveEmptyLines := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 跳过HTML注释和无意义的标签
		if strings.HasPrefix(trimmed, "<!--") && strings.HasSuffix(trimmed, "-->") {
			continue
		}
		if strings.HasPrefix(trimmed, "<img") || strings.HasPrefix(trimmed, "<Media>") {
			continue
		}

		// 保留所有有内容的行
		if trimmed != "" {
			// 清理行内多余空格，但保留基本格式
			cleanedLine := strings.TrimSpace(line)
			cleanedLines = append(cleanedLines, cleanedLine)
			consecutiveEmptyLines = 0
		} else {
			// 对于空行，在段落之间最多保留一个
			consecutiveEmptyLines++
			// 只在两个非空行之间保留一个空行
			if consecutiveEmptyLines == 1 && i > 0 && i < len(lines)-1 {
				// 检查前后是否都有内容
				hasPrevContent := false
				hasNextContent := false

				// 检查前面是否有内容
				for j := i - 1; j >= 0; j-- {
					if strings.TrimSpace(lines[j]) != "" {
						hasPrevContent = true
						break
					}
				}

				// 检查后面是否有内容
				for j := i + 1; j < len(lines); j++ {
					if strings.TrimSpace(lines[j]) != "" {
						hasNextContent = true
						break
					}
				}

				// 只有前后都有内容时才保留空行
				if hasPrevContent && hasNextContent {
					cleanedLines = append(cleanedLines, "")
				}
			}
		}
	}

	// 进一步清理：合并多个连续空行为单个空行
	var finalLines []string
	lastWasEmpty := false

	for _, line := range cleanedLines {
		if line == "" {
			if !lastWasEmpty {
				finalLines = append(finalLines, line)
				lastWasEmpty = true
			}
		} else {
			finalLines = append(finalLines, line)
			lastWasEmpty = false
		}
	}

	cleanedContent := strings.Join(finalLines, "\n")
	cleanedContent = strings.TrimSpace(cleanedContent)

	logger.GetLogger().Debug("Cleaned empty lines",
		zap.Int("original_lines", len(lines)),
		zap.Int("cleaned_lines", len(cleanedLines)),
		zap.Int("final_lines", len(finalLines)),
		zap.Int("original_length", len(content)),
		zap.Int("cleaned_length", len(cleanedContent)),
	)

	return cleanedContent
}

// detectMarkdownContent 智能检测内容是否为Markdown格式
func (s *RagServer) detectMarkdownContent(content string) bool {
	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	if totalLines == 0 {
		return false
	}

	var markdownFeatures struct {
		headers    int
		lists      int
		codeBlocks int
		links      int
		emphasis   int
		tables     int
		quotes     int
	}

	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// 检测代码块
		if strings.HasPrefix(trimmed, "```") {
			markdownFeatures.codeBlocks++
			inCodeBlock = !inCodeBlock
			continue
		}

		// 跳过代码块内的内容
		if inCodeBlock {
			continue
		}

		// 检测标题 (# ## ### ...)
		if strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, " ") {
			// 验证是否为有效的Markdown标题
			headerLevel := 0
			for _, char := range trimmed {
				if char == '#' {
					headerLevel++
				} else if char == ' ' {
					break
				} else {
					headerLevel = 0
					break
				}
			}
			if headerLevel > 0 && headerLevel <= 6 {
				markdownFeatures.headers++
			}
		}

		// 检测无序列表 (- * +)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") {
			markdownFeatures.lists++
		}

		// 检测有序列表 (1. 2. ...)
		if len(trimmed) > 2 {
			for i, char := range trimmed {
				if char >= '0' && char <= '9' {
					continue
				} else if char == '.' && i > 0 && i < len(trimmed)-1 && trimmed[i+1] == ' ' {
					markdownFeatures.lists++
					break
				} else {
					break
				}
			}
		}

		// 检测链接 [text](url) 或 ![alt](src)
		if strings.Contains(trimmed, "](") && (strings.Contains(trimmed, "[") || strings.Contains(trimmed, "![")) {
			markdownFeatures.links++
		}

		// 检测强调 **bold** *italic* __bold__ _italic_
		if strings.Contains(trimmed, "**") || strings.Contains(trimmed, "__") ||
			(strings.Count(trimmed, "*") >= 2 && !strings.HasPrefix(trimmed, "* ")) ||
			(strings.Count(trimmed, "_") >= 2 && !strings.HasPrefix(trimmed, "_ ")) {
			markdownFeatures.emphasis++
		}

		// 检测表格 (| column |)
		if strings.Contains(trimmed, "|") && strings.Count(trimmed, "|") >= 2 {
			markdownFeatures.tables++
		}

		// 检测引用 > quote
		if strings.HasPrefix(trimmed, "> ") {
			markdownFeatures.quotes++
		}
	}

	// 计算Markdown特征得分
	score := 0
	totalFeatures := markdownFeatures.headers + markdownFeatures.lists +
		markdownFeatures.codeBlocks + markdownFeatures.links +
		markdownFeatures.emphasis + markdownFeatures.tables +
		markdownFeatures.quotes

	// 权重评分
	score += markdownFeatures.headers * 3    // 标题权重最高
	score += markdownFeatures.lists * 2      // 列表权重较高
	score += markdownFeatures.codeBlocks * 2 // 代码块权重较高
	score += markdownFeatures.links * 1      // 链接权重中等
	score += markdownFeatures.emphasis * 1   // 强调权重中等
	score += markdownFeatures.tables * 2     // 表格权重较高
	score += markdownFeatures.quotes * 1     // 引用权重中等

	// 判断阈值
	// 1. 总特征数超过总行数的15%，或
	// 2. 得分超过总行数的20%，或
	// 3. 有3个以上的标题
	isMarkdown := (float64(totalFeatures)/float64(totalLines) > 0.15) ||
		(float64(score)/float64(totalLines) > 0.20) ||
		(markdownFeatures.headers >= 3)

	logger.GetLogger().Debug("Markdown detection analysis",
		zap.Int("total_lines", totalLines),
		zap.Int("headers", markdownFeatures.headers),
		zap.Int("lists", markdownFeatures.lists),
		zap.Int("code_blocks", markdownFeatures.codeBlocks),
		zap.Int("links", markdownFeatures.links),
		zap.Int("emphasis", markdownFeatures.emphasis),
		zap.Int("tables", markdownFeatures.tables),
		zap.Int("quotes", markdownFeatures.quotes),
		zap.Int("total_features", totalFeatures),
		zap.Int("score", score),
		zap.Bool("is_markdown", isMarkdown),
	)

	return isMarkdown
}
