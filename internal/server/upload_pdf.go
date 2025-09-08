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
	defer object.Close()

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
			fmt.Printf("Failed to generate embedding for chunk %d: %v\n", i, err)
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
			fmt.Printf("Failed to store chunk %d: %v\n", i, err)
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
		fmt.Printf("Failed to cache document: %v\n", err)
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
			fmt.Printf("AST-based chunking failed: %v, falling back to simple text chunking\n", err)
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
