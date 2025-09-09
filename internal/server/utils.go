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
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/hsn0918/rag/internal/clients/doc2x"
	"github.com/hsn0918/rag/internal/logger"
	"go.uber.org/zap"
)

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

// cleanUTF8 清理字符串中的无效UTF-8字节序列
func (s *RagServer) cleanUTF8(text string) string {
	if utf8.ValidString(text) {
		return text
	}

	// 使用utf8.Valid逐字节检查并替换无效字符
	var result strings.Builder
	for _, r := range text {
		if r == utf8.RuneError {
			// 跳过无效字符或替换为空格
			result.WriteRune(' ')
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// generateEmbedding 使用嵌入客户端生成文本的向量表示
func (s *RagServer) generateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// 检查缓存
	cachedEmbedding, err := s.Cache.GetEmbedding(ctx, text)
	if err == nil && len(cachedEmbedding) > 0 {
		return cachedEmbedding, nil
	}

	// 调用嵌入服务
	embeddingResp, err := s.Embedding.CreateEmbeddingWithDefaults(s.Config.Services.Embedding.Model, text)
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
	_ = s.Cache.CacheEmbedding(ctx, text, embeddingVec)

	return embeddingVec, nil
}

// processPDFWithCaching handles PDF processing with caching logic
func (s *RagServer) processPDFWithCaching(ctx context.Context, pdfData []byte) (string, int, error) {
	// 计算PDF文件的MD5摘要
	md5Hash := fmt.Sprintf("%x", md5.Sum(pdfData))

	// 检查MinIO中是否有已处理的文本内容
	processedTextKey := fmt.Sprintf("processed/%s.txt", md5Hash)
	var textContent string
	var pageCount int

	// 首先检查MinIO中是否有处理后的文本
	processedExists, err := s.Storage.CheckFileExists(ctx, processedTextKey)
	if err == nil && processedExists {
		// MinIO中有缓存的处理结果，直接读取
		logger.GetLogger().Info("MinIO processed text cache hit", zap.String("md5", md5Hash))

		object, err := s.Storage.DownloadFile(ctx, processedTextKey)
		if err != nil {
			return "", 0, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to download cached processed text: %w", err))
		}
		defer object.Close()

		textBytes, err := io.ReadAll(object)
		if err != nil {
			return "", 0, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read cached processed text: %w", err))
		}

		textContent = string(textBytes)
		pageCount = 0 // 无法从缓存中获取页数，使用默认值
		logger.GetLogger().Info("Successfully loaded processed text from MinIO cache", zap.Int("length", len(textContent)))
	} else {
		// MinIO中没有缓存，需要处理PDF
		textContent, pageCount, err = s.processWithDoc2X(ctx, pdfData, md5Hash, processedTextKey)
		if err != nil {
			return "", 0, err
		}
	}

	return textContent, pageCount, nil
}

// processWithDoc2X handles Doc2X processing with Redis caching
func (s *RagServer) processWithDoc2X(ctx context.Context, pdfData []byte, md5Hash, processedTextKey string) (string, int, error) {
	// 检查Redis中的Doc2X响应缓存
	logger.GetLogger().Info("MinIO processed text cache miss, checking Redis cache", zap.String("md5", md5Hash))

	var statusResp *doc2x.StatusResponse
	cachedResp := &doc2x.StatusResponse{}
	err := s.Cache.GetDoc2XResponse(ctx, md5Hash, cachedResp)
	if err == nil && cachedResp.Data != nil {
		// Redis缓存命中，直接使用缓存的结果
		statusResp = cachedResp
		logger.GetLogger().Info("Redis Doc2X cache hit", zap.String("md5", md5Hash))
	} else {
		// 两级缓存都未命中，进行Doc2X处理
		logger.GetLogger().Info("Both caches miss, processing with Doc2X", zap.String("md5", md5Hash))

		// 使用 Doc2X 客户端上传并处理 PDF
		uploadResp, err := s.Doc2X.UploadPDF(pdfData)
		if err != nil {
			return "", 0, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to upload PDF to Doc2X: %w", err))
		}

		if uploadResp.Code != "success" {
			return "", 0, connect.NewError(connect.CodeInternal, fmt.Errorf("Doc2X upload failed: %s", uploadResp.Code))
		}

		// 等待处理完成
		statusResp, err = s.Doc2X.WaitForParsing(uploadResp.Data.UID, 5*time.Second)
		if err != nil {
			return "", 0, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to parse PDF: %w", err))
		}

		// 缓存Redis响应结果
		if statusResp.Data != nil && statusResp.Data.Status == "success" {
			err = s.Cache.CacheDoc2XResponse(ctx, md5Hash, statusResp)
			if err != nil {
				logger.GetLogger().Error("Failed to cache Doc2X response in Redis", zap.String("md5", md5Hash), zap.Error(err))
			} else {
				logger.GetLogger().Info("Doc2X response cached in Redis", zap.String("md5", md5Hash))
			}
		}
	}

	if statusResp.Data == nil || statusResp.Data.Result == nil {
		return "", 0, connect.NewError(connect.CodeInternal, fmt.Errorf("parsing result is empty"))
	}

	// 提取文本内容
	var allText strings.Builder
	for _, page := range statusResp.Data.Result.Pages {
		if page.Md != "" {
			allText.WriteString(page.Md)
			allText.WriteString("\n\n")
		}
	}

	textContent := allText.String()
	pageCount := len(statusResp.Data.Result.Pages)

	// 将处理后的文本内容缓存到MinIO
	if textContent != "" {
		textReader := bytes.NewReader([]byte(textContent))
		err = s.Storage.UploadFile(ctx, processedTextKey, textReader, int64(len(textContent)), "text/plain")
		if err != nil {
			logger.GetLogger().Error("Failed to cache processed text in MinIO", zap.String("md5", md5Hash), zap.Error(err))
		} else {
			logger.GetLogger().Info("Processed text cached in MinIO", zap.String("md5", md5Hash), zap.Int("length", len(textContent)))
		}
	}

	return textContent, pageCount, nil
}
