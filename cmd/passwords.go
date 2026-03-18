package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/trojas-gnister/waybox/internal/config"
)

var passwordsCmd = &cobra.Command{
	Use:   "passwords",
	Short: "Show stored VM passwords",
	RunE: func(cmd *cobra.Command, args []string) error {
		passwords, err := config.LoadPasswords()
		if err != nil {
			return err
		}

		if len(passwords.VMs) == 0 {
			fmt.Println("No passwords stored.")
			return nil
		}

		// Sort by name
		names := make([]string, 0, len(passwords.VMs))
		for name := range passwords.VMs {
			names = append(names, name)
		}
		sort.Strings(names)

		fmt.Printf("%-20s %s\n", "VM", "PASSWORD")
		for _, name := range names {
			fmt.Printf("%-20s %s\n", name, passwords.VMs[name])
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(passwordsCmd)
}
