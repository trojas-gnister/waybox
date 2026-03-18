package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/trojas-gnister/waybox/internal/config"
	"github.com/trojas-gnister/waybox/internal/vm"
	"github.com/trojas-gnister/waybox/internal/waypipe"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new application VM",
	RunE:  runCreate,
}

func init() {
	f := createCmd.Flags()
	f.String("name", "", "VM name (required)")
	f.StringSlice("system", nil, "System packages to install")
	f.StringSlice("flatpak", nil, "Flatpak packages to install")
	f.Uint64("memory", config.DefaultMemoryMB, "Memory in MB")
	f.Uint32("vcpus", uint32(config.DefaultVCPUs), "Number of vCPUs")
	f.Uint64("disk", config.DefaultDiskSizeGB, "Disk size in GB")
	f.Bool("headless", false, "Create a headless (no GUI) VM")
	f.StringSlice("usb", nil, "USB devices to passthrough (vendor:product)")
	f.Bool("usb-hotplug", false, "Use hot-plug mode for USB devices")
	f.StringSlice("share", nil, "Shared folders (host:guest:tag)")
	f.Bool("share-readonly", false, "Make shared folders read-only")
	f.String("network-bridge", "", "Bridge name for bridged networking")
	f.Bool("no-network", false, "Create an airgapped VM (no network)")
	f.Bool("grant-device-access", false, "Grant flatpak apps device access")
	f.BoolP("yes", "y", false, "Skip confirmation prompt")

	createCmd.MarkFlagRequired("name")
	rootCmd.AddCommand(createCmd)
}

func runCreate(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	systemPkgs, _ := cmd.Flags().GetStringSlice("system")
	flatpakPkgs, _ := cmd.Flags().GetStringSlice("flatpak")
	memory, _ := cmd.Flags().GetUint64("memory")
	vcpus, _ := cmd.Flags().GetUint32("vcpus")
	disk, _ := cmd.Flags().GetUint64("disk")
	headless, _ := cmd.Flags().GetBool("headless")
	usbAddrs, _ := cmd.Flags().GetStringSlice("usb")
	usbHotplug, _ := cmd.Flags().GetBool("usb-hotplug")
	shares, _ := cmd.Flags().GetStringSlice("share")
	shareRO, _ := cmd.Flags().GetBool("share-readonly")
	bridge, _ := cmd.Flags().GetString("network-bridge")
	noNetwork, _ := cmd.Flags().GetBool("no-network")
	grantDev, _ := cmd.Flags().GetBool("grant-device-access")
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	if noNetwork && bridge != "" {
		return fmt.Errorf("--no-network and --network-bridge are mutually exclusive")
	}

	// Build config
	b := config.NewBuilder(name).
		Memory(memory).
		VCPUs(vcpus).
		DiskSize(disk).
		Headless(headless).
		USBHotplug(usbHotplug).
		GrantDeviceAccess(grantDev)

	if len(systemPkgs) > 0 {
		b.SystemPackages(systemPkgs)
	}
	if len(flatpakPkgs) > 0 {
		b.FlatpakPackages(flatpakPkgs)
	}

	// Parse USB devices
	for _, addr := range usbAddrs {
		dev, err := vm.DetectUSBDevice(addr)
		if err != nil {
			return fmt.Errorf("USB device %s: %w", addr, err)
		}
		b.AddUSBDevice(*dev)
	}

	// Parse shared folders
	for i, share := range shares {
		parts := strings.SplitN(share, ":", 3)
		if len(parts) != 3 {
			return fmt.Errorf("invalid share format %q (expected host:guest:tag)", share)
		}
		b.AddSharedFolder(config.SharedFolder{
			HostPath:  parts[0],
			GuestPath: parts[1],
			Tag:       parts[2],
			ReadOnly:  shareRO,
		})
		_ = i
	}

	// Network mode
	if noNetwork {
		b.NoNetwork()
	} else if bridge != "" {
		b.Bridge(bridge)
	}

	cfg, err := b.Build()
	if err != nil {
		return err
	}

	// Confirmation
	if !skipConfirm {
		fmt.Printf("Create VM %q?\n", cfg.Name)
		fmt.Printf("  Memory: %d MB, vCPUs: %d, Disk: %d GB\n", cfg.MemoryMB, cfg.VCPUs, cfg.DiskSizeGB)
		fmt.Printf("  Packages: %v\n", cfg.SystemPackages)
		if len(cfg.FlatpakPackages) > 0 {
			fmt.Printf("  Flatpak: %v\n", cfg.FlatpakPackages)
		}
		fmt.Print("Proceed? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(answer)) != "y" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Provision
	p := vm.NewProvisioner(cfg)
	if err := p.ProvisionVM(); err != nil {
		return err
	}

	// Generate desktop files
	if err := waypipe.GenerateDesktopFiles(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: desktop file generation failed: %v\n", err)
	}

	fmt.Printf("VM %q created successfully.\n", cfg.Name)
	fmt.Printf("Password: %s\n", cfg.UserPassword)
	fmt.Printf("Start with: waybox start %s\n", cfg.Name)
	fmt.Printf("Launch app: waybox launch %s <app>\n", cfg.Name)
	return nil
}
