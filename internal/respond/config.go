package respond

import (
	"errors"
	"fmt"
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
	KeyEnv  string           `mapstructure:"key_env"`
	Models  map[string]Model `mapstructure:"models"`
}

type Model struct {
}

var config atomic.Pointer[Config]

func InitConfig() error {
	viper.SetConfigName("respond")
	viper.SetConfigType("yaml")
	viper.SetDefault("host", "localhost")
	viper.SetDefault("port", 8080)

	if home := os.Getenv("RESPOND_HOME"); home != "" {
		viper.AddConfigPath(home)
	} else if dir, err := os.UserHomeDir(); err == nil {
		viper.AddConfigPath(filepath.Join(dir, ".respond"))
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
			return err
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return err
	}
	config.Store(&cfg)
	return nil
}

func (c *Config) BaseURL() string {
	return fmt.Sprintf("http://%s:%d", c.Host, c.Port)
}
