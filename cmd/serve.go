package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var (
	serveHost    string
	servePort    int
	serveAPIURL string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the reverse proxy server",
	Long: `Start the reverse proxy server that converts Codex Responses API to OpenAI Compatibility API.

Environment:
  API_KEY  API key for upstream OpenAI Compatibility API (optional).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
		if serveAPIURL == "" {
			return fmt.Errorf("--api-url is required")
		}
		apiKey := os.Getenv("API_KEY")
		if apiKey == "" {
			slog.Warn("API_KEY not set")
		}
		slog.Info("Starting server", "host", serveHost, "port", servePort, "upstream", serveAPIURL)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().StringVar(&serveHost, "host", "localhost", "Host to bind the server to")
	serveCmd.Flags().IntVar(&servePort, "port", 8080, "Port to bind the server to")
	serveCmd.Flags().StringVar(&serveAPIURL, "api-url", "", "Upstream OpenAI Compatibility API URL (e.g. https://crof.ai/v1)")
}
