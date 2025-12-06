package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/hsn0918/rag/internal/adapters"
	ragv1 "github.com/hsn0918/rag/internal/gen/rag/v1"
	"github.com/hsn0918/rag/pkg/search"
)

// ExtractKeywords RPC: 单独返回关键词
func (s *RagServer) ExtractKeywords(ctx context.Context, req *connect.Request[ragv1.ExtractKeywordsRequest]) (*connect.Response[ragv1.ExtractKeywordsResponse], error) {
	query := req.Msg.GetQuery()
	if query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("query is required"))
	}
	keywords, err := s.generateKeywords(ctx, query)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate keywords: %w", err))
	}
	return connect.NewResponse(&ragv1.ExtractKeywordsResponse{Keywords: keywords}), nil
}

// GenerateQueryEmbedding RPC: 生成查询向量
func (s *RagServer) GenerateQueryEmbedding(ctx context.Context, req *connect.Request[ragv1.GenerateQueryEmbeddingRequest]) (*connect.Response[ragv1.GenerateQueryEmbeddingResponse], error) {
	q := req.Msg.GetQueryText()
	if q == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("query_text is required"))
	}
	vec, err := s.generateEmbedding(ctx, q)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate query embedding: %w", err))
	}
	return connect.NewResponse(&ragv1.GenerateQueryEmbeddingResponse{Vector: vec}), nil
}

// SearchChunks RPC: 基于向量搜索
func (s *RagServer) SearchChunks(ctx context.Context, req *connect.Request[ragv1.SearchChunksRequest]) (*connect.Response[ragv1.SearchChunksResponse], error) {
	vec := req.Msg.GetVector()
	if len(vec) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("vector is required"))
	}
	limit := int(req.Msg.GetLimit())
	if limit <= 0 {
		limit = 15
	}
	threshold := req.Msg.GetThreshold()
	if threshold <= 0 {
		threshold = 0.3
	}

	results, err := s.DB.SearchSimilarChunks(ctx, vec, limit, threshold)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to search similar chunks: %w", err))
	}

	resp := &ragv1.SearchChunksResponse{
		Chunks: mapSearchChunks(results, threshold),
	}
	return connect.NewResponse(resp), nil
}

// RerankChunks RPC: 对传入的 chunk 进行重排序
func (s *RagServer) RerankChunks(ctx context.Context, req *connect.Request[ragv1.RerankChunksRequest]) (*connect.Response[ragv1.RerankChunksResponse], error) {
	chunks, err := fromRerankInputs(req.Msg.GetChunks())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid chunks: %w", err))
	}
	query := req.Msg.GetQuery()
	keywords := req.Msg.GetKeywords()
	maxChunks := int(req.Msg.GetMaxChunks())
	if maxChunks <= 0 {
		maxChunks = 5
	}
	minSim := req.Msg.GetMinSimilarity()
	if minSim <= 0 {
		minSim = 0.25
	}

	reranked := search.RerankChunksWithKeywords(chunks, query, keywords, maxChunks, minSim)
	resp := &ragv1.RerankChunksResponse{
		Chunks: mapRerankChunks(reranked, true),
	}
	return connect.NewResponse(resp), nil
}

// SummarizeContext RPC: 基于 chunks 生成总结
func (s *RagServer) SummarizeContext(ctx context.Context, req *connect.Request[ragv1.SummarizeContextRequest]) (*connect.Response[ragv1.SummarizeContextResponse], error) {
	query := req.Msg.GetQuery()
	chunks, err := fromRerankInputs(req.Msg.GetChunks())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid chunks: %w", err))
	}
	summary, err := s.generateContextSummary(ctx, chunks, query)
	if err != nil || summary == "" {
		summary = s.generateBasicContextSummary(chunks, query)
	}
	return connect.NewResponse(&ragv1.SummarizeContextResponse{
		Summary: summary,
	}), nil
}

func mapSearchChunks(chunks []adapters.ChunkSearchResult, threshold float32) []*ragv1.SearchChunk {
	out := make([]*ragv1.SearchChunk, 0, len(chunks))
	for _, c := range chunks {
		if c.Similarity < threshold {
			continue
		}
		item := &ragv1.SearchChunk{
			Id:         c.ChunkID,
			DocumentId: c.DocumentID,
			Similarity: c.Similarity,
			Snippet:    truncateSnippet(c.Content, 240),
		}
		if len(c.Metadata) > 0 {
			if b, err := json.Marshal(c.Metadata); err == nil {
				item.MetadataJson = string(b)
			}
		}
		out = append(out, item)
	}
	return out
}

func mapRerankChunks(chunks []adapters.ChunkSearchResult, includeScore bool) []*ragv1.RerankChunk {
	out := make([]*ragv1.RerankChunk, 0, len(chunks))
	for _, c := range chunks {
		item := &ragv1.RerankChunk{
			Id:         c.ChunkID,
			Similarity: c.Similarity,
			Snippet:    truncateSnippet(c.Content, 240),
		}
		if includeScore && c.Metadata != nil {
			switch v := c.Metadata["advanced_score"].(type) {
			case float64:
				item.Score = v
			case float32:
				item.Score = float64(v)
			}
		}
		if len(c.Metadata) > 0 {
			if b, err := json.Marshal(c.Metadata); err == nil {
				item.MetadataJson = string(b)
			}
		}
		out = append(out, item)
	}
	return out
}

func fromRerankInputs(inputs []*ragv1.RerankChunkInput) ([]adapters.ChunkSearchResult, error) {
	out := make([]adapters.ChunkSearchResult, 0, len(inputs))
	for _, in := range inputs {
		item := adapters.ChunkSearchResult{
			ChunkID:    in.GetId(),
			Similarity: float32(in.GetSimilarity()),
			Content:    in.GetContent(),
		}
		if mj := in.GetMetadataJson(); mj != "" {
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(mj), &m); err != nil {
				return nil, err
			}
			item.Metadata = m
		}
		out = append(out, item)
	}
	return out, nil
}

func truncateSnippet(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
