package goblin

import (
	"context"
	_ "embed"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Source: https://github.com/openai/codex/blob/main/codex-rs/models-manager/prompt.md (Apache-2.0)
//
//go:embed base_instructions.md
var baseInstructions string

func RunServer() error {
	return runServer(config.Load())
}

func runServer(cfg *Config) error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	srv := &http.Server{
		Addr:    cfg.Address,
		Handler: NewHandler(cfg),
	}

	errCh := make(chan error, 1)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("starting server", "addr", cfg.Address)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	shutdownErr := srv.Shutdown(shutdownCtx)

	select {
	case listenErr := <-errCh:
		return errors.Join(listenErr, shutdownErr)
	default:
		return shutdownErr
	}
}

func NewHandler(cfg *Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleGetHealthz)
	mux.HandleFunc("GET /models", handleGetModels(cfg))
	mux.HandleFunc("POST /responses", handlePostResponses(cfg))
	return loggingMiddleware(mux)
}

func handleGetHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}` + "\n"))
}


func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
		)
		next.ServeHTTP(w, r)
	})
}
