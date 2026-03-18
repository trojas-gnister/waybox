package waypipe

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/trojas-gnister/waybox/internal/audio"
)

// Ports holds the vsock port assignments for a session.
type Ports struct {
	Waypipe uint32
	Audio   uint32
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

// StartSession establishes a waypipe vsock connection and audio tunnel to a VM.
//
// Host-side commands:
//
//	waypipe --no-gpu client --vsock --socket {cid}:{waypipePort} -- {app}
//	socat VSOCK-LISTEN:{audioPort},fork UNIX:/run/user/1000/pulse/native
//
// The guest is expected to already be running:
//
//	waypipe server --vsock --socket 0:{waypipePort}  (systemd service)
//	socat UNIX-LISTEN:/tmp/pulse-bridge,fork VSOCK-CONNECT:2:{audioPort}  (systemd service)
func StartSession(ctx context.Context, vmName, app string, cid uint32, ports Ports) (*Session, error) {
	ctx, cancel := context.WithCancel(ctx)

	// Start audio tunnel first
	tun, err := audio.StartTunnel(ctx, cid, ports.Audio)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("starting audio tunnel: %w", err)
	}

	// Start waypipe client
	socket := fmt.Sprintf("%d:%d", cid, ports.Waypipe)
	cmd := exec.CommandContext(ctx, "waypipe", "--no-gpu", "client", "--vsock", "--socket", socket, "--", app)
	cmd.Stdout = nil
	cmd.Stderr = nil

	slog.Info("launching waypipe session", "vm", vmName, "app", app, "cid", cid, "socket", socket)
	if err := cmd.Start(); err != nil {
		tun.Stop()
		cancel()
		return nil, fmt.Errorf("starting waypipe client: %w", err)
	}

	return &Session{
		VMName:     vmName,
		App:        app,
		CID:        cid,
		waypipeCmd: cmd,
		audioTun:   tun,
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

// Wait blocks until the waypipe client process exits.
func (s *Session) Wait() error {
	err := s.waypipeCmd.Wait()
	s.audioTun.Stop()
	s.cancel()
	if err != nil {
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
