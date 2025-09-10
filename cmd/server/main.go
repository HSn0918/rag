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
		panic("logger initialization failed: " + err.Error())
	}
	defer func() {
		if syncErr := logger.Sync(); syncErr != nil {
		}
	}()

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
