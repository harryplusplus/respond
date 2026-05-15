package goblin

import (
	"context"
	_ "embed"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

type server struct {
	log    *slog.Logger
	srcLog *slog.Logger
	cfg    *GoblinConfig
}

func newServer(cfg *GoblinConfig) *server {
	log, srcLog := newComponentLogger("server")
	return &server{log: log, srcLog: srcLog, cfg: cfg}
}

// Source: https://github.com/openai/codex/blob/main/codex-rs/models-manager/prompt.md (Apache-2.0)
//
//go:embed base_instructions.md
var baseInstructions string

func RunServer() error {
	return newServer(goblinConfig.Load()).run()
}

func NewHandler(cfg *GoblinConfig) http.Handler {
	return newServer(cfg).newHandler()
}

func (s *server) run() error {
	srv := &http.Server{
		Addr:    s.cfg.Address,
		Handler: s.newHandler(),
	}

	listenErrCh := make(chan error, 1)
	notifyCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		s.log.Info("starting server", "addr", s.cfg.Address)
		if err := srv.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				listenErrCh <- err
			}
			stop()
		}
	}()

	<-notifyCtx.Done()
	s.log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	shutdownErr := srv.Shutdown(shutdownCtx)

	select {
	case listenErr := <-listenErrCh:
		return errors.Join(listenErr, shutdownErr)
	default:
		return shutdownErr
	}
}

func (s *server) newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleGetHealthz)
	mux.HandleFunc("GET /models", newModelsHandler(s.cfg).handleGetModels())
	mux.HandleFunc("POST /responses", newResponsesHandler(s.cfg).handlePostResponses())
	return s.loggingMiddleware(mux)
}

func (s *server) handleGetHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status":"ok"}` + "\n")); err != nil {
		s.srcLog.Error("healthz handler: write response", "error", err)
	}
}

func (s *server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
		)
		next.ServeHTTP(w, r)
	})
}
