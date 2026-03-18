package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/trojas-gnister/waybox/internal/config"
	"github.com/trojas-gnister/waybox/internal/libvirt"
	"github.com/trojas-gnister/waybox/internal/vm"
)

var destroyCmd = &cobra.Command{
	Use:   "destroy <name>",
	Short: "Destroy a VM and delete its config",
	Args:  cobra.ExactArgs(1),
	RunE:  runDestroy,
}

func init() {
	destroyCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	rootCmd.AddCommand(destroyCmd)
}

func runDestroy(cmd *cobra.Command, args []string) error {
	name := args[0]
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	cfg, err := config.Load(name)
	if err != nil {
		// Even if config doesn't exist, try to destroy the libvirt domain
		cfg = &config.AppVMConfig{
			Name:  name,
			VMDir: config.DefaultVMDir,
		}
	}

	if !skipConfirm {
		fmt.Printf("Destroy VM %q? This will delete the VM and all its data.\n", name)
		fmt.Print("Proceed? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(answer)) != "y" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	if err := vm.DestroyVM(libvirt.NewVirsh(), cfg); err != nil {
		return err
	}
	fmt.Printf("VM %q destroyed.\n", name)
	return nil
}
