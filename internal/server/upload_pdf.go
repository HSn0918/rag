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
	ragv1 "github.com/hsn0918/rag/internal/gen/proto/rag/v1"
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

	// 向量化处理
	successfulChunks := 0
	for i, chunk := range chunks {
		// 清理无效的UTF-8字符
		cleanContent := s.cleanUTF8(chunk.Content)

		// 生成嵌入向量
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

// chunkTextContent uses intelligent markdown-aware chunking
func (s *RagServer) chunkTextContent(content string) ([]chunking.Chunk, error) {
	// 检测内容类型并选择合适的分块策略
	if strings.Contains(content, "#") && (strings.Contains(content, "##") || strings.Contains(content, "- ") || strings.Contains(content, "* ")) {
		// 看起来像markdown内容，首先尝试AST-based chunking
		chunker := chunking.NewMarkdownChunker(512, 50, true)
		chunks, err := chunker.ChunkMarkdown(content)
		if err != nil {
			// AST分块失败时，回退到简单文本分块
			logger.GetLogger().Warn("AST-based chunking failed, falling back to simple text chunking", zap.Error(err))
			return s.simpleTextChunking(content), nil
		}
		return chunks, nil
	}

	// 回退到简单文本分块
	return s.simpleTextChunking(content), nil
}

// simpleTextChunking provides fallback text chunking
func (s *RagServer) simpleTextChunking(content string) []chunking.Chunk {
	const chunkSize = 512
	const overlap = 50

	if len(content) <= chunkSize {
		return []chunking.Chunk{{
			Content: content,
			Type:    "text",
			Level:   0,
			Title:   "Document",
		}}
	}

	var chunks []chunking.Chunk
	start := 0
	chunkIndex := 0

	for start < len(content) {
		end := start + chunkSize
		if end > len(content) {
			end = len(content)
		}

		chunk := chunking.Chunk{
			Content:    content[start:end],
			Type:       "text",
			Level:      0,
			Title:      fmt.Sprintf("Part %d", chunkIndex+1),
			StartIndex: start,
			EndIndex:   end,
			Metadata: map[string]string{
				"chunk_index": fmt.Sprintf("%d", chunkIndex),
			},
		}
		chunks = append(chunks, chunk)

		if end == len(content) {
			break
		}

		// 下一块的起始位置（考虑重叠）
		start = end - overlap
		if start <= 0 {
			start = end
		}
		chunkIndex++
	}

	return chunks
}

// cleanEmptyLines removes excessive empty lines from text content
func (s *RagServer) cleanEmptyLines(content string) string {
	// 分割文本为行
	lines := strings.Split(content, "\n")
	var cleanedLines []string

	for _, line := range lines {
		// 检查是否为空行（只包含空白字符）
		trimmedLine := strings.TrimSpace(line)

		// 只保留非空行
		if trimmedLine != "" {
			cleanedLines = append(cleanedLines, trimmedLine)
		}
	}

	// 重新连接行
	cleanedContent := strings.Join(cleanedLines, "\n")

	// 去除开头和结尾的空白
	cleanedContent = strings.TrimSpace(cleanedContent)

	logger.GetLogger().Debug("Cleaned empty lines",
		zap.Int("original_lines", len(lines)),
		zap.Int("cleaned_lines", len(cleanedLines)),
		zap.Int("original_length", len(content)),
		zap.Int("cleaned_length", len(cleanedContent)),
	)

	return cleanedContent
}
