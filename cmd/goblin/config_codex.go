package main

import (
	"github.com/harryplusplus/goblin/internal/goblin"
	"github.com/spf13/cobra"
)

var configCodexCmd = &cobra.Command{
	Use:   "codex",
	Short: "Generate/update Codex config.toml from Goblin config",
	Long: `Reads Goblin config and writes the Codex configuration file
($CODEX_HOME/config.toml, falling back to ~/.codex/config.toml).

If the Goblin config has model definitions, generates a model
catalog for Codex and links it in the config.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return goblin.RunConfigCodex()
	},
}

func init() {
	configCmd.AddCommand(configCodexCmd)
}
