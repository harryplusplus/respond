/*
Copyright © 2026 harryplusplus <harryplusplus@gmail.com>
*/
package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the reverse proxy server",
	Long:  `Start the reverse proxy server that converts Codex Responses API to OpenAI Compatibility API.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

		host := viper.GetString("host")
		port := viper.GetInt("port")
		apiURL := viper.GetString("api_url")
		if apiURL == "" {
			return fmt.Errorf("api_url is required in config file")
		}

		if envKey := viper.GetString("api_key_env"); envKey != "" {
			apiKey := os.Getenv(envKey)
			if apiKey == "" {
				slog.Warn("api key not found", "env", envKey)
			} else {
				slog.Info("api key loaded", "env", envKey)
			}
		}

		slog.Info("Starting server", "host", host, "port", port, "api_url", apiURL)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
