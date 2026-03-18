package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/trojas-gnister/waybox/internal/config"
	"github.com/trojas-gnister/waybox/internal/libvirt"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured VMs",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := config.ConfigDir()
		if err != nil {
			return err
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No VMs configured.")
				return nil
			}
			return err
		}

		virsh := libvirt.NewVirsh()
		found := false

		fmt.Printf("%-20s %-12s %-8s %-6s %-10s\n", "NAME", "STATE", "MEM", "VCPU", "NET")
		fmt.Println(strings.Repeat("-", 58))

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
				continue
			}
			if entry.Name() == config.PasswordFileName {
				continue
			}

			name := strings.TrimSuffix(entry.Name(), ".toml")
			cfg, err := config.Load(name)
			if err != nil {
				continue
			}

			state := "unknown"
			if s, ok := virsh.GetVMState(name); ok {
				state = s
			}

			netMode := cfg.NetworkMode.Mode
			if netMode == "Bridge" {
				netMode = "br:" + cfg.NetworkMode.BridgeName
			}

			fmt.Printf("%-20s %-12s %-8d %-6d %-10s\n",
				cfg.Name, state, cfg.MemoryMB, cfg.VCPUs, strings.ToLower(netMode))
			found = true
		}

		if !found {
			fmt.Println("No VMs configured.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
