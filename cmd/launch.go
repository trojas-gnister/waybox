package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
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

	// Wait for vsock connectivity
	if err := waitForVsock(cid, cfg.WaypipePort); err != nil {
		return fmt.Errorf("waiting for vsock: %w", err)
	}

	// Set up signal handling for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Start waypipe session + audio tunnel
	ports := waypipe.Ports{
		Waypipe: cfg.WaypipePort,
		Audio:   cfg.AudioPort,
	}

	session, err := waypipe.StartSession(ctx, vmName, app, cid, ports)
	if err != nil {
		return err
	}

	fmt.Printf("Launched %s in VM %s (CID %d). Press Ctrl+C to stop.\n", app, vmName, cid)

	// Block until app exits or user interrupts
	if err := session.Wait(); err != nil {
		// Context cancelled (Ctrl+C) is not an error
		if ctx.Err() != nil {
			fmt.Println("\nSession ended.")
			return nil
		}
		return err
	}

	fmt.Println("App exited.")
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

// waitForVsock polls vsock connectivity using socat.
func waitForVsock(cid uint32, port uint32) error {
	slog.Debug("waiting for vsock connectivity", "cid", cid, "port", port)

	for i := 0; i < 30; i++ {
		// Try to connect briefly with socat
		cmd := exec.Command("socat", "-T1",
			fmt.Sprintf("VSOCK-CONNECT:%d:%d", cid, port),
			"STDOUT")
		if err := cmd.Run(); err == nil {
			slog.Debug("vsock connected")
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for vsock CID %d port %d", cid, port)
}
