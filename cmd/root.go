package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "waybox",
	Short: "Isolated application VMs with native Wayland integration",
	Long: `waybox creates lightweight NixOS VMs where each VM serves a single application.
Apps appear as regular windows on your host Wayland desktop via waypipe over vsock.`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
