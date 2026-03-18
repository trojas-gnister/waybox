package vm

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/trojas-gnister/waybox/internal/config"
	"github.com/trojas-gnister/waybox/internal/libvirt"
)

// StartVM starts a VM, waits for boot, and hot-plugs USB devices if configured.
func StartVM(virsh libvirt.Virsh, cfg *config.AppVMConfig) error {
	slog.Info("starting VM", "name", cfg.Name)

	if err := virsh.Start(cfg.Name); err != nil {
		return err
	}

	time.Sleep(time.Duration(config.VMBootWaitSecs) * time.Second)

	// Hot-attach USB if in hotplug mode
	if cfg.USBHotplug && len(cfg.USBDevices) > 0 {
		if err := AttachUSBHotplug(virsh, cfg); err != nil {
			return err
		}
	}

	if cfg.Headless {
		slog.Info("headless VM started — connect with: virsh console", "name", cfg.Name)
	} else {
		slog.Info("VM started — use 'waybox launch' to run apps", "name", cfg.Name)
	}
	return nil
}

// StopVM gracefully shuts down a VM, detaching USB devices first if needed.
func StopVM(virsh libvirt.Virsh, cfg *config.AppVMConfig) error {
	slog.Info("stopping VM", "name", cfg.Name)

	if cfg.USBHotplug && len(cfg.USBDevices) > 0 {
		DetachUSBHotplug(virsh, cfg)
	}

	return virsh.Shutdown(cfg.Name)
}

// DestroyVM force-stops and undefines a VM, cleaning up all resources.
func DestroyVM(virsh libvirt.Virsh, cfg *config.AppVMConfig) error {
	slog.Info("destroying VM", "name", cfg.Name)

	// Get IP before destroying (for SSH known_hosts cleanup)
	vmIP, _ := virsh.GetVMIP(cfg.Name)

	if !virsh.DomainExists(cfg.Name) {
		slog.Debug("VM not found in libvirt", "name", cfg.Name)
	} else {
		// Force stop
		virsh.DestroyUnchecked(cfg.Name)
		time.Sleep(3 * time.Second)

		// Undefine with storage
		if err := virsh.Undefine(cfg.Name, true); err != nil {
			slog.Debug("undefine with storage failed, trying without", "error", err)
			if err := virsh.Undefine(cfg.Name, false); err != nil {
				slog.Error("failed to undefine VM", "error", err)
			}
		}
	}

	// Remove disk manually if still present
	diskPath := filepath.Join(cfg.VMDir, cfg.Name+".qcow2")
	if _, err := os.Stat(diskPath); err == nil {
		if err := os.Remove(diskPath); err != nil {
			slog.Debug("removing disk with sudo", "error", err)
			exec.Command("sudo", "rm", "-f", diskPath).Run()
		}
	}

	// Verify removal
	if virsh.DomainExists(cfg.Name) {
		slog.Warn("VM still in virsh list — may need manual cleanup", "name", cfg.Name)
	}

	// Clean SSH known_hosts
	if vmIP != "" {
		exec.Command("ssh-keygen", "-R", vmIP).Run()
	}

	// Remove config file
	configPath, err := config.ConfigPath(cfg.Name)
	if err == nil {
		os.Remove(configPath)
	}

	// Remove desktop files
	removeDesktopFiles(cfg.Name)

	slog.Info("VM destroyed", "name", cfg.Name)
	return nil
}

func removeDesktopFiles(vmName string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	appsDir := filepath.Join(home, ".local", "share", "applications")
	entries, err := os.ReadDir(appsDir)
	if err != nil {
		return
	}

	prefix := fmt.Sprintf("vm-%s-", vmName)
	for _, entry := range entries {
		if !entry.IsDir() && len(entry.Name()) > len(prefix) && entry.Name()[:len(prefix)] == prefix {
			path := filepath.Join(appsDir, entry.Name())
			os.Remove(path)
			slog.Debug("removed desktop file", "path", path)
		}
	}
}
