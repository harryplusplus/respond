package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	Host      string       `mapstructure:"host"`
	Port      int          `mapstructure:"port"`
	APIURL    string       `mapstructure:"api_url"`
	APIKeyEnv string       `mapstructure:"api_key_env"`
	Models    []ModelEntry `mapstructure:"models"`
}

type ModelEntry struct {
	Slug                   string   `mapstructure:"slug"`
	DisplayName            string   `mapstructure:"display_name"`
	Priority               int      `mapstructure:"priority"`
	DefaultReasoningEffort string   `mapstructure:"default_reasoning_effort"`
	ContextWindow          int      `mapstructure:"context_window"`
	InputModalities        []string `mapstructure:"input_modalities"`
}

var C Config

func Init() error {
	viper.SetConfigName("respond")
	viper.SetConfigType("toml")
	viper.SetDefault("host", "localhost")
	viper.SetDefault("port", 8080)

	if home := os.Getenv("RESPOND_HOME"); home != "" {
		viper.AddConfigPath(home)
	}

	homeDir, err := os.UserHomeDir()
	if err == nil {
		viper.AddConfigPath(filepath.Join(homeDir, ".respond"))
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return err
		}
	}

	return viper.Unmarshal(&C)
}

func (c Config) BaseURL() string {
	return fmt.Sprintf("http://%s:%d", c.Host, c.Port)
}
