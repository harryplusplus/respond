package main

import (
	"github.com/spf13/cobra"

	"github.com/harryplusplus/respond/internal/respond"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the reverse proxy server",
	Long:  `Start the reverse proxy server that converts Codex Responses API to OpenAI Compatibility API.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return respond.RunServer()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
