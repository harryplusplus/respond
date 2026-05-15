package goblin

import (
	"cmp"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
)

func handleGetModels(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		models := make([]ModelInfo, 0)
		for _, p := range cfg.Providers {
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

		if err := json.NewEncoder(w).Encode(ModelsResponse{Models: models}); err != nil {
			slog.Error("failed to encode models response", "error", err)
		}
	}
}
