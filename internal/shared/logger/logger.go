package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

const DefaultLevel = "info"

func NormalizeLevel(value string) (string, error) {
	level := strings.ToLower(strings.TrimSpace(value))
	if level == "" {
		return DefaultLevel, nil
	}
	switch level {
	case "debug", "info", "warn", "warning", "error":
		if level == "warning" {
			return "warn", nil
		}
		return level, nil
	default:
		return "", fmt.Errorf("invalid log level %q; expected debug, info, warn, or error", value)
	}
}

func Configure(value string) error {
	normalized, err := NormalizeLevel(value)
	if err != nil {
		return err
	}
	var level slog.Level
	switch normalized {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	return nil
}

func Debug(ctx context.Context, message string, args ...any) {
	slog.Default().DebugContext(ctx, message, args...)
}
func Info(ctx context.Context, message string, args ...any) {
	slog.Default().InfoContext(ctx, message, args...)
}
func Warn(ctx context.Context, message string, args ...any) {
	slog.Default().WarnContext(ctx, message, args...)
}
func Error(ctx context.Context, message string, args ...any) {
	slog.Default().ErrorContext(ctx, message, args...)
}
