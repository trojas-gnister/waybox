package cmd

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var consoleCmd = &cobra.Command{
	Use:   "console <name>",
	Short: "Connect to VM serial console",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := exec.Command("sudo", "virsh", "-c", "qemu:///system", "console", args[0])
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

func init() {
	rootCmd.AddCommand(consoleCmd)
}
