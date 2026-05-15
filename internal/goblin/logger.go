package goblin

import (
	"log/slog"
	"os"
)

var (
	baseLog        = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	baseLogWithSrc = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
	}))
)

func newComponentLogger(name string) (log *slog.Logger, logWithSrc *slog.Logger) {
	return baseLog.With("component", name), baseLogWithSrc.With("component", name)
}
