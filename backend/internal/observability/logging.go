package observability

import (
	"context"
	"log/slog"
	"os"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"
const userIDKey ctxKey = "user_id"

func NewLogger(env string) *slog.Logger {
	level := slog.LevelInfo
	opts := &slog.HandlerOptions{Level: level}
	if env == "development" {
		opts.Level = slog.LevelDebug
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

// WithRequestID 将 request_id 注入 context。
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// WithUserID 将 user_id 注入 context。
func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

// RequestID 从 context 读取 request_id。
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// UserID 从 context 读取 user_id。
func UserID(ctx context.Context) int64 {
	if v, ok := ctx.Value(userIDKey).(int64); ok {
		return v
	}
	return 0
}

// Logger 返回带 request_id / user_id 上下文字段的 logger。
func Logger(ctx context.Context) *slog.Logger {
	attrs := []any{}
	if rid := RequestID(ctx); rid != "" {
		attrs = append(attrs, "request_id", rid)
	}
	if uid := UserID(ctx); uid != 0 {
		attrs = append(attrs, "user_id", uid)
	}
	return slog.Default().With(attrs...)
}
