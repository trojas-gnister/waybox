package audio

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
)

// Tunnel manages a socat vsock audio relay between host and guest.
//
// Host side: socat VSOCK-LISTEN:{port},fork UNIX:/run/user/1000/pulse/native
// Guest side (systemd service): socat UNIX-LISTEN:/tmp/pulse-bridge,fork VSOCK-CONNECT:2:{port}
type Tunnel struct {
	cmd    *exec.Cmd
	ctx    context.Context
	cancel context.CancelFunc
}

// StartTunnel starts a socat process on the host that listens on vsock
// and forwards to the local PulseAudio socket.
func StartTunnel(ctx context.Context, cid uint32, audioPort uint32) (*Tunnel, error) {
	ctx, cancel := context.WithCancel(ctx)

	socatArg := fmt.Sprintf("VSOCK-LISTEN:%d,reuseaddr,fork", audioPort)
	pulseSocket := "UNIX:/run/user/1000/pulse/native"

	cmd := exec.CommandContext(ctx, "socat", socatArg, pulseSocket)
	cmd.Stdout = nil
	cmd.Stderr = nil

	slog.Debug("starting audio tunnel", "vsock_port", audioPort, "pulse_socket", pulseSocket)
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("starting socat audio tunnel: %w", err)
	}

	return &Tunnel{
		cmd:    cmd,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Stop terminates the audio tunnel.
func (t *Tunnel) Stop() {
	t.cancel()
	if t.cmd.Process != nil {
		t.cmd.Process.Kill()
	}
	t.cmd.Wait()
	slog.Debug("audio tunnel stopped")
}
