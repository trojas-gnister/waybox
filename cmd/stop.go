package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/trojas-gnister/waybox/internal/config"
	"github.com/trojas-gnister/waybox/internal/libvirt"
	"github.com/trojas-gnister/waybox/internal/vm"
)

var stopCmd = &cobra.Command{
	Use:   "stop <name>",
	Short: "Stop a VM gracefully",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(args[0])
		if err != nil {
			return err
		}
		if err := vm.StopVM(libvirt.NewVirsh(), cfg); err != nil {
			return err
		}
		fmt.Printf("VM %q stopped.\n", cfg.Name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
