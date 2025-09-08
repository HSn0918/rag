package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/hsn0918/rag/internal/adapters"
	"github.com/hsn0918/rag/internal/config"
	"github.com/hsn0918/rag/internal/gen/proto/rag/v1/ragv1connect"
	"github.com/hsn0918/rag/internal/redis"
	"github.com/hsn0918/rag/internal/server"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func main() {
	// 1. 加载配置
	// 调用 internal/config/config.go 中的 LoadConfig 函数
	cfg, err := config.LoadConfig(".") // 从 . 目录加载 config.yaml
	if err != nil {
		log.Fatalf("无法加载配置: %v", err)
	}

	// 2. 根据配置构建数据库连接字符串 (DSN)
	// 使用从 Config 结构体中解析出的字段
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.DBName,
	)

	// 3. 初始化数据库适配器，传入连接字符串和嵌入维度
	db, err := adapters.NewPostgresVectorDB(dsn, 4096) // Qwen3-Embedding-8B 使用 4096 维度
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}

	// 4. 初始化 Redis 客户端
	redisClient, err := redis.NewClientFromConfig(cfg)
	if err != nil {
		log.Fatalf("Redis 客户端初始化失败: %v", err)
	}
	defer redisClient.Close()

	// 创建缓存服务
	cacheService := redis.NewCacheService(redisClient)

	// 5. 初始化所有服务客户端
	clients, err := server.NewClients(cfg)
	if err != nil {
		log.Fatalf("客户端初始化失败: %v", err)
	}

	// 6. 初始化 ConnectRPC 服务，传入所有依赖
	ragServer := server.NewRagServerWithClients(db, cacheService, clients, cfg)

	// 7. 设置路由并启动服务器
	mux := http.NewServeMux()
	path, handler := ragv1connect.NewRagServiceHandler(ragServer)
	mux.Handle(path, handler)

	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	log.Printf("服务正在启动，监听地址: %s\n", addr)

	// 使用 h2c 以允许不安全的 HTTP/2 连接（用于本地开发）
	err = http.ListenAndServe(
		addr,
		h2c.NewHandler(mux, &http2.Server{}),
	)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("监听失败: %v", err)
	}
}
