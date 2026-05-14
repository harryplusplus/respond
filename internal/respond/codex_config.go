package respond

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const (
	codexHomeEnv      = "CODEX_HOME"
	respondProviderID = "respond"
)

func codexConfigPath(dir string) string {
	return filepath.Join(dir, "config.toml")
}

func RunCodexConfig() error {
	dir, err := codexDir()
	if err != nil {
		return err
	}
	codexCfgPath := codexConfigPath(dir)

	codexCfgData, err := os.ReadFile(codexCfgPath)
	if err != nil {
		return fmt.Errorf("read codex config: %w", err)
	}

	var codexCfg map[string]any
	if err := toml.Unmarshal(codexCfgData, &codexCfg); err != nil {
		return fmt.Errorf("parse codex config: %w", err)
	}

	baseURL := config.Load().BaseURL()

	if !applyRespondConfig(codexCfg, baseURL) {
		fmt.Printf("%s: already up to date\n", codexCfgPath)
		return nil
	}

	updated, err := toml.Marshal(codexCfg)
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
	fmt.Printf("  model_provider = %s\n", respondProviderID)
	fmt.Printf("  model_providers.respond.base_url = %s\n", baseURL)
	fmt.Printf("  backup: %s\n", codexBakPath)

	return nil
}

func codexModelProvider(cfg map[string]any) string {
	s, _ := cfg["model_provider"].(string)
	return s
}

func setCodexModelProvider(cfg map[string]any, v string) {
	cfg["model_provider"] = v
}

func codexProvider(cfg map[string]any, id string) map[string]any {
	providers, _ := cfg["model_providers"].(map[string]any)
	if providers == nil {
		providers = make(map[string]any)
		cfg["model_providers"] = providers
	}
	p, _ := providers[id].(map[string]any)
	if p == nil {
		p = make(map[string]any)
		providers[id] = p
	}
	return p
}

func codexProviderBaseURL(p map[string]any) string {
	s, _ := p["base_url"].(string)
	return s
}

func setCodexProviderBaseURL(p map[string]any, v string) {
	p["base_url"] = v
}

func applyRespondConfig(cfg map[string]any, baseURL string) bool {
	var changed bool

	if codexModelProvider(cfg) != respondProviderID {
		setCodexModelProvider(cfg, respondProviderID)
		changed = true
	}

	p := codexProvider(cfg, respondProviderID)
	if codexProviderBaseURL(p) != baseURL {
		setCodexProviderBaseURL(p, baseURL)
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
