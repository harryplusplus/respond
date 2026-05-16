package goblin

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

type GoblinConfig struct {
	Address   string              `yaml:"address"`
	Providers map[string]Provider `yaml:"providers"`
}

func (c *GoblinConfig) baseURL() string {
	return "http://" + c.Address
}

type Provider struct {
	BaseURL string                `yaml:"base_url"`
	EnvKey  string                `yaml:"env_key"`
	Models  map[string]*ModelInfo `yaml:"models"`
}

const (
	goblinHomeEnv  = "GOBLIN_HOME"
	defaultAddress = "localhost:8080"
)

func goblinDir() (string, error) {
	home := os.Getenv(goblinHomeEnv)
	if home != "" {
		return home, nil
	}

	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user home directory: %w", err)
	}
	return filepath.Join(dir, ".goblin"), nil
}

func goblinConfigPath(dir string) string {
	return filepath.Join(dir, "goblin.yaml")
}

func loadGoblinConfig() (*GoblinConfig, error) {
	dir, err := goblinDir()
	if err != nil {
		return nil, err
	}

	cfg := &GoblinConfig{}

	data, err := os.ReadFile(goblinConfigPath(dir))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, parseGoblinConfig(cfg)
}

func parseGoblinConfig(cfg *GoblinConfig) error {
	if cfg.Address == "" {
		cfg.Address = defaultAddress
	}

	host, port, err := net.SplitHostPort(cfg.Address)
	if err != nil {
		return fmt.Errorf("invalid address %q: %w", cfg.Address, err)
	}
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("host must be localhost or a valid IPv4 address, got %q", host)
		}
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid port %q in address %q", port, cfg.Address)
	}
	if portNum < 1 || portNum > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", portNum)
	}

	for name, p := range cfg.Providers {
		p.BaseURL = strings.TrimRight(p.BaseURL, "/")
		cfg.Providers[name] = p
	}

	HydrateModels(cfg)
	return nil
}

func HydrateModels(cfg *GoblinConfig) {
	for name, p := range cfg.Providers {
		for slug, m := range p.Models {
			if m == nil {
				m = &ModelInfo{}
			}
			m.Slug = name + "/" + slug
			fillModelDefaults(m)
			p.Models[slug] = m
		}
	}
}
