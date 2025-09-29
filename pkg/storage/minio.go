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

type ObjectStorage interface {
	GeneratePresignedUploadURL(ctx context.Context, objectKey string, expires time.Duration) (string, error)
	GeneratePresignedDownloadURL(ctx context.Context, objectKey string, expires time.Duration) (string, error)
	UploadFile(ctx context.Context, objectKey string, reader io.Reader, objectSize int64, contentType string) error
	DownloadFile(ctx context.Context, objectKey string) (*minio.Object, error)
	DeleteFile(ctx context.Context, objectKey string) error
	GetFileInfo(ctx context.Context, objectKey string) (minio.ObjectInfo, error)
	CheckFileExists(ctx context.Context, objectKey string) (bool, error)
}

type MinIOClient struct {
	client     *minio.Client
	bucketName string
}

var _ ObjectStorage = (*MinIOClient)(nil)

type MinIOConfig struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
	UseSSL          bool
}

func NewMinIOClient(config MinIOConfig) (*MinIOClient, error) {
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKeyID, config.SecretAccessKey, ""),
		Secure: config.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, config.BucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket existence: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, config.BucketName, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	return &MinIOClient{client: client, bucketName: config.BucketName}, nil
}

func (mc *MinIOClient) GeneratePresignedUploadURL(ctx context.Context, objectKey string, expires time.Duration) (string, error) {
	presignedURL, err := mc.client.PresignedPutObject(ctx, mc.bucketName, objectKey, expires)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return presignedURL.String(), nil
}

func (mc *MinIOClient) GeneratePresignedDownloadURL(ctx context.Context, objectKey string, expires time.Duration) (string, error) {
	presignedURL, err := mc.client.PresignedGetObject(ctx, mc.bucketName, objectKey, expires, url.Values{})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned download URL: %w", err)
	}
	return presignedURL.String(), nil
}

func (mc *MinIOClient) UploadFile(ctx context.Context, objectKey string, reader io.Reader, objectSize int64, contentType string) error {
	_, err := mc.client.PutObject(ctx, mc.bucketName, objectKey, reader, objectSize, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}
	return nil
}

func (mc *MinIOClient) DownloadFile(ctx context.Context, objectKey string) (*minio.Object, error) {
	object, err := mc.client.GetObject(ctx, mc.bucketName, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	return object, nil
}

func (mc *MinIOClient) DeleteFile(ctx context.Context, objectKey string) error {
	if err := mc.client.RemoveObject(ctx, mc.bucketName, objectKey, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}

func (mc *MinIOClient) GetFileInfo(ctx context.Context, objectKey string) (minio.ObjectInfo, error) {
	objInfo, err := mc.client.StatObject(ctx, mc.bucketName, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return minio.ObjectInfo{}, fmt.Errorf("failed to get file info: %w", err)
	}
	return objInfo, nil
}

func (mc *MinIOClient) CheckFileExists(ctx context.Context, objectKey string) (bool, error) {
	_, err := mc.client.StatObject(ctx, mc.bucketName, objectKey, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}
	return true, nil
}
