package logger

import (
	"context"
	"log/slog"
	"os"
)

type ctxKey struct{}

func Init() {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(h))
}

func InitWithLevel(level slog.Level) {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(h))
}

func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

func WithAttrs(ctx context.Context, args ...any) context.Context {
	l := FromContext(ctx).With(args...)
	return context.WithValue(ctx, ctxKey{}, l)
}