package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/trojas-gnister/waybox/internal/config"
	"github.com/trojas-gnister/waybox/internal/libvirt"
	"github.com/trojas-gnister/waybox/internal/vm"
)

var usbAttachCmd = &cobra.Command{
	Use:   "usb-attach <name> <vendor:product>",
	Short: "Hot-attach a USB device to a running VM",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vmName := args[0]
		dev, err := vm.DetectUSBDevice(args[1])
		if err != nil {
			return err
		}
		if err := vm.AttachUSBDevice(libvirt.NewVirsh(), vmName, dev); err != nil {
			return err
		}
		fmt.Printf("USB device %s attached to %s.\n", dev.Description, vmName)
		return nil
	},
}

var usbDetachCmd = &cobra.Command{
	Use:   "usb-detach <name> <vendor:product>",
	Short: "Hot-detach a USB device from a running VM",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vmName := args[0]
		dev := &config.UsbDevice{
			VendorID:  args[1][:4],
			ProductID: args[1][5:],
		}
		if err := vm.DetachUSBDevice(libvirt.NewVirsh(), vmName, dev); err != nil {
			return err
		}
		fmt.Printf("USB device detached from %s.\n", vmName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(usbAttachCmd)
	rootCmd.AddCommand(usbDetachCmd)
}
