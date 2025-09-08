package server

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	ragv1 "github.com/hsn0918/rag/internal/gen/proto/rag/v1"
)

// GetContext 接口的实现。
func (s *RagServer) GetContext(
	_ context.Context,
	req *connect.Request[ragv1.GetContextRequest],
) (*connect.Response[ragv1.GetContextResponse], error) {
	query := req.Msg.GetQuery()

	if query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("query is required"))
	}

	// TODO: 实现完整的向量搜索功能
	// 1. 生成查询向量
	// 2. 搜索相似文档块
	// 3. 重排序和过滤
	// 暂时返回占位符响应
	contextBuilder := strings.Builder{}
	contextBuilder.WriteString(fmt.Sprintf("Query: %s\n", query))
	contextBuilder.WriteString("Context retrieval functionality is being implemented.\n")
	contextBuilder.WriteString("This will include:\n")
	contextBuilder.WriteString("1. Vector similarity search\n")
	contextBuilder.WriteString("2. Semantic ranking\n")
	contextBuilder.WriteString("3. Structured context formatting\n")

	return connect.NewResponse(&ragv1.GetContextResponse{
		Context: contextBuilder.String(),
	}), nil
}
