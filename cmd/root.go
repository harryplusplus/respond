/*
Copyright © 2026 harryplusplus <harryplusplus@gmail.com>
*/
package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Version is set via -ldflags at build time, e.g.:
//
//	go build -ldflags="-X github.com/harryplusplus/respond/cmd.Version=0.1.0" .
var Version = "0.0.0"

var rootCmd = &cobra.Command{
	Use:   "respond",
	Short: "Reverse proxy: Codex Responses API -> OpenAI Compatibility API",
	Long: `A reverse proxy server that converts OpenAI's Responses API (Streaming)
to OpenAI Compatibility API for use with Codex.`,
	Version: Version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return initConfig()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func initConfig() error {
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
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return nil
		}
		return err
	}
	return nil
}

func init() {
	rootCmd.SetVersionTemplate("{{.Version}}\n")
}
