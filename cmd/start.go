package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/trojas-gnister/waybox/internal/config"
	"github.com/trojas-gnister/waybox/internal/libvirt"
	"github.com/trojas-gnister/waybox/internal/vm"
)

var startCmd = &cobra.Command{
	Use:   "start <name>",
	Short: "Start a VM",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(args[0])
		if err != nil {
			return err
		}
		if err := vm.StartVM(libvirt.NewVirsh(), cfg); err != nil {
			return err
		}
		fmt.Printf("VM %q started.\n", cfg.Name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
