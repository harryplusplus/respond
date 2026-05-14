package respond

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/spf13/viper"
)

type Config struct {
	Host      string              `mapstructure:"host"`
	Port      int                 `mapstructure:"port"`
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

func InitConfig() error {
	v := viper.New()
	v.SetConfigName("respond")
	v.SetConfigType("yaml")
	v.SetDefault("host", "localhost")
	v.SetDefault("port", 8080)

	dir, err := respondDir()
	if err != nil {
		return err
	}
	v.AddConfigPath(dir)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
			return err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return err
	}
	if err := parseConfig(&cfg); err != nil {
		return err
	}
	config.Store(&cfg)
	return nil
}

func (c *Config) BaseURL() string {
	return fmt.Sprintf("http://%s:%d", c.Host, c.Port)
}

func parseConfig(cfg *Config) error {
	if cfg.Host != "localhost" {
		ip := net.ParseIP(cfg.Host)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("host must be valid IPv4 address or localhost, got %q", cfg.Host)
		}
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", cfg.Port)
	}
	return nil
}
