package goblin

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const (
	codexHomeEnv     = "CODEX_HOME"
	goblinProviderID = "goblin"
)

type codexConfig struct {
	raw map[string]any
}

func newCodexConfig(raw map[string]any) *codexConfig {
	return &codexConfig{raw: raw}
}

func (c *codexConfig) modelProvider() string {
	s, _ := c.raw["model_provider"].(string)
	return s
}

func (c *codexConfig) setModelProvider(v string) {
	c.raw["model_provider"] = v
}

func (c *codexConfig) goblinProvider() *codexProvider {
	providers, _ := c.raw["model_providers"].(map[string]any)
	if providers == nil {
		providers = make(map[string]any)
		c.raw["model_providers"] = providers
	}
	p, _ := providers[goblinProviderID].(map[string]any)
	if p == nil {
		p = make(map[string]any)
		providers[goblinProviderID] = p
	}
	return &codexProvider{raw: p}
}

type codexProvider struct {
	raw map[string]any
}

func (p *codexProvider) baseURL() string {
	s, _ := p.raw["base_url"].(string)
	return s
}

func (p *codexProvider) setBaseURL(v string) {
	p.raw["base_url"] = v
}

func (p *codexProvider) name() string {
	s, _ := p.raw["name"].(string)
	return s
}

func (p *codexProvider) setName(v string) {
	p.raw["name"] = v
}

func codexConfigPath(dir string) string {
	return filepath.Join(dir, "config.toml")
}

func runConfigCodex(cfg *GoblinConfig) error {
	dir, err := codexDir()
	if err != nil {
		return err
	}
	codexCfgPath := codexConfigPath(dir)

	codexCfgData, err := os.ReadFile(codexCfgPath)
	if err != nil {
		return fmt.Errorf("read codex config: %w", err)
	}

	var raw map[string]any
	if err := toml.Unmarshal(codexCfgData, &raw); err != nil {
		return fmt.Errorf("parse codex config: %w", err)
	}

	baseURL := cfg.baseURL()
	cc := newCodexConfig(raw)

	if !cc.applyGoblinConfig(baseURL) {
		fmt.Printf("%s: already up to date\n", codexCfgPath)
		return nil
	}

	updated, err := toml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal codex config: %w", err)
	}

	codexBakPath := codexCfgPath + ".bak"
	if err := os.Rename(codexCfgPath, codexBakPath); err != nil {
		return fmt.Errorf("backup codex config: %w", err)
	}

	if err := os.WriteFile(codexCfgPath, updated, 0644); err != nil {
		return fmt.Errorf("write codex config: %w", err)
	}

	fmt.Printf("%s: updated\n", codexCfgPath)
	fmt.Printf("  model_provider = %s\n", goblinProviderID)
	fmt.Printf("  model_providers.goblin.base_url = %s\n", baseURL)
	fmt.Printf("  backup: %s\n", codexBakPath)

	return nil
}

func RunConfigCodex() error {
	return runConfigCodex(goblinConfig.Load())
}

func (c *codexConfig) applyGoblinConfig(baseURL string) bool {
	var changed bool

	if c.modelProvider() != goblinProviderID {
		c.setModelProvider(goblinProviderID)
		changed = true
	}

	p := c.goblinProvider()
	if p.baseURL() != baseURL {
		p.setBaseURL(baseURL)
		changed = true
	}

	// required by codex config validation
	if p.name() != goblinProviderID {
		p.setName(goblinProviderID)
		changed = true
	}

	return changed
}

func codexDir() (string, error) {
	codexHome := os.Getenv(codexHomeEnv)
	if codexHome != "" {
		return codexHome, nil
	}

	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user home directory: %w", err)
	}
	return filepath.Join(dir, ".codex"), nil
}
