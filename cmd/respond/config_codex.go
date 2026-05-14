package main

import (
	"github.com/harryplusplus/respond/internal/respond"
	"github.com/spf13/cobra"
)

var configCodexCmd = &cobra.Command{
	Use:   "codex",
	Short: "Generate/update Codex config.toml from Respond config",
	Long: `Reads Respond config and writes the Codex configuration file
($CODEX_HOME/config.toml, falling back to ~/.codex/config.toml).

If the Respond config has model definitions, generates a model
catalog for Codex and links it in the config.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return respond.RunCodexConfig(respond.Cfg.Load())
	},
}

func init() {
	configCmd.AddCommand(configCodexCmd)
}
