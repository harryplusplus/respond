package goblin

import (
	"cmp"
	"slices"
)

type ModelsManager struct {
	cfg *Config
}

func NewModelsManager(cfg *Config) *ModelsManager {
	return &ModelsManager{cfg: cfg}
}

func (m *ModelsManager) Models() ModelsResponse {
	models := make([]ModelInfo, 0)

	for _, p := range m.cfg.Providers {
		for _, mo := range p.Models {
			if mo != nil {
				models = append(models, *mo)
			}
		}
	}

	slices.SortStableFunc(models, func(a, b ModelInfo) int {
		return cmp.Compare(*a.Priority, *b.Priority)
	})

	return ModelsResponse{Models: models}
}
