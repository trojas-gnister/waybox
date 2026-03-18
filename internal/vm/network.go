package vm

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/trojas-gnister/waybox/internal/libvirt"
)

// RemoveNetworkInterface detaches the network interface from an airgapped VM.
func RemoveNetworkInterface(virsh libvirt.Virsh, vmName string) error {
	slog.Debug("fetching VM XML to find network interface")

	xml, err := virsh.DumpXML(vmName)
	if err != nil {
		return fmt.Errorf("getting VM XML: %w", err)
	}

	// Find MAC address
	mac := parseMAC(xml)
	if mac == "" {
		slog.Warn("no network interface found to remove (already removed?)")
		return nil
	}
	slog.Info("found network interface", "mac", mac)

	// Create detach XML
	detachXML, err := libvirt.NetworkInterfaceXML(mac, "default")
	if err != nil {
		return err
	}

	xmlPath := filepath.Join(os.TempDir(), vmName+"-detach-nic.xml")
	if err := os.WriteFile(xmlPath, []byte(detachXML), 0644); err != nil {
		return err
	}
	defer os.Remove(xmlPath)

	isRunning := virsh.IsVMRunning(vmName)

	var detachErr error
	if isRunning {
		detachErr = virsh.DetachDevice(vmName, xmlPath, true, true)
	} else {
		detachErr = virsh.DetachDevice(vmName, xmlPath, false, true)
	}

	if detachErr != nil {
		slog.Warn("failed to remove network interface", "error", detachErr)
		return nil // non-fatal
	}

	slog.Info("network interface removed, VM is now airgapped", "mac", mac)
	return nil
}

func parseMAC(xml string) string {
	for _, line := range strings.Split(xml, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "<mac address=") {
			continue
		}
		// Try single quotes: address='xx:xx:xx:xx:xx:xx'
		if idx := strings.Index(trimmed, "address='"); idx >= 0 {
			rest := trimmed[idx+9:]
			if end := strings.IndexByte(rest, '\''); end >= 0 {
				return rest[:end]
			}
		}
		// Try double quotes
		if idx := strings.Index(trimmed, `address="`); idx >= 0 {
			rest := trimmed[idx+9:]
			if end := strings.IndexByte(rest, '"'); end >= 0 {
				return rest[:end]
			}
		}
	}
	return ""
}
