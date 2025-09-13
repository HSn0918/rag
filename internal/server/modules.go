package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"github.com/hsn0918/rag/internal/adapters"
	"github.com/hsn0918/rag/internal/clients/embedding"
	"github.com/hsn0918/rag/internal/config"
	"github.com/hsn0918/rag/internal/gen/rag/v1/ragv1connect"
	"github.com/hsn0918/rag/internal/logger"
	"github.com/hsn0918/rag/internal/middleware"
	"github.com/hsn0918/rag/internal/redis"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var Module = fx.Options(
	fx.Provide(
		NewConfig,
		NewLogger,
		NewDatabase,
		NewRedisClient,
		NewCacheService,
		NewClients,
		NewRagServer,
		NewHTTPServer,
	),
	fx.Invoke(StartServer),
)

func NewConfig() (*config.Config, error) {
	cfg, err := config.LoadConfig(".")
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func NewLogger() (*zap.Logger, error) {
	return zap.NewProduction()
}

func NewDatabase(cfg *config.Config) (adapters.VectorDB, error) {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.DBName,
	)

	embeddingModel := cfg.Services.Embedding.Model
	dimensions := embedding.GetDefaultDimensions(embeddingModel)
	logger.Get().Info("使用embedding模型", zap.String("model", embeddingModel), zap.Int("dimensions", dimensions))

	return adapters.NewPostgresVectorDB(dsn, dimensions)
}

func NewRedisClient(cfg *config.Config) (*redis.Client, error) {
	return redis.NewClientFromConfig(*cfg)
}

func NewCacheService(redisClient *redis.Client) *redis.CacheService {
	return redis.NewCacheService(redisClient)
}

func NewRagServer(db adapters.VectorDB, cache *redis.CacheService, clients *Clients, cfg *config.Config) (*RagServer, error) {
	return NewRagServerWithClients(db, cache, clients, *cfg)
}

func NewHTTPServer(ragServer *RagServer, cfg *config.Config) *http.Server {
	mux := http.NewServeMux()

	// 配置Connect RPC选项
	opts := []connect.HandlerOption{
		// 配置JSON编码为下划线格式
		connect.WithCodec(&protoJSONCodec{
			marshaler: protojson.MarshalOptions{
				UseProtoNames:   true, // 使用proto字段名(下划线格式)
				EmitUnpopulated: true, // 输出空值字段
			},
			unmarshaler: protojson.UnmarshalOptions{
				DiscardUnknown: true, // 忽略未知字段
			},
		}),
		// 添加验证中间件
		connect.WithInterceptors(middleware.HTTPValidator()),
	}

	path, handler := ragv1connect.NewRagServiceHandler(ragServer, opts...)
	mux.Handle(path, handler)

	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	logger.Get().Info("服务正在启动", zap.String("address", addr))

	return &http.Server{
		Addr:    addr,
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}
}

type protoJSONCodec struct {
	marshaler   protojson.MarshalOptions
	unmarshaler protojson.UnmarshalOptions
}

func (c *protoJSONCodec) Name() string {
	return "json"
}

func (c *protoJSONCodec) Marshal(v any) ([]byte, error) {
	if msg, ok := v.(interface{ ProtoReflect() protoreflect.Message }); ok {
		return c.marshaler.Marshal(msg.ProtoReflect().Interface())
	}
	return nil, fmt.Errorf("cannot marshal %T", v)
}

func (c *protoJSONCodec) Unmarshal(data []byte, v any) error {
	if msg, ok := v.(interface{ ProtoReflect() protoreflect.Message }); ok {
		return c.unmarshaler.Unmarshal(data, msg.ProtoReflect().Interface())
	}
	return fmt.Errorf("cannot unmarshal to %T", v)
}

func StartServer(httpServer *http.Server, lifecycle fx.Lifecycle, shutdowner fx.Shutdowner) {
	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Get().Info("HTTP server starting", zap.String("addr", httpServer.Addr))
			go func() {
				if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Get().Error("HTTP server failed", zap.Error(err))
					err := shutdowner.Shutdown()
					if err != nil {
						return
					}
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Get().Info("HTTP server stopping")
			return httpServer.Shutdown(ctx)
		},
	})
}
