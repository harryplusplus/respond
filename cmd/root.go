/*
Copyright © 2026 harryplusplus <harryplusplus@gmail.com>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// Version is set via -ldflags at build time, e.g.:
//   go build -ldflags="-X github.com/harryplusplus/codex-compat/cmd.Version=0.1.0" .
var Version = "0.0.0"

var rootCmd = &cobra.Command{
	Use:     "codex-compat",
	Short:   "Reverse proxy: Codex Responses API -> OpenAI Compatibility API",
	Long: `A reverse proxy server that converts OpenAI's Responses API (Streaming)
to OpenAI Compatibility API for use with Codex app.`,
	Version: Version,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.SetVersionTemplate("{{.Version}}\n")
}
