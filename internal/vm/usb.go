package vm

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/trojas-gnister/waybox/internal/config"
	"github.com/trojas-gnister/waybox/internal/libvirt"
)

// SetupUSBPermanent attaches all configured USB devices to the VM config (offline).
func SetupUSBPermanent(virsh libvirt.Virsh, cfg *config.AppVMConfig) error {
	slog.Info("setting up USB passthrough (permanent mode)")

	for _, dev := range cfg.USBDevices {
		slog.Debug("attaching USB device", "desc", dev.Description, "id", dev.VendorID+":"+dev.ProductID)

		xmlStr, err := libvirt.USBHostdevXML(dev.VendorID, dev.ProductID)
		if err != nil {
			return err
		}

		xmlPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s-usb-%s-%s.xml", cfg.Name, dev.VendorID, dev.ProductID))
		if err := os.WriteFile(xmlPath, []byte(xmlStr), 0644); err != nil {
			return err
		}

		err = virsh.AttachDevice(cfg.Name, xmlPath, false, true)
		os.Remove(xmlPath)

		if err != nil {
			slog.Warn("failed to attach USB device", "id", dev.VendorID+":"+dev.ProductID, "error", err)
		} else {
			slog.Info("USB device attached permanently", "desc", dev.Description)
		}
	}
	return nil
}

// AttachUSBHotplug hot-attaches all configured USB devices to a running VM.
func AttachUSBHotplug(virsh libvirt.Virsh, cfg *config.AppVMConfig) error {
	slog.Info("hot-attaching USB devices")
	for _, dev := range cfg.USBDevices {
		if err := AttachUSBDevice(virsh, cfg.Name, &dev); err != nil {
			return err
		}
	}
	return nil
}

// DetachUSBHotplug hot-detaches all configured USB devices from a running VM.
func DetachUSBHotplug(virsh libvirt.Virsh, cfg *config.AppVMConfig) {
	slog.Info("hot-detaching USB devices")
	for _, dev := range cfg.USBDevices {
		_ = DetachUSBDevice(virsh, cfg.Name, &dev)
	}
}

// AttachUSBDevice hot-attaches a single USB device to a running VM.
func AttachUSBDevice(virsh libvirt.Virsh, vmName string, dev *config.UsbDevice) error {
	slog.Debug("attaching USB", "desc", dev.Description, "id", dev.VendorID+":"+dev.ProductID)

	xmlStr, err := libvirt.USBHostdevXML(dev.VendorID, dev.ProductID)
	if err != nil {
		return err
	}

	xmlPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s-usb-%s-%s.xml", vmName, dev.VendorID, dev.ProductID))
	if err := os.WriteFile(xmlPath, []byte(xmlStr), 0644); err != nil {
		return err
	}
	defer os.Remove(xmlPath)

	if err := virsh.AttachDevice(vmName, xmlPath, true, false); err != nil {
		return fmt.Errorf("attach USB %s:%s: %w", dev.VendorID, dev.ProductID, err)
	}
	slog.Info("USB device attached", "desc", dev.Description)
	return nil
}

// DetachUSBDevice hot-detaches a single USB device from a running VM.
func DetachUSBDevice(virsh libvirt.Virsh, vmName string, dev *config.UsbDevice) error {
	slog.Debug("detaching USB", "desc", dev.Description, "id", dev.VendorID+":"+dev.ProductID)

	xmlStr, err := libvirt.USBHostdevXML(dev.VendorID, dev.ProductID)
	if err != nil {
		return err
	}

	xmlPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s-usb-%s-%s.xml", vmName, dev.VendorID, dev.ProductID))
	if err := os.WriteFile(xmlPath, []byte(xmlStr), 0644); err != nil {
		return err
	}
	defer os.Remove(xmlPath)

	if err := virsh.DetachDevice(vmName, xmlPath, true, false); err != nil {
		return fmt.Errorf("detach USB %s:%s: %w", dev.VendorID, dev.ProductID, err)
	}
	slog.Info("USB device detached", "desc", dev.Description)
	return nil
}
