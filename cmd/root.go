package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/harryplusplus/respond/internal/config"
)

// Version is set via -ldflags at build time, e.g.:
//	go build -ldflags="-X github.com/harryplusplus/respond/cmd.Version=0.1.0" .
var Version = "0.0.0"

var rootCmd = &cobra.Command{
	Use:   "respond",
	Short: "Reverse proxy: Codex Responses API -> OpenAI Compatibility API",
	Long: `A reverse proxy server that converts OpenAI's Responses API (Streaming)
to OpenAI Compatibility API for use with Codex.`,
	Version: Version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return config.Init()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.SetVersionTemplate("{{.Version}}\n")
}
