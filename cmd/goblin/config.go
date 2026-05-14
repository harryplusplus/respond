package main

import (
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage Goblin configuration",
}

func init() {
	rootCmd.AddCommand(configCmd)
}
