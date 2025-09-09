package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type ServiceConfig struct {
	BaseURL string `mapstructure:"base_url"`
	APIKey  string `mapstructure:"api_key"`
	Model   string `mapstructure:"model"`
}

type ChunkingConfig struct {
	MaxChunkSize      int     `mapstructure:"max_chunk_size"`
	OverlapSize       int     `mapstructure:"overlap_size"`
	MinChunkSize      int     `mapstructure:"min_chunk_size"`
	SentenceBoundary  bool    `mapstructure:"sentence_boundary"`
	ParagraphBoundary bool    `mapstructure:"paragraph_boundary"`
	AdaptiveSize      bool    `mapstructure:"adaptive_size"`
	SizeMultiplier    float64 `mapstructure:"size_multiplier"`
}

type Config struct {
	Server struct {
		Host string `mapstructure:"host"`
		Port string `mapstructure:"port"`
	} `mapstructure:"server"`
	Database struct {
		Host     string `mapstructure:"host"`
		Port     int    `mapstructure:"port"`
		User     string `mapstructure:"user"`
		Password string `mapstructure:"password"`
		DBName   string `mapstructure:"dbname"`
	} `mapstructure:"database"`
	Redis struct {
		Host     string `mapstructure:"host"`
		Port     int    `mapstructure:"port"`
		Password string `mapstructure:"password"`
		DB       int    `mapstructure:"db"`
	} `mapstructure:"redis"`
	MinIO struct {
		Endpoint        string `mapstructure:"endpoint"`
		AccessKeyID     string `mapstructure:"access_key_id"`
		SecretAccessKey string `mapstructure:"secret_access_key"`
		BucketName      string `mapstructure:"bucket_name"`
		UseSSL          bool   `mapstructure:"use_ssl"`
	} `mapstructure:"minio"`
	Chunking ChunkingConfig `mapstructure:"chunking"`
	Services struct {
		Doc2X     ServiceConfig `mapstructure:"doc2x"`
		Embedding struct {
			ServiceConfig `mapstructure:",squash"`
		} `mapstructure:"embedding"`
		Reranker ServiceConfig `mapstructure:"reranker"`
		LLM      ServiceConfig `mapstructure:"llm"`
	} `mapstructure:"services"`
}

func LoadConfig(path string) (config Config, err error) {
	// Set default values for chunking
	viper.SetDefault("chunking.max_chunk_size", 512)
	viper.SetDefault("chunking.overlap_size", 50)
	viper.SetDefault("chunking.min_chunk_size", 100)
	viper.SetDefault("chunking.sentence_boundary", true)
	viper.SetDefault("chunking.paragraph_boundary", true)
	viper.SetDefault("chunking.adaptive_size", true)
	viper.SetDefault("chunking.size_multiplier", 1.5)

	viper.AddConfigPath(path)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.AutomaticEnv()

	if err = viper.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("failed to read config file: %w", err)
	}

	if err = viper.Unmarshal(&config); err != nil {
		return Config{}, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return
}
