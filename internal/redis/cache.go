package redis

import (
	"context"
	"fmt"
	"time"
)

type CacheService struct {
	client *Client
}

func NewCacheService(client *Client) *CacheService {
	return &CacheService{
		client: client,
	}
}

const (
	DefaultTTL           = 1 * time.Hour
	EmbeddingCacheTTL    = 24 * time.Hour
	DocumentCacheTTL     = 6 * time.Hour
	SearchResultCacheTTL = 30 * time.Minute
	Doc2XCacheTTL        = 7 * 24 * time.Hour // 7 days for Doc2X results
)

func (s *CacheService) CacheEmbedding(ctx context.Context, text string, embedding []float32) error {
	key := fmt.Sprintf("embedding:%s", hashText(text))
	return s.client.SetJSON(ctx, key, embedding, EmbeddingCacheTTL)
}

func (s *CacheService) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	key := fmt.Sprintf("embedding:%s", hashText(text))
	var embedding []float32
	err := s.client.GetJSON(ctx, key, &embedding)
	if err != nil {
		return nil, err
	}
	return embedding, nil
}

func (s *CacheService) CacheSearchResults(ctx context.Context, query string, results interface{}) error {
	key := fmt.Sprintf("search:%s", hashText(query))
	return s.client.SetJSON(ctx, key, results, SearchResultCacheTTL)
}

func (s *CacheService) GetSearchResults(ctx context.Context, query string, dest interface{}) error {
	key := fmt.Sprintf("search:%s", hashText(query))
	return s.client.GetJSON(ctx, key, dest)
}

func (s *CacheService) CacheDocument(ctx context.Context, docID string, document interface{}) error {
	key := fmt.Sprintf("doc:%s", docID)
	return s.client.SetJSON(ctx, key, document, DocumentCacheTTL)
}

func (s *CacheService) GetDocument(ctx context.Context, docID string, dest interface{}) error {
	key := fmt.Sprintf("doc:%s", docID)
	return s.client.GetJSON(ctx, key, dest)
}

func (s *CacheService) InvalidateDocument(ctx context.Context, docID string) error {
	key := fmt.Sprintf("doc:%s", docID)
	return s.client.Delete(ctx, key)
}

func (s *CacheService) SetCounter(ctx context.Context, name string, value int64, ttl time.Duration) error {
	key := fmt.Sprintf("counter:%s", name)
	return s.client.Set(ctx, key, fmt.Sprintf("%d", value), ttl)
}

func (s *CacheService) IncrementCounter(ctx context.Context, name string, ttl time.Duration) (int64, error) {
	key := fmt.Sprintf("counter:%s", name)

	cmd := s.client.client.B().Incr().Key(key).Build()
	result := s.client.client.Do(ctx, cmd)
	if result.Error() != nil {
		return 0, result.Error()
	}

	count, err := result.ToInt64()
	if err != nil {
		return 0, err
	}

	if count == 1 && ttl > 0 {
		expireCmd := s.client.client.B().Expire().Key(key).Seconds(int64(ttl.Seconds())).Build()
		if err := s.client.client.Do(ctx, expireCmd).Error(); err != nil {
			return count, fmt.Errorf("failed to set TTL: %w", err)
		}
	}

	return count, nil
}

func (s *CacheService) GetCounter(ctx context.Context, name string) (int64, error) {
	key := fmt.Sprintf("counter:%s", name)
	value, err := s.client.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	if value == "" {
		return 0, nil
	}

	var count int64
	if _, err := fmt.Sscanf(value, "%d", &count); err != nil {
		return 0, fmt.Errorf("invalid counter value: %w", err)
	}
	return count, nil
}

func (s *CacheService) SetSession(ctx context.Context, sessionID string, data interface{}, ttl time.Duration) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return s.client.SetJSON(ctx, key, data, ttl)
}

func (s *CacheService) GetSession(ctx context.Context, sessionID string, dest interface{}) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return s.client.GetJSON(ctx, key, dest)
}

func (s *CacheService) DeleteSession(ctx context.Context, sessionID string) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return s.client.Delete(ctx, key)
}

func (s *CacheService) SetUserData(ctx context.Context, userID string, field string, value interface{}, ttl time.Duration) error {
	key := fmt.Sprintf("user:%s", userID)
	data, err := marshalJSON(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	fields := map[string]string{
		field: string(data),
	}
	return s.client.SetHash(ctx, key, fields, ttl)
}

func (s *CacheService) GetUserData(ctx context.Context, userID string, field string, dest interface{}) error {
	key := fmt.Sprintf("user:%s", userID)
	value, err := s.client.GetHashField(ctx, key, field)
	if err != nil {
		return err
	}
	if value == "" {
		return nil
	}

	return unmarshalJSON([]byte(value), dest)
}

func (s *CacheService) InvalidateUserData(ctx context.Context, userID string, fields ...string) error {
	key := fmt.Sprintf("user:%s", userID)
	if len(fields) == 0 {
		return s.client.Delete(ctx, key)
	}
	return s.client.DeleteHashFields(ctx, key, fields...)
}

func (s *CacheService) ClearCache(ctx context.Context, pattern string) error {
	return s.client.FlushDB(ctx)
}

func (s *CacheService) CacheDoc2XResponse(ctx context.Context, md5Hash string, response interface{}) error {
	key := fmt.Sprintf("doc2x:%s", md5Hash)
	return s.client.SetJSON(ctx, key, response, Doc2XCacheTTL)
}

func (s *CacheService) GetDoc2XResponse(ctx context.Context, md5Hash string, dest interface{}) error {
	key := fmt.Sprintf("doc2x:%s", md5Hash)
	return s.client.GetJSON(ctx, key, dest)
}
