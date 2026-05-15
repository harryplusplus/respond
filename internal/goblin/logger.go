package goblin

import (
	"log/slog"
	"os"
)

var (
	baseLog    = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	baseSrcLog = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
	}))
)

func newComponentLogger(name string) (log *slog.Logger, srcLog *slog.Logger) {
	return baseLog.With("component", name), baseSrcLog.With("component", name)
}
