package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOClient struct {
	client     *minio.Client
	bucketName string
}

type MinIOConfig struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
	UseSSL          bool
}

func NewMinIOClient(config MinIOConfig) (*MinIOClient, error) {
	// 初始化 MinIO 客户端
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKeyID, config.SecretAccessKey, ""),
		Secure: config.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	// 检查 bucket 是否存在，不存在则创建
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, config.BucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if !exists {
		err = client.MakeBucket(ctx, config.BucketName, minio.MakeBucketOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	return &MinIOClient{
		client:     client,
		bucketName: config.BucketName,
	}, nil
}

// GeneratePresignedUploadURL 生成预签名上传 URL
func (mc *MinIOClient) GeneratePresignedUploadURL(ctx context.Context, objectKey string, expires time.Duration) (string, error) {

	// 生成预签名 PUT URL
	presignedURL, err := mc.client.PresignedPutObject(ctx, mc.bucketName, objectKey, expires)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignedURL.String(), nil
}

// GeneratePresignedDownloadURL 生成预签名下载 URL
func (mc *MinIOClient) GeneratePresignedDownloadURL(objectKey string, expires time.Duration) (string, error) {
	ctx := context.Background()

	// 生成预签名 GET URL
	presignedURL, err := mc.client.PresignedGetObject(ctx, mc.bucketName, objectKey, expires, url.Values{})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned download URL: %w", err)
	}

	return presignedURL.String(), nil
}

// UploadFile 直接上传文件
func (mc *MinIOClient) UploadFile(ctx context.Context, objectKey string, reader io.Reader, objectSize int64, contentType string) error {
	_, err := mc.client.PutObject(ctx, mc.bucketName, objectKey, reader, objectSize, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}

	return nil
}

// DownloadFile 下载文件
func (mc *MinIOClient) DownloadFile(ctx context.Context, objectKey string) (*minio.Object, error) {
	object, err := mc.client.GetObject(ctx, mc.bucketName, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}

	return object, nil
}

// DeleteFile 删除文件
func (mc *MinIOClient) DeleteFile(ctx context.Context, objectKey string) error {
	err := mc.client.RemoveObject(ctx, mc.bucketName, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

// GetFileInfo 获取文件信息
func (mc *MinIOClient) GetFileInfo(ctx context.Context, objectKey string) (minio.ObjectInfo, error) {
	objInfo, err := mc.client.StatObject(ctx, mc.bucketName, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return minio.ObjectInfo{}, fmt.Errorf("failed to get file info: %w", err)
	}

	return objInfo, nil
}

// CheckFileExists 检查文件是否存在
func (mc *MinIOClient) CheckFileExists(ctx context.Context, objectKey string) (bool, error) {
	_, err := mc.client.StatObject(ctx, mc.bucketName, objectKey, minio.StatObjectOptions{})
	if err != nil {
		// 检查是否是 "对象不存在" 错误
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}

	return true, nil
}
