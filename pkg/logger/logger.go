// Package logger provides centralized logging functionality for the RAG system.
// It follows Uber Go Style Guide conventions for error handling and naming.
package logger

import (
	"fmt"
	"log/slog"
	"os"
)

var (
	// instance holds the global logger instance.
	// Using unexported variable to control access through methods.
	instance *slog.Logger
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

// Init initializes the global logger with production-style JSON handler.
// It returns an InitError if logger creation fails.
func Init() error {
	return InitWithConfig(slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
}

// InitWithConfig initializes the logger with custom slog handler options.
// It allows for more flexible logger setup in different environments.
func InitWithConfig(opts slog.HandlerOptions) error {
	handler := slog.NewJSONHandler(os.Stdout, &opts)
	instance = slog.New(handler)
	return nil
}

// Get returns the global logger instance.
// It creates a default logger if none exists, following fail-safe pattern.
// For consistent naming with Uber Go Style Guide, renamed from GetLogger.
func Get() *slog.Logger {
	if instance == nil {
		// Fallback to default logger if not initialized
		_ = Init()
	}
	return instance
}

// MustGet returns the global logger instance or panics if not initialized.
// Use this only when logger initialization failure should terminate the program.
func MustGet() *slog.Logger {
	if instance == nil {
		panic("logger: not initialized, call Init() first")
	}
	return instance
}

// Sync flushes any buffered log entries if supported by handler.
// It's safe to call multiple times and handles nil logger gracefully.
func Sync() error {
	if instance == nil {
		return nil
	}

	// Support handlers that expose Sync/Close semantics.
	type syncer interface {
		Sync() error
	}
	if s, ok := instance.Handler().(syncer); ok {
		return s.Sync()
	}
	if c, ok := instance.Handler().(interface{ Close() error }); ok {
		return c.Close()
	}
	return nil
}

// IsInitialized reports whether the logger has been initialized.
func IsInitialized() bool {
	return instance != nil
}
