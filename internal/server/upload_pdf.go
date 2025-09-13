package server

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"connectrpc.com/connect"
	"go.uber.org/zap"

	"github.com/hsn0918/rag/internal/chunking"
	ragv1 "github.com/hsn0918/rag/internal/gen/rag/v1"
	"github.com/hsn0918/rag/internal/logger"
)

var consecutiveNewlines = regexp.MustCompile(`\n{3,}`)

func (s *RagServer) UploadPdf(
	ctx context.Context,
	req *connect.Request[ragv1.UploadPdfRequest],
) (*connect.Response[ragv1.UploadPdfResponse], error) {
	fileKey := req.Msg.GetFileKey()
	filename := req.Msg.GetFilename()
	exists, err := s.Storage.CheckFileExists(ctx, fileKey)
	if err != nil {
		logger.Get().Error("failed to check file existence", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check file existence: %w", err))
	}
	if !exists {
		logger.Get().Error("file not found in storage", zap.String("file_key", fileKey))
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("file not found in storage: %s", fileKey))
	}
	object, err := s.Storage.DownloadFile(ctx, fileKey)
	if err != nil {
		logger.Get().Error("failed to download file", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to download file: %w", err))
	}
	defer object.Close()
	pdfData, err := io.ReadAll(object)
	if err != nil {
		logger.Get().Error("failed to read PDF data", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read PDF data: %w", err))
	}
	if len(pdfData) == 0 {
		logger.Get().Error("PDF file is empty")
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("PDF file is empty"))
	}

	textContent, pageCount, err := s.processPDFWithCaching(ctx, pdfData)
	if err != nil {
		logger.Get().Error("failed to process PDF with caching", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to process PDF with caching: %w", err))
	}
	if textContent == "" {
		logger.Get().Error("no text extracted from PDF")
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("no text extracted from PDF"))
	}

	textContent = s.cleanEmptyLines(textContent)
	md5Hash := fmt.Sprintf("%x", md5.Sum(pdfData))
	doc2xUID := fmt.Sprintf("processed_%s", md5Hash)

	chunks, err := s.chunkTextContent(textContent)
	if err != nil {
		logger.Get().Error("failed to chunk text", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to chunk text: %w", err))
	}

	// [REVERTED] Store the main document directly, without a transaction.
	docID, err := s.DB.StoreDocument(ctx, filename, req.Msg.GetFileKey(), map[string]any{
		"source":     filename,
		"pages":      pageCount,
		"doc2x_uid":  doc2xUID,
		"md5_hash":   md5Hash,
		"created_at": time.Now(),
	})
	if err != nil {
		logger.Get().Error("failed to store document", zap.Error(err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to store document: %w", err))
	}

	// Process chunks sequentially. Each database call is an independent operation.
	successfulChunks := 0
	for i, chunk := range chunks {
		cleanContent := s.cleanText(chunk.Content)

		embeddingVec, err := s.generateEmbedding(ctx, cleanContent)
		if err != nil {
			logger.Get().Error("Failed to generate embedding for chunk", zap.Int("chunk_id", i), zap.Error(err))
			continue
		}

		metadata := make(map[string]any)
		for k, v := range chunk.Metadata {
			metadata[k] = v
		}
		metadata["chunk_length"] = len(cleanContent)
		metadata["chunk_type"] = chunk.Type
		metadata["chunk_title"] = chunk.Title

		// [REVERTED] Store each chunk directly, without a transaction.
		err = s.DB.StoreChunk(ctx, docID, i, cleanContent, embeddingVec, metadata)
		if err != nil {
			logger.Get().Error("Failed to store chunk", zap.Int("chunk_id", i), zap.Error(err))
			continue
		}

		successfulChunks++
	}

	// Cache the document information.
	err = s.Cache.CacheDocument(ctx, docID, map[string]any{
		"title":     filename,
		"content":   textContent,
		"doc2x_uid": doc2xUID,
		"md5_hash":  md5Hash,
		"chunks":    len(chunks),
	})
	if err != nil {
		logger.Get().Warn("Failed to cache document", zap.String("doc_id", docID), zap.Error(err))
	}

	return connect.NewResponse(&ragv1.UploadPdfResponse{
		Success:    true,
		Message:    fmt.Sprintf("PDF processed successfully. Document ID: %s, Chunks: %d", docID, successfulChunks),
		DocumentId: docID,
	}), nil
}

// chunkTextContent applies semantic-aware chunking to text content
func (s *RagServer) chunkTextContent(content string) ([]chunking.Chunk, error) {
	chunkConfig := s.Config.Chunking

	// Check if semantic chunking is enabled
	useSemanticChunking := s.Config.Chunking.EnableSemantic

	if useSemanticChunking && s.EmbeddingClient != nil {
		logger.Get().Info("Using semantic chunking")

		semanticChunker, err := chunking.NewSemanticChunker(
			chunkConfig.MaxChunkSize,
			chunkConfig.MinChunkSize,
			s.EmbeddingClient,
			chunking.WithModel(s.Config.Services.Embedding.Model),
			chunking.WithSimilarityThreshold(s.Config.Chunking.SimilarityThreshold),
			chunking.WithParallelProcessing(true),
		)
		if err != nil {
			logger.Get().Error("Failed to create semantic chunker, falling back to standard chunking", zap.Error(err))
			// Fall back to standard chunking
		} else {
			ctx := context.Background()
			chunks, err := semanticChunker.ChunkText(ctx, content)
			if err != nil {
				logger.Get().Error("Semantic chunking failed, falling back to standard chunking", zap.Error(err))
				// Fall back to standard chunking
			} else {
				return chunks, nil
			}
		}
	}

	// Standard chunking fallback
	if s.detectMarkdownContent(content) {
		logger.Get().Debug("Using standard Markdown chunker")
		chunker, err := chunking.NewMarkdownChunker(
			chunkConfig.MaxChunkSize,
			chunkConfig.OverlapSize,
			true,
		)
		if err != nil {
			logger.Get().Error("Failed to create markdown chunker", zap.Error(err))
			return nil, err
		}
		return chunker.ChunkMarkdown(content)
	}

	logger.Get().Debug("Detected plain text content, using standard chunker")
	// For plain text, use the markdown chunker which handles plain text well
	chunker, err := chunking.NewMarkdownChunker(
		chunkConfig.MaxChunkSize,
		chunkConfig.OverlapSize,
		false,
	)
	if err != nil {
		return nil, err
	}
	return chunker.ChunkMarkdown(content)
}

func (s *RagServer) cleanEmptyLines(content string) string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	cleaned := consecutiveNewlines.ReplaceAllString(normalized, "\n\n")
	return strings.TrimSpace(cleaned)
}

func (s *RagServer) detectMarkdownContent(content string) bool {
	const (
		headerWeight    = 3
		listWeight      = 2
		codeBlockWeight = 2
		tableWeight     = 2
		linkWeight      = 1
		emphasisWeight  = 1
		quoteWeight     = 1

		featureRatioThreshold = 0.15
		scoreRatioThreshold   = 0.20
		minHeaderCount        = 3
	)

	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	if totalLines == 0 {
		return false
	}

	var markdownFeatures struct {
		headers, lists, codeBlocks, links, emphasis, tables, quotes int
	}
	inCodeBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "```") {
			markdownFeatures.codeBlocks++
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			markdownFeatures.headers++
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			markdownFeatures.lists++
		}
	}
	totalFeatures := markdownFeatures.headers + markdownFeatures.lists + markdownFeatures.codeBlocks
	score := markdownFeatures.headers*headerWeight + markdownFeatures.lists*listWeight + markdownFeatures.codeBlocks*codeBlockWeight
	isMarkdown := (float64(totalFeatures)/float64(totalLines) > featureRatioThreshold) ||
		(float64(score)/float64(totalLines) > scoreRatioThreshold) ||
		(markdownFeatures.headers >= minHeaderCount)

	logger.Get().Debug("Markdown detection analysis",
		zap.Int("total_lines", totalLines), zap.Int("total_features", totalFeatures),
		zap.Int("score", score), zap.Bool("is_markdown", isMarkdown))

	return isMarkdown
}
