// Package storage provides object storage functionality for the RAG system.
// It follows Uber Go Style Guide conventions for interface design and error handling.
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

// ObjectStorage defines the interface for object storage operations.
type ObjectStorage interface {
	GeneratePresignedUploadURL(ctx context.Context, objectKey string, expires time.Duration) (string, error)
	GeneratePresignedDownloadURL(ctx context.Context, objectKey string, expires time.Duration) (string, error)
	UploadFile(ctx context.Context, objectKey string, reader io.Reader, objectSize int64, contentType string) error
	DownloadFile(ctx context.Context, objectKey string) (*minio.Object, error)
	DeleteFile(ctx context.Context, objectKey string) error
	GetFileInfo(ctx context.Context, objectKey string) (minio.ObjectInfo, error)
	CheckFileExists(ctx context.Context, objectKey string) (bool, error)
}

// MinIOClient provides MinIO-based object storage implementation.
// It implements the ObjectStorage interface with full feature support.
type MinIOClient struct {
	client     *minio.Client
	bucketName string
}

// Compile-time check to ensure MinIOClient implements ObjectStorage interface
var _ ObjectStorage = (*MinIOClient)(nil)

// MinIOConfig holds configuration parameters for MinIO client initialization.
type MinIOConfig struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
	UseSSL          bool
}

// NewMinIOClient creates a new MinIO client with the provided configuration.
// It automatically creates the bucket if it doesn't exist.
func NewMinIOClient(config MinIOConfig) (*MinIOClient, error) {
	// Initialize MinIO client
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKeyID, config.SecretAccessKey, ""),
		Secure: config.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	// Check if bucket exists, create if it doesn't
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

// GeneratePresignedUploadURL generates a presigned URL for file upload.
// The URL expires after the specified duration.
func (mc *MinIOClient) GeneratePresignedUploadURL(ctx context.Context, objectKey string, expires time.Duration) (string, error) {
	// Generate presigned PUT URL
	presignedURL, err := mc.client.PresignedPutObject(ctx, mc.bucketName, objectKey, expires)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignedURL.String(), nil
}

// GeneratePresignedDownloadURL generates a presigned URL for file download.
// The URL expires after the specified duration.
func (mc *MinIOClient) GeneratePresignedDownloadURL(ctx context.Context, objectKey string, expires time.Duration) (string, error) {
	// Generate presigned GET URL
	presignedURL, err := mc.client.PresignedGetObject(ctx, mc.bucketName, objectKey, expires, url.Values{})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned download URL: %w", err)
	}

	return presignedURL.String(), nil
}

// UploadFile directly uploads a file to the object storage.
// It requires the content type to be specified for proper handling.
func (mc *MinIOClient) UploadFile(ctx context.Context, objectKey string, reader io.Reader, objectSize int64, contentType string) error {
	_, err := mc.client.PutObject(ctx, mc.bucketName, objectKey, reader, objectSize, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}

	return nil
}

// DownloadFile downloads a file from the object storage.
// Returns a minio.Object that must be closed by the caller.
func (mc *MinIOClient) DownloadFile(ctx context.Context, objectKey string) (*minio.Object, error) {
	object, err := mc.client.GetObject(ctx, mc.bucketName, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}

	return object, nil
}

// DeleteFile removes a file from the object storage.
// It returns an error if the file doesn't exist or cannot be deleted.
func (mc *MinIOClient) DeleteFile(ctx context.Context, objectKey string) error {
	err := mc.client.RemoveObject(ctx, mc.bucketName, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

// GetFileInfo retrieves metadata information about a file.
// Returns ObjectInfo with details like size, modification time, etc.
func (mc *MinIOClient) GetFileInfo(ctx context.Context, objectKey string) (minio.ObjectInfo, error) {
	objInfo, err := mc.client.StatObject(ctx, mc.bucketName, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return minio.ObjectInfo{}, fmt.Errorf("failed to get file info: %w", err)
	}

	return objInfo, nil
}

// CheckFileExists checks if a file exists in the object storage.
// Returns true if the file exists, false if it doesn't, and an error for other failures.
func (mc *MinIOClient) CheckFileExists(ctx context.Context, objectKey string) (bool, error) {
	_, err := mc.client.StatObject(ctx, mc.bucketName, objectKey, minio.StatObjectOptions{})
	if err != nil {
		// Check if it's a "object not found" error
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}

	return true, nil
}