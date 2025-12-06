package server

import (
	"context"
	"encoding/json"
	"time"

	"connectrpc.com/connect"
	ragv1 "github.com/hsn0918/rag/internal/gen/rag/v1"
)

// ListDocuments 列出已上传的文档（按创建时间倒序，游标分页）
func (s *RagServer) ListDocuments(
	ctx context.Context,
	req *connect.Request[ragv1.ListDocumentsRequest],
) (*connect.Response[ragv1.ListDocumentsResponse], error) {
	pageSize := int(req.Msg.GetPageSize())
	if pageSize <= 0 {
		pageSize = 50
	}
	cursor := req.Msg.GetCursor()

	docs, nextCursor, err := s.DB.ListDocuments(ctx, pageSize, cursor)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	out := make([]*ragv1.Document, 0, len(docs))
	for _, d := range docs {
		var metadataJSON string
		if len(d.Metadata) > 0 {
			if b, err := json.Marshal(d.Metadata); err == nil {
				metadataJSON = string(b)
			}
		}

		createdAt := d.CreatedAt.UTC().Format(time.RFC3339)
		out = append(out, &ragv1.Document{
			Id:           d.ID,
			Title:        d.Title,
			MinioKey:     d.MinioKey,
			MetadataJson: metadataJSON,
			CreatedAt:    createdAt,
		})
	}

	resp := &ragv1.ListDocumentsResponse{
		Documents:  out,
		NextCursor: nextCursor,
	}
	return connect.NewResponse(resp), nil
}
