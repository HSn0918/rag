package main

import (
	"context"

	"github.com/hsn0918/rag/internal/server"
	"github.com/hsn0918/rag/pkg/logger"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func main() {
	app := fx.New(
		server.Module,
		fx.NopLogger,
	)

	// Start application with timeout
	startCtx, cancel := context.WithTimeout(context.Background(), fx.DefaultTimeout)
	defer cancel()

	if err := app.Start(startCtx); err != nil {
		logger.Get().Fatal("application startup failed", zap.Error(err))
	}

	// Wait for application termination
	<-app.Done()

	// Stop application gracefully
	stopCtx, stopCancel := context.WithTimeout(context.Background(), fx.DefaultTimeout)
	defer stopCancel()

	if err := app.Stop(stopCtx); err != nil {
		logger.Get().Error("application shutdown failed", zap.Error(err))
	}
}
