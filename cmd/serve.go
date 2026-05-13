package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/harryplusplus/respond/internal/config"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the reverse proxy server",
	Long:  `Start the reverse proxy server that converts Codex Responses API to OpenAI Compatibility API.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

		if config.C.APIURL == "" {
			return fmt.Errorf("api_url is required in config file")
		}

		if config.C.APIKeyEnv != "" {
			apiKey := os.Getenv(config.C.APIKeyEnv)
			if apiKey == "" {
				slog.Warn("api key not found", "env", config.C.APIKeyEnv)
			} else {
				slog.Info("api key loaded", "env", config.C.APIKeyEnv)
			}
		}

		slog.Info("Starting server", "host", config.C.Host, "port", config.C.Port, "api_url", config.C.APIURL)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
