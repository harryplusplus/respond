package goblin

import (
	"cmp"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
)

type modelsHandler struct {
	log        *slog.Logger
	logWithSrc *slog.Logger
	cfg        *GoblinConfig
}

func newModelsHandler(cfg *GoblinConfig) *modelsHandler {
	log, logWithSrc := newComponentLogger("models")
	return &modelsHandler{log: log, logWithSrc: logWithSrc, cfg: cfg}
}

func (h *modelsHandler) handleGetModels() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		models := make([]ModelInfo, 0)
		for _, p := range h.cfg.Providers {
			for _, mo := range p.Models {
				if mo != nil {
					models = append(models, *mo)
				}
			}
		}

		slices.SortStableFunc(models, func(a, b ModelInfo) int {
			return cmp.Compare(*a.Priority, *b.Priority)
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(modelsResponse{Models: models}); err != nil {
			h.logWithSrc.Error("failed to encode models response", "error", err)
		}
	}
}
