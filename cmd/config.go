/*
Copyright © 2026 harryplusplus <harryplusplus@gmail.com>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage Respond configuration",
}

func init() {
	rootCmd.AddCommand(configCmd)
}
