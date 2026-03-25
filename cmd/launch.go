package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"
	"github.com/trojas-gnister/waybox/internal/config"
	"github.com/trojas-gnister/waybox/internal/libvirt"
	"github.com/trojas-gnister/waybox/internal/waypipe"
)

var launchCmd = &cobra.Command{
	Use:   "launch <vm> <app>",
	Short: "Launch an app from a VM via waypipe",
	Long:  "Starts the VM if needed, establishes waypipe + audio over vsock, and runs the app.",
	Args:  cobra.ExactArgs(2),
	RunE:  runLaunch,
}

func init() {
	rootCmd.AddCommand(launchCmd)
}

func runLaunch(cmd *cobra.Command, args []string) error {
	vmName := args[0]
	app := args[1]

	cfg, err := config.Load(vmName)
	if err != nil {
		return err
	}

	virsh := libvirt.NewVirsh()

	// Start VM if not running
	if !virsh.IsVMRunning(vmName) {
		slog.Info("starting VM", "name", vmName)
		if err := virsh.Start(vmName); err != nil {
			return fmt.Errorf("starting VM: %w", err)
		}
		time.Sleep(time.Duration(config.VMBootWaitSecs) * time.Second)
	}

	// Get vsock CID from libvirt XML
	cid, err := getVsockCID(virsh, vmName)
	if err != nil {
		return err
	}
	slog.Info("vsock CID", "cid", cid)

	// Wait for guest launcher daemon to be ready
	fmt.Printf("Waiting for VM %s to be ready...\n", vmName)
	if err := waypipe.WaitForLauncher(cid, config.DefaultLauncherPort, 120*time.Second); err != nil {
		return err
	}

	// Set up signal handling for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Start waypipe session + audio tunnel
	ports := waypipe.Ports{
		Waypipe:  cfg.WaypipePort,
		Audio:    cfg.AudioPort,
		Launcher: config.DefaultLauncherPort,
	}

	session, err := waypipe.StartSession(ctx, vmName, app, cid, ports)
	if err != nil {
		return err
	}

	fmt.Printf("Launched %s in VM %s. Press Ctrl+C to stop.\n", app, vmName)

	// Block until app exits or user interrupts
	if err := session.Wait(); err != nil {
		return err
	}

	fmt.Println("Session ended.")
	return nil
}

func getVsockCID(virsh libvirt.Virsh, vmName string) (uint32, error) {
	xml, err := virsh.DumpXML(vmName)
	if err != nil {
		return 0, fmt.Errorf("getting VM XML: %w", err)
	}

	cid, ok := libvirt.ParseVsockCID(xml)
	if ok {
		return cid, nil
	}

	return 0, fmt.Errorf("vsock CID not found in VM XML — is vsock enabled?")
}
