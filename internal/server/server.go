package server

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/hsn0918/rag/internal/adapters"
	"github.com/hsn0918/rag/internal/clients/doc2x"
	"github.com/hsn0918/rag/internal/clients/embedding"
	"github.com/hsn0918/rag/internal/clients/openai"
	"github.com/hsn0918/rag/internal/clients/rerank"
	"github.com/hsn0918/rag/internal/config"
	ragv1 "github.com/hsn0918/rag/internal/gen/proto/rag/v1"
	"github.com/hsn0918/rag/internal/redis"
	"github.com/hsn0918/rag/internal/storage"
)

// RagServer 实现了 ragv1connect.RagServiceHandler 接口。
// 它包含了服务所需的所有依赖项。
type RagServer struct {
	db        adapters.VectorDB
	cache     *redis.CacheService
	storage   *storage.MinIOClient
	doc2x     *doc2x.Client
	embedding *embedding.Client
	llm       *openai.Client
	reranker  *rerank.Client
	cfg       config.Config
}

// NewRagServer 是 RagServer 的构造函数。
func NewRagServer(
	db adapters.VectorDB,
	cache *redis.CacheService,
	storageClient *storage.MinIOClient,
	doc2xClient *doc2x.Client,
	embeddingClient *embedding.Client,
	llmClient *openai.Client,
	rerankClient *rerank.Client,
	cfg config.Config,
) *RagServer {
	return &RagServer{
		db:        db,
		cache:     cache,
		storage:   storageClient,
		doc2x:     doc2xClient,
		embedding: embeddingClient,
		llm:       llmClient,
		reranker:  rerankClient,
		cfg:       cfg,
	}
}

// PreUpload 接口的实现，生成预签名上传 URL
func (s *RagServer) PreUpload(
	ctx context.Context,
	req *connect.Request[ragv1.PreUploadRequest],
) (*connect.Response[ragv1.PreUploadResponse], error) {
	filename := req.Msg.GetFilename()
	if filename == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("filename is required"))
	}

	// 生成唯一的对象键
	objectKey, err := s.generateObjectKey(filename)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate object key: %w", err))
	}

	// 生成预签名上传 URL，有效期 15 分钟
	expires := 15 * time.Minute
	uploadURL, err := s.storage.GeneratePresignedUploadURL(ctx, objectKey, expires)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate upload URL: %w", err))
	}

	return connect.NewResponse(&ragv1.PreUploadResponse{
		UploadUrl: uploadURL,
		FileKey:   objectKey,
		ExpiresIn: int64(expires.Seconds()),
	}), nil
}

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
	exists, err := s.storage.CheckFileExists(ctx, fileKey)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check file existence: %w", err))
	}

	if !exists {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("file not found in storage: %s", fileKey))
	}

	// 从 MinIO 下载文件
	object, err := s.storage.DownloadFile(ctx, fileKey)
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

	// 1. 计算PDF文件的MD5摘要
	md5Hash := fmt.Sprintf("%x", md5.Sum(pdfData))

	// 2. 检查MinIO中是否有已处理的文本内容
	processedTextKey := fmt.Sprintf("processed/%s.txt", md5Hash)
	var textContent string
	var statusResp *doc2x.StatusResponse
	var pageCount int

	// 首先检查MinIO中是否有处理后的文本
	processedExists, err := s.storage.CheckFileExists(ctx, processedTextKey)
	if err == nil && processedExists {
		// MinIO中有缓存的处理结果，直接读取
		fmt.Printf("MinIO processed text cache hit for MD5: %s\n", md5Hash)

		object, err := s.storage.DownloadFile(ctx, processedTextKey)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to download cached processed text: %w", err))
		}
		defer object.Close()

		textBytes, err := io.ReadAll(object)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read cached processed text: %w", err))
		}

		textContent = string(textBytes)
		pageCount = 0 // 无法从缓存中获取页数，使用默认值
		fmt.Printf("Successfully loaded processed text from MinIO cache (length: %d)\n", len(textContent))
	} else {
		// MinIO中没有缓存，检查Redis中的Doc2X响应缓存
		fmt.Printf("MinIO processed text cache miss for MD5: %s, checking Redis cache...\n", md5Hash)

		cachedResp := &doc2x.StatusResponse{}
		err = s.cache.GetDoc2XResponse(ctx, md5Hash, cachedResp)
		if err == nil && cachedResp.Data != nil {
			// Redis缓存命中，直接使用缓存的结果
			statusResp = cachedResp
			fmt.Printf("Redis Doc2X cache hit for MD5: %s\n", md5Hash)
		} else {
			// 两级缓存都未命中，进行Doc2X处理
			fmt.Printf("Both caches miss for MD5: %s, processing with Doc2X...\n", md5Hash)

			// 使用 Doc2X 客户端上传并处理 PDF
			uploadResp, err := s.doc2x.UploadPDF(pdfData)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to upload PDF to Doc2X: %w", err))
			}

			if uploadResp.Code != "success" {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("Doc2X upload failed: %s", uploadResp.Code))
			}

			// 等待处理完成
			statusResp, err = s.doc2x.WaitForParsing(uploadResp.Data.UID, 5*time.Second)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse PDF: %w", err))
			}

			// 缓存Redis响应结果
			if statusResp.Data != nil && statusResp.Data.Status == "success" {
				err = s.cache.CacheDoc2XResponse(ctx, md5Hash, statusResp)
				if err != nil {
					fmt.Printf("Failed to cache Doc2X response in Redis: %v\n", err)
				} else {
					fmt.Printf("Doc2X response cached in Redis for MD5: %s\n", md5Hash)
				}
			}
		}

		if statusResp.Data == nil || statusResp.Data.Result == nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("parsing result is empty"))
		}

		// 提取文本内容
		var allText strings.Builder
		for _, page := range statusResp.Data.Result.Pages {
			if page.Md != "" {
				allText.WriteString(page.Md)
				allText.WriteString("\n\n")
			}
		}

		textContent = allText.String()
		pageCount = len(statusResp.Data.Result.Pages)

		// 将处理后的文本内容缓存到MinIO
		if textContent != "" {
			textReader := bytes.NewReader([]byte(textContent))
			err = s.storage.UploadFile(ctx, processedTextKey, textReader, int64(len(textContent)), "text/plain")
			if err != nil {
				fmt.Printf("Failed to cache processed text in MinIO: %v\n", err)
			} else {
				fmt.Printf("Processed text cached in MinIO for MD5: %s (length: %d)\n", md5Hash, len(textContent))
			}
		}
	}

	// 3. 验证提取的文本内容
	if textContent == "" {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("no text extracted from PDF"))
	}

	// 获取处理UID，用于后续的元数据存储
	doc2xUID := fmt.Sprintf("processed_%s", md5Hash)

	// 4. 将文档存储到数据库
	docID, err := s.db.StoreDocument(ctx, "uploaded_pdf", textContent, map[string]interface{}{
		"source":     "pdf_upload",
		"pages":      pageCount,
		"doc2x_uid":  doc2xUID,
		"md5_hash":   md5Hash,
		"created_at": time.Now(),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to store document: %w", err))
	}

	// 5. 文本分块和向量化处理
	chunks := s.chunkText(textContent, 512, 50) // 512 字符块，50 字符重叠

	for i, chunk := range chunks {
		// 生成嵌入向量
		embeddingVec, err := s.generateEmbedding(ctx, chunk)
		if err != nil {
			// 记录错误但继续处理其他块
			fmt.Printf("Failed to generate embedding for chunk %d: %v\n", i, err)
			continue
		}

		// 存储文本块和向量
		err = s.db.StoreChunk(ctx, docID, i, chunk, embeddingVec, map[string]interface{}{
			"chunk_length": len(chunk),
		})
		if err != nil {
			fmt.Printf("Failed to store chunk %d: %v\n", i, err)
			continue
		}
	}

	// 6. 缓存文档信息
	err = s.cache.CacheDocument(ctx, docID, map[string]interface{}{
		"title":     "uploaded_pdf",
		"content":   textContent,
		"doc2x_uid": doc2xUID,
		"md5_hash":  md5Hash,
		"chunks":    len(chunks),
	})
	if err != nil {
		// 缓存失败不影响主要流程
		fmt.Printf("Failed to cache document: %v\n", err)
	}

	return connect.NewResponse(&ragv1.UploadPdfResponse{
		Success:    true,
		Message:    fmt.Sprintf("PDF processed successfully. Document ID: %s, Chunks: %d", docID, len(chunks)),
		DocumentId: docID,
	}), nil
}

// chunkText 将文本分割成指定大小的块
func (s *RagServer) chunkText(text string, chunkSize, overlap int) []string {
	if len(text) <= chunkSize {
		return []string{text}
	}

	var chunks []string
	start := 0

	for start < len(text) {
		end := start + chunkSize
		if end > len(text) {
			end = len(text)
		}

		chunk := text[start:end]
		chunks = append(chunks, chunk)

		// 如果这是最后一块，退出
		if end == len(text) {
			break
		}

		// 下一块的起始位置（考虑重叠）
		start = end - overlap
		if start <= 0 {
			start = end
		}
	}

	return chunks
}

// generateEmbedding 使用嵌入客户端生成文本的向量表示
func (s *RagServer) generateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// 检查缓存
	cachedEmbedding, err := s.cache.GetEmbedding(ctx, text)
	if err == nil && len(cachedEmbedding) > 0 {
		return cachedEmbedding, nil
	}

	// 调用嵌入服务
	embeddingResp, err := s.embedding.CreateEmbeddingWithDefaults(s.cfg.Services.Embedding.Model, text)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding: %w", err)
	}

	if len(embeddingResp.Data) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}

	// 转换为 float32
	embeddingVec := make([]float32, len(embeddingResp.Data[0].Embedding))
	for i, val := range embeddingResp.Data[0].Embedding {
		embeddingVec[i] = float32(val)
	}

	// 缓存结果
	_ = s.cache.CacheEmbedding(ctx, text, embeddingVec)

	return embeddingVec, nil
}

// generateObjectKey 生成唯一的对象键
func (s *RagServer) generateObjectKey(filename string) (string, error) {
	// 生成随机字符串作为前缀
	randomBytes := make([]byte, 16)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	randomStr := hex.EncodeToString(randomBytes)
	timestamp := time.Now().Unix()

	// 格式: {timestamp}_{random}_{filename}
	objectKey := fmt.Sprintf("%d_%s_%s", timestamp, randomStr, filename)

	return objectKey, nil
}

// GetContext 接口的实现。
func (s *RagServer) GetContext(
	ctx context.Context,
	req *connect.Request[ragv1.GetContextRequest],
) (*connect.Response[ragv1.GetContextResponse], error) {
	panic("implement me")
}
