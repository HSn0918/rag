package server

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	ragv1 "github.com/hsn0918/rag/internal/gen/rag/v1"
)

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
	uploadURL, err := s.Storage.GeneratePresignedUploadURL(ctx, objectKey, expires)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate upload URL: %w", err))
	}

	return connect.NewResponse(&ragv1.PreUploadResponse{
		UploadUrl: uploadURL,
		FileKey:   objectKey,
		ExpiresIn: int64(expires.Seconds()),
	}), nil
}
