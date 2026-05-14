package respond

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
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

var config Config

func InitConfig() error {
	viper.SetConfigName("respond")
	viper.SetConfigType("yaml")
	viper.SetDefault("host", "localhost")
	viper.SetDefault("port", 8080)

	if home := os.Getenv("RESPOND_HOME"); home != "" {
		viper.AddConfigPath(home)
	}

	if dir, err := os.UserHomeDir(); err == nil {
		viper.AddConfigPath(filepath.Join(dir, ".respond"))
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
			return err
		}
	}

	viper.OnConfigChange(func(e fsnotify.Event) {
		fmt.Println("Config file changed:", e.Name)
	})

	viper.WatchConfig()

	return viper.Unmarshal(&config)
}

func (c Config) BaseURL() string {
	return fmt.Sprintf("http://%s:%d", c.Host, c.Port)
}
