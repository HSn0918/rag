// Package logger provides centralized logging functionality for the RAG system.
// It follows Uber Go Style Guide conventions for error handling and naming.
package logger

import (
	"fmt"

	"go.uber.org/zap"
)

var (
	// instance holds the global logger instance.
	// Using unexported variable to control access through methods.
	instance *zap.Logger
)

// InitError represents logger initialization errors.
type InitError struct {
	Op  string // the operation that failed
	Err error  // the underlying error
}

func (e *InitError) Error() string {
	return fmt.Sprintf("logger: %s failed: %v", e.Op, e.Err)
}

func (e *InitError) Unwrap() error {
	return e.Err
}

// Init initializes the global logger with production configuration.
// It returns an InitError if logger creation fails.
func Init() error {
	logger, err := zap.NewProduction()
	if err != nil {
		return &InitError{
			Op:  "init production logger",
			Err: err,
		}
	}

	instance = logger
	return nil
}

// InitWithConfig initializes the logger with custom zap configuration.
// It allows for more flexible logger setup in different environments.
func InitWithConfig(config zap.Config) error {
	logger, err := config.Build()
	if err != nil {
		return &InitError{
			Op:  "init logger with config",
			Err: err,
		}
	}

	instance = logger
	return nil
}

// Get returns the global logger instance.
// It creates a production logger if none exists, following fail-safe pattern.
// For consistent naming with Uber Go Style Guide, renamed from GetLogger.
func Get() *zap.Logger {
	if instance == nil {
		// Fallback to production logger if not initialized
		logger, err := zap.NewProduction()
		if err != nil {
			// If even fallback fails, use no-op logger
			logger = zap.NewNop()
		}
		instance = logger
	}
	return instance
}

// MustGet returns the global logger instance or panics if not initialized.
// Use this only when logger initialization failure should terminate the program.
func MustGet() *zap.Logger {
	if instance == nil {
		panic("logger: not initialized, call Init() first")
	}
	return instance
}

// Sync flushes any buffered log entries.
// It's safe to call multiple times and handles nil logger gracefully.
func Sync() error {
	if instance != nil {
		return instance.Sync()
	}
	return nil
}

// IsInitialized reports whether the logger has been initialized.
func IsInitialized() bool {
	return instance != nil
}
