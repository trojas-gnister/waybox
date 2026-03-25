package waypipe

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"time"

	"github.com/trojas-gnister/waybox/internal/audio"
)

// Ports holds the vsock port assignments for a session.
type Ports struct {
	Waypipe  uint32
	Audio    uint32
	Launcher uint32
}

// Session manages a waypipe display session and audio tunnel over vsock.
type Session struct {
	VMName     string
	App        string
	CID        uint32
	waypipeCmd *exec.Cmd
	audioTun   *audio.Tunnel
	ctx        context.Context
	cancel     context.CancelFunc
}

// StartSession establishes a waypipe session with a VM.
//
// Architecture:
//
//	Host:  waypipe --no-gpu client --vsock --socket {cid}:{waypipePort}  (LISTENS)
//	Guest: waypipe --vsock --socket 2:{waypipePort} server -- {app}      (CONNECTS)
//
// The guest runs a launcher daemon on LauncherPort that accepts app commands
// over vsock and spawns waypipe server for each request.
func StartSession(ctx context.Context, vmName, app string, cid uint32, ports Ports) (*Session, error) {
	ctx, cancel := context.WithCancel(ctx)

	// Start audio tunnel (host listens on vsock for guest audio)
	tun, err := audio.StartTunnel(ctx, cid, ports.Audio)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("starting audio tunnel: %w", err)
	}

	// Start waypipe client on host (LISTENS for guest server connection)
	// Options must come BEFORE the mode subcommand
	socket := fmt.Sprintf("%d:%d", cid, ports.Waypipe)
	waypipeCmd := exec.CommandContext(ctx, "waypipe", "--no-gpu", "--vsock", "--socket", socket, "client")
	waypipeCmd.Stdout = nil
	waypipeCmd.Stderr = nil

	slog.Info("starting waypipe client (listening)", "socket", socket)
	if err := waypipeCmd.Start(); err != nil {
		tun.Stop()
		cancel()
		return nil, fmt.Errorf("starting waypipe client: %w", err)
	}

	// Give the client a moment to start listening
	time.Sleep(500 * time.Millisecond)

	// Tell the guest launcher to run the app via waypipe server
	slog.Info("sending launch command to guest", "app", app, "launcher_port", ports.Launcher)
	if err := sendLaunchCommand(cid, ports.Launcher, app); err != nil {
		waypipeCmd.Process.Kill()
		tun.Stop()
		cancel()
		return nil, fmt.Errorf("sending launch command to guest: %w", err)
	}

	return &Session{
		VMName:     vmName,
		App:        app,
		CID:        cid,
		waypipeCmd: waypipeCmd,
		audioTun:   tun,
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

// sendLaunchCommand connects to the guest's launcher daemon over vsock
// and sends the app command to run.
func sendLaunchCommand(cid uint32, launcherPort uint32, app string) error {
	// Use socat to send the app command to the guest launcher
	cmd := exec.Command("socat", "-",
		fmt.Sprintf("VSOCK-CONNECT:%d:%d", cid, launcherPort))
	cmd.Stdin = nil

	// Create a pipe to write the command
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("connecting to guest launcher: %w", err)
	}

	// Send the app command
	fmt.Fprintf(stdin, "%s\n", app)
	stdin.Close()

	// Don't wait for socat to finish — it will stay open while the app runs
	go cmd.Wait()

	return nil
}

// Wait blocks until the waypipe client process exits.
func (s *Session) Wait() error {
	err := s.waypipeCmd.Wait()
	s.audioTun.Stop()
	s.cancel()
	if err != nil {
		// Context cancelled (Ctrl+C) is normal
		if s.ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("waypipe exited: %w", err)
	}
	return nil
}

// Stop terminates the session gracefully.
func (s *Session) Stop() error {
	s.cancel()
	s.audioTun.Stop()
	if s.waypipeCmd.Process != nil {
		return s.waypipeCmd.Process.Kill()
	}
	return nil
}

// WaitForLauncher polls the guest's launcher daemon until it's ready.
func WaitForLauncher(cid uint32, launcherPort uint32, timeout time.Duration) error {
	slog.Debug("waiting for guest launcher", "cid", cid, "port", launcherPort)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Try to connect briefly with socat
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		cmd := exec.CommandContext(ctx, "socat", "-T1",
			fmt.Sprintf("VSOCK-CONNECT:%d:%d", cid, launcherPort),
			"/dev/null")
		err := cmd.Run()
		cancel()

		if err == nil {
			slog.Debug("guest launcher ready")
			return nil
		}

		// Also check if vsock is reachable at all
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for guest launcher (CID %d port %d) — VM may still be booting", cid, launcherPort)
}

