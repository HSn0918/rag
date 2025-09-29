// Package config provides configuration management for the RAG system.
// It follows Uber Go Style Guide conventions for struct organization and error handling.
package config

import (
	"errors"
	"fmt"

	"github.com/spf13/viper"
)

// Common configuration errors
var (
	ErrConfigNotFound = errors.New("configuration file not found")
	ErrInvalidConfig  = errors.New("invalid configuration")
)

// ServiceConfig holds common configuration for external service clients.
// Fields are organized by logical grouping and include validation tags.
type ServiceConfig struct {
	// Connection settings
	BaseURL string `mapstructure:"base_url" validate:"required,url"`
	APIKey  string `mapstructure:"api_key" validate:"required"`

	// Service settings
	Model string `mapstructure:"model" validate:"required"`
}

// ChunkingConfig defines text chunking parameters.
// Fields are organized by feature with validation tags.
type ChunkingConfig struct {
	// Size constraints (required)
	MaxChunkSize int `mapstructure:"max_chunk_size" validate:"required,min=100,max=10000"`
	MinChunkSize int `mapstructure:"min_chunk_size" validate:"required,min=50"`
	OverlapSize  int `mapstructure:"overlap_size" validate:"min=0"`

	// Semantic processing (optional)
	EnableSemantic      bool    `mapstructure:"enable_semantic"`
	SimilarityThreshold float64 `mapstructure:"similarity_threshold" validate:"min=0.0,max=1.0"`
}

// Validate checks the chunking configuration and sets defaults.
func (c *ChunkingConfig) Validate() error {
	// Set defaults for zero values
	if c.MaxChunkSize == 0 {
		c.MaxChunkSize = 2000
	}
	if c.MinChunkSize == 0 {
		c.MinChunkSize = 200
	}
	if c.OverlapSize == 0 {
		c.OverlapSize = 200
	}
	if c.SimilarityThreshold == 0 {
		c.SimilarityThreshold = 0.75
	}

	// Validation rules
	if c.MinChunkSize >= c.MaxChunkSize {
		return fmt.Errorf("%w: min chunk size must be less than max chunk size", ErrInvalidConfig)
	}
	if c.OverlapSize >= c.MaxChunkSize {
		return fmt.Errorf("%w: overlap size must be less than max chunk size", ErrInvalidConfig)
	}

	return nil
}

// Config represents the complete application configuration.
// Structs are organized by functional domain with clear separation.
type Config struct {
	// Server configuration
	Server struct {
		Host string `mapstructure:"host" validate:"required"`
		Port string `mapstructure:"port" validate:"required,numeric"`
	} `mapstructure:"server"`

	// Database configuration
	Database struct {
		Host     string `mapstructure:"host" validate:"required,hostname"`
		Port     int    `mapstructure:"port" validate:"required,min=1,max=65535"`
		User     string `mapstructure:"user" validate:"required"`
		Password string `mapstructure:"password" validate:"required"`
		DBName   string `mapstructure:"dbname" validate:"required"`
	} `mapstructure:"database"`

	// Cache configuration
	Redis struct {
		Host     string `mapstructure:"host" validate:"required,hostname"`
		Port     int    `mapstructure:"port" validate:"required,min=1,max=65535"`
		Password string `mapstructure:"password"`
		DB       int    `mapstructure:"db" validate:"min=0,max=15"`
	} `mapstructure:"redis"`

	// Object storage configuration
	MinIO struct {
		Endpoint        string `mapstructure:"endpoint" validate:"required,url"`
		AccessKeyID     string `mapstructure:"access_key_id" validate:"required"`
		SecretAccessKey string `mapstructure:"secret_access_key" validate:"required"`
		BucketName      string `mapstructure:"bucket_name" validate:"required"`
		UseSSL          bool   `mapstructure:"use_ssl"`
	} `mapstructure:"minio"`

	// Processing configuration
	Chunking ChunkingConfig `mapstructure:"chunking"`

	// External services configuration
	Services struct {
		Doc2X     ServiceConfig `mapstructure:"doc2x"`
		Embedding struct {
			ServiceConfig `mapstructure:",squash"`
		} `mapstructure:"embedding"`
		Reranker ServiceConfig `mapstructure:"reranker"`
		LLM      ServiceConfig `mapstructure:"llm"`
	} `mapstructure:"services"`
}

// Validate performs configuration validation and sets defaults.
func (c *Config) Validate() error {
	// Validate chunking configuration
	if err := c.Chunking.Validate(); err != nil {
		return fmt.Errorf("chunking config: %w", err)
	}

	// Additional validation logic can be added here
	// such as checking database connectivity, service availability, etc.

	return nil
}

// LoadConfig loads configuration from file and environment variables.
// It follows Uber Go Style Guide error handling patterns.
func LoadConfig(configPath string) (*Config, error) {
	// Configure viper
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(configPath)
	viper.AutomaticEnv()

	// Set intelligent defaults
	setDefaults()

	// Read configuration
	if err := viper.ReadInConfig(); err != nil {
		if errors.As(err, &viper.ConfigFileNotFoundError{}) {
			return nil, fmt.Errorf("%w: %v", ErrConfigNotFound, err)
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Unmarshal into struct
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// setDefaults configures sensible default values.
func setDefaults() {
	// Server defaults
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", "8080")

	// Chunking defaults
	viper.SetDefault("chunking.max_chunk_size", 512)
	viper.SetDefault("chunking.min_chunk_size", 100)
	viper.SetDefault("chunking.overlap_size", 50)
	viper.SetDefault("chunking.sentence_boundary", true)
	viper.SetDefault("chunking.paragraph_boundary", true)
	viper.SetDefault("chunking.adaptive_size", true)
	viper.SetDefault("chunking.size_multiplier", 1.5)

	// Redis defaults
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.db", 0)

	// MinIO defaults
	viper.SetDefault("minio.use_ssl", false)
}

// MustLoadConfig loads configuration and panics on failure.
// Use this only in main() or init() functions where failure should be fatal.
func MustLoadConfig(configPath string) *Config {
	config, err := LoadConfig(configPath)
	if err != nil {
		panic(fmt.Sprintf("failed to load configuration: %v", err))
	}
	return config
}
