package respond

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"

	"github.com/spf13/viper"
)

type Config struct {
	Address   string              `mapstructure:"address"`
	Providers map[string]Provider `mapstructure:"providers"`
}

type Provider struct {
	BaseURL string           `mapstructure:"base_url"`
	EnvKey  string           `mapstructure:"env_key"`
	Models  map[string]Model `mapstructure:"models"`
}

type Model struct {
}

var config atomic.Pointer[Config]

const respondHomeEnv = "RESPOND_HOME"

func respondDir() (string, error) {
	home := os.Getenv(respondHomeEnv)
	if home != "" {
		return home, nil
	}

	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user home directory: %w", err)
	}
	return filepath.Join(dir, ".respond"), nil
}

func respondConfigPath(dir string) string {
	return filepath.Join(dir, "respond.yaml")
}

func loadConfig() (*Config, error) {
	v := viper.New()
	v.SetConfigName("respond")
	v.SetConfigType("yaml")
	v.SetDefault("address", "localhost:8080")

	dir, err := respondDir()
	if err != nil {
		return nil, err
	}
	v.AddConfigPath(dir)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	if err := parseConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func InitConfig() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	config.Store(cfg)
	return nil
}

func (c *Config) baseURL() string {
	return "http://" + c.Address
}

func parseConfig(cfg *Config) error {
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
	return nil
}
