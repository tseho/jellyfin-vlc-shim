package logger

import (
	"log/slog"
	"os"
	"strings"
)

var globalLogger *slog.Logger

// Initialize sets up the global logger with the specified log level
func Initialize(logLevel string) {
	level := parseLogLevel(logLevel)

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	globalLogger = slog.New(handler)
	slog.SetDefault(globalLogger)
}

// parseLogLevel converts a string log level to slog.Level
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Get returns the global logger instance
func Get() *slog.Logger {
	if globalLogger == nil {
		// Fallback to default logger if not initialized
		return slog.Default()
	}
	return globalLogger
}
