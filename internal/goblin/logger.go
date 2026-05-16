package goblin

import (
	"log/slog"
	"os"
)

var (
	logBase        = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	logBaseWithSrc = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
	}))
)

type Logger struct {
	log        *slog.Logger
	logWithSrc *slog.Logger
}

func newLogger(name string) Logger {
	return Logger{
		log:        logBase.With("component", name),
		logWithSrc: logBaseWithSrc.With("component", name),
	}
}

func (l *Logger) Debug(msg string, args ...any) {
	l.log.Debug(msg, args...)
}

func (l *Logger) Info(msg string, args ...any) {
	l.log.Info(msg, args...)
}

func (l *Logger) Warn(msg string, args ...any) {
	l.logWithSrc.Warn(msg, args...)
}

func (l *Logger) Error(msg string, args ...any) {
	l.logWithSrc.Error(msg, args...)
}
