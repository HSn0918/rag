package server

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/hsn0918/rag/internal/adapters"
	ragv1 "github.com/hsn0918/rag/internal/gen/rag/v1"
)

// DeleteDocument 删除文档及其分块
func (s *RagServer) DeleteDocument(
	ctx context.Context,
	req *connect.Request[ragv1.DeleteDocumentRequest],
) (*connect.Response[ragv1.DeleteDocumentResponse], error) {
	docID := strings.TrimSpace(req.Msg.GetDocumentId())
	if docID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("document_id is required"))
	}

	if err := s.DB.DeleteDocument(ctx, docID); err != nil {
		if errors.Is(err, adapters.ErrDocumentNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&ragv1.DeleteDocumentResponse{
		Success: true,
		Message: "document deleted",
	}), nil
}
