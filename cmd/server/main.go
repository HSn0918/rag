package main

import (
	"context"

	"github.com/hsn0918/rag/internal/logger"
	"github.com/hsn0918/rag/internal/server"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func main() {
	if err := logger.Init(); err != nil {
		panic("初始化日志失败: " + err.Error())
	}
	defer logger.Sync()

	app := fx.New(
		server.Module,
		fx.NopLogger,
	)

	// 启动应用并保持运行
	startCtx, cancel := context.WithTimeout(context.Background(), fx.DefaultTimeout)
	defer cancel()

	if err := app.Start(startCtx); err != nil {
		logger.GetLogger().Fatal("应用启动失败", zap.Error(err))
	}

	// 等待应用结束
	<-app.Done()

	// 停止应用
	stopCtx, cancel := context.WithTimeout(context.Background(), fx.DefaultTimeout)
	defer cancel()

	if err := app.Stop(stopCtx); err != nil {
		logger.GetLogger().Error("应用停止失败", zap.Error(err))
	}
}
