package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/rueidis"

	"github.com/hsn0918/rag/pkg/config"
)

// RedisClient defines the interface for Redis operations.
// This interface enables easier testing and potential implementation swapping.
type RedisClient interface {
	// Basic operations
	Set(ctx context.Context, key string, value string, expiration time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)

	// JSON operations
	SetJSON(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	GetJSON(ctx context.Context, key string, dest interface{}) error

	// Hash operations
	SetHash(ctx context.Context, key string, fields map[string]string, expiration time.Duration) error
	GetHash(ctx context.Context, key string) (map[string]string, error)
	GetHashField(ctx context.Context, key, field string) (string, error)
	DeleteHashFields(ctx context.Context, key string, fields ...string) error

	// Utility operations
	Ping(ctx context.Context) error
	FlushDB(ctx context.Context) error
	Close()
}

// Client implements RedisClient using rueidis.
type Client struct {
	client rueidis.Client
}

// ClientOptions holds configuration for Redis client creation.
type ClientOptions struct {
	Host     string `validate:"required"`
	Port     int    `validate:"min=1,max=65535"`
	Password string // optional
	DB       int    `validate:"min=0,max=15"`
}

func NewClient(opts ClientOptions) (*Client, error) {
	client, err := rueidis.NewClient(rueidis.ClientOption{
		InitAddress: []string{fmt.Sprintf("%s:%d", opts.Host, opts.Port)},
		Password:    opts.Password,
		SelectDB:    opts.DB,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Redis client: %w", err)
	}

	return &Client{
		client: client,
	}, nil
}

func NewClientFromConfig(cfg config.Config) (*Client, error) {
	return NewClient(ClientOptions{
		Host:     cfg.Redis.Host,
		Port:     cfg.Redis.Port,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
}

func (c *Client) Close() { c.client.Close() }

func (c *Client) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	var cmd rueidis.Completed
	if expiration > 0 {
		cmd = c.client.B().Set().Key(key).Value(value).ExSeconds(int64(expiration.Seconds())).Build()
	} else {
		cmd = c.client.B().Set().Key(key).Value(value).Build()
	}
	return c.client.Do(ctx, cmd).Error()
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	cmd := c.client.B().Get().Key(key).Build()
	result := c.client.Do(ctx, cmd)
	if result.Error() != nil {
		if rueidis.IsRedisNil(result.Error()) {
			return "", nil
		}
		return "", result.Error()
	}
	return result.ToString()
}

func (c *Client) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	cmd := c.client.B().Del().Key(keys...).Build()
	return c.client.Do(ctx, cmd).Error()
}

func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	cmd := c.client.B().Exists().Key(key).Build()
	result := c.client.Do(ctx, cmd)
	if result.Error() != nil {
		return false, result.Error()
	}
	count, err := result.ToInt64()
	return count > 0, err
}

// JSON helpers
func (c *Client) SetJSON(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	jsonData, err := marshalJSON(value)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return c.Set(ctx, key, string(jsonData), expiration)
}

func (c *Client) GetJSON(ctx context.Context, key string, dest interface{}) error {
	data, err := c.Get(ctx, key)
	if err != nil {
		return err
	}
	if data == "" {
		return nil
	}
	return unmarshalJSON([]byte(data), dest)
}

// Hash helpers
func (c *Client) SetHash(ctx context.Context, key string, fields map[string]string, expiration time.Duration) error {
	if len(fields) == 0 {
		return nil
	}
	for field, value := range fields {
		cmd := c.client.B().Hset().Key(key).FieldValue().FieldValue(field, value).Build()
		if err := c.client.Do(ctx, cmd).Error(); err != nil {
			return err
		}
	}
	if expiration > 0 {
		expireCmd := c.client.B().Expire().Key(key).Seconds(int64(expiration.Seconds())).Build()
		if err := c.client.Do(ctx, expireCmd).Error(); err != nil {
			return fmt.Errorf("failed to set TTL: %w", err)
		}
	}
	return nil
}

func (c *Client) GetHash(ctx context.Context, key string) (map[string]string, error) {
	cmd := c.client.B().Hgetall().Key(key).Build()
	result := c.client.Do(ctx, cmd)
	if result.Error() != nil {
		return nil, result.Error()
	}
	return result.AsStrMap()
}

func (c *Client) GetHashField(ctx context.Context, key, field string) (string, error) {
	cmd := c.client.B().Hget().Key(key).Field(field).Build()
	result := c.client.Do(ctx, cmd)
	if result.Error() != nil {
		if rueidis.IsRedisNil(result.Error()) {
			return "", nil
		}
		return "", result.Error()
	}
	return result.ToString()
}

func (c *Client) DeleteHashFields(ctx context.Context, key string, fields ...string) error {
	if len(fields) == 0 {
		return nil
	}
	cmd := c.client.B().Hdel().Key(key).Field(fields...).Build()
	return c.client.Do(ctx, cmd).Error()
}

func (c *Client) Ping(ctx context.Context) error {
	cmd := c.client.B().Ping().Build()
	return c.client.Do(ctx, cmd).Error()
}

func (c *Client) FlushDB(ctx context.Context) error {
	cmd := c.client.B().Flushdb().Build()
	return c.client.Do(ctx, cmd).Error()
}
