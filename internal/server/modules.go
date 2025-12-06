package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/hsn0918/rag/internal/adapters"
	"github.com/hsn0918/rag/internal/gen/rag/v1/ragv1connect"
	pkgdoc2x "github.com/hsn0918/rag/pkg/clients/doc2x"
	pkgembedding "github.com/hsn0918/rag/pkg/clients/embedding"
	pkgopenai "github.com/hsn0918/rag/pkg/clients/openai"
	pkgrerank "github.com/hsn0918/rag/pkg/clients/rerank"
	"github.com/hsn0918/rag/pkg/config"
	"github.com/hsn0918/rag/pkg/logger"
	"github.com/hsn0918/rag/pkg/middleware"
	"github.com/hsn0918/rag/pkg/redis"
	"github.com/hsn0918/rag/pkg/storage"
	"go.uber.org/fx"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Module 是主要的FX依赖注入模块
var Module = fx.Options(
	// 基础设施模块
	InfrastructureModule,
	// 客户端模块
	ClientsModule,
	// 服务模块
	ServicesModule,
	// HTTP服务器模块
	HTTPServerModule,
	// 启动器
	fx.Invoke(StartHTTPServer),
)

// InfrastructureModule 基础设施模块 - 配置、日志、数据库、缓存
var InfrastructureModule = fx.Module("infrastructure",
	fx.Provide(
		NewAppConfig,
		NewAppLogger,
		NewVectorDatabase,
		NewRedisConnection,
		NewCacheService,
	),
)

// ClientsModule 客户端模块 - 外部服务客户端
var ClientsModule = fx.Module("clients",
	fx.Provide(
		NewExternalClients,
	),
)

// ServicesModule 服务模块 - 业务逻辑服务
var ServicesModule = fx.Module("services",
	fx.Provide(
		NewRagService,
	),
)

// HTTPServerModule HTTP服务器模块
var HTTPServerModule = fx.Module("http_server",
	fx.Provide(
		NewHTTPHandler,
	),
)

// ================================
// 基础设施构造函数
// ================================

// NewAppConfig 创建应用配置
func NewAppConfig() (*config.Config, error) {
	cfg, err := config.LoadConfig(".")
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	return cfg, nil
}

// NewAppLogger 创建应用日志器
func NewAppLogger() (*slog.Logger, error) {
	if err := logger.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}
	return logger.Get(), nil
}

// NewVectorDatabase 创建向量数据库连接
func NewVectorDatabase(cfg *config.Config) (adapters.VectorDB, error) {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.DBName,
	)

	embeddingModel := cfg.Services.Embedding.Model
	dimensions := pkgembedding.GetDefaultDimensions(embeddingModel)
	logger.Get().Info("初始化向量数据库",
		"model", embeddingModel,
		"dimensions", dimensions)

	db, err := adapters.NewPostgresVectorDB(dsn, dimensions)
	if err != nil {
		return nil, fmt.Errorf("failed to create vector database: %w", err)
	}
	return db, nil
}

// NewRedisConnection 创建Redis连接
func NewRedisConnection(cfg *config.Config) (*redis.Client, error) {
	client, err := redis.NewClientFromConfig(*cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis client: %w", err)
	}
	return client, nil
}

// NewCacheService 创建缓存服务
func NewCacheService(redisClient *redis.Client) *redis.CacheService {
	return redis.NewCacheService(redisClient)
}

// ================================
// 客户端构造函数
// ================================

// NewClients 根据配置创建所有客户端 (向后兼容函数)
func NewClients(cfg *config.Config) (*ExternalClients, error) {
	// 创建 MinIO 客户端
	minioClient, err := storage.NewMinIOClient(storage.MinIOConfig{
		Endpoint:        cfg.MinIO.Endpoint,
		AccessKeyID:     cfg.MinIO.AccessKeyID,
		SecretAccessKey: cfg.MinIO.SecretAccessKey,
		BucketName:      cfg.MinIO.BucketName,
		UseSSL:          cfg.MinIO.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	return &ExternalClients{
		Doc2X:     pkgdoc2x.NewClient(cfg.Services.Doc2X),
		Embedding: pkgembedding.NewClient(cfg.Services.Embedding.ServiceConfig),
		LLM:       pkgopenai.NewClient(cfg.Services.LLM),
		Reranker:  pkgrerank.NewClient(cfg.Services.Reranker),
		Storage:   minioClient,
	}, nil
}

// NewExternalClients 创建所有外部服务客户端
func NewExternalClients(cfg *config.Config) (*ExternalClients, error) {
	return NewClients(cfg)
}

// ================================
// 服务构造函数
// ================================

// NewRagService 创建RAG服务
func NewRagService(
	db adapters.VectorDB,
	cache *redis.CacheService,
	clients *ExternalClients,
	cfg *config.Config,
) (*RagServer, error) {
	// 创建RAG服务实例
	server := &RagServer{
		DB:        db,
		Cache:     cache,
		Storage:   clients.Storage,
		Doc2X:     clients.Doc2X,
		Embedding: clients.Embedding,
		LLM:       clients.LLM,
		Reranker:  clients.Reranker,
		Config:    cfg,
	}

	// 初始化搜索优化器
	searchOptimizer, err := NewSearchOptimizer(
		server,
		20, // 初始候选数
		5,  // 最终结果数
		WithMinSimilarity(0.25),
		WithParallelScoring(true),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create search optimizer: %w", err)
	}

	server.SearchOptimizer = searchOptimizer
	return server, nil
}

// ================================
// HTTP服务器构造函数
// ================================

// NewHTTPHandler 创建HTTP处理器
func NewHTTPHandler(ragService *RagServer, cfg *config.Config) *http.Server {
	mux := http.NewServeMux()

	// Connect RPC选项配置
	connectOpts := []connect.HandlerOption{
		connect.WithCodec(&ProtoJSONCodec{
			marshaler: protojson.MarshalOptions{
				UseProtoNames:   true, // 使用proto字段名(下划线格式)
				EmitUnpopulated: true, // 输出空值字段
			},
			unmarshaler: protojson.UnmarshalOptions{
				DiscardUnknown: true, // 忽略未知字段
			},
		}),
		connect.WithInterceptors(middleware.HTTPValidator()),
	}

	// 注册RPC服务处理器
	path, handler := ragv1connect.NewRagServiceHandler(ragService, connectOpts...)
	mux.Handle(path, handler)

	serverAddr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	logger.Get().Info("HTTP服务器配置完成", "address", serverAddr)

	return &http.Server{
		Addr:    serverAddr,
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}
}

// ================================
// 生命周期管理
// ================================

// StartHTTPServer 启动HTTP服务器
func StartHTTPServer(httpServer *http.Server, lifecycle fx.Lifecycle, shutdowner fx.Shutdowner) {
	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Get().Info("启动HTTP服务器", "addr", httpServer.Addr)
			go func() {
				if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Get().Error("HTTP服务器启动失败", "error", err)
					if shutdownErr := shutdowner.Shutdown(); shutdownErr != nil {
						logger.Get().Error("应用程序关闭失败", "error", shutdownErr)
					}
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Get().Info("停止HTTP服务器")
			return httpServer.Shutdown(ctx)
		},
	})
}

// ================================
// 辅助类型和函数
// ================================

// ProtoJSONCodec Connect RPC的JSON编解码器
type ProtoJSONCodec struct {
	marshaler   protojson.MarshalOptions
	unmarshaler protojson.UnmarshalOptions
}

func (c *ProtoJSONCodec) Name() string {
	return "json"
}

func (c *ProtoJSONCodec) Marshal(v any) ([]byte, error) {
	if msg, ok := v.(interface{ ProtoReflect() protoreflect.Message }); ok {
		return c.marshaler.Marshal(msg.ProtoReflect().Interface())
	}
	return nil, fmt.Errorf("cannot marshal %T", v)
}

func (c *ProtoJSONCodec) Unmarshal(data []byte, v any) error {
	if msg, ok := v.(interface{ ProtoReflect() protoreflect.Message }); ok {
		return c.unmarshaler.Unmarshal(data, msg.ProtoReflect().Interface())
	}
	return fmt.Errorf("cannot unmarshal to %T", v)
}
