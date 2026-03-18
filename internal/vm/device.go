package vm

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/trojas-gnister/waybox/internal/config"
)

// GpuVendor classifies GPU manufacturers.
type GpuVendor string

const (
	GpuAMD     GpuVendor = "amd"
	GpuIntel   GpuVendor = "intel"
	GpuNVIDIA  GpuVendor = "nvidia"
	GpuUnknown GpuVendor = "unknown"
)

// GpuRenderNode represents a detected GPU render node on the host.
type GpuRenderNode struct {
	Vendor     GpuVendor
	PCISlot    string
	RenderNode string
	ByPath     string
}

// DetectGPURenderNodes scans /sys/class/drm/renderD* for GPU render nodes.
func DetectGPURenderNodes() []GpuRenderNode {
	var nodes []GpuRenderNode

	entries, err := os.ReadDir("/sys/class/drm")
	if err != nil {
		slog.Warn("cannot read /sys/class/drm", "error", err)
		return nodes
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "renderD") {
			continue
		}

		renderNode := filepath.Join("/dev/dri", name)

		// Resolve PCI device via 'device' symlink
		deviceLink := filepath.Join("/sys/class/drm", name, "device")
		target, err := os.Readlink(deviceLink)
		if err != nil {
			slog.Debug("cannot resolve device link", "node", name, "error", err)
			continue
		}
		pciSlot := filepath.Base(target)
		if pciSlot == "" {
			continue
		}

		// Read vendor ID from sysfs
		vendorPath := filepath.Join(deviceLink, "vendor")
		vendorData, err := os.ReadFile(vendorPath)
		if err != nil {
			slog.Debug("cannot read vendor", "pci", pciSlot, "error", err)
			continue
		}
		vendorHex := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(string(vendorData)), "0x"))

		var vendor GpuVendor
		switch vendorHex {
		case config.GPUVendorAMD:
			vendor = GpuAMD
		case config.GPUVendorIntel:
			vendor = GpuIntel
		case config.GPUVendorNVIDIA:
			vendor = GpuNVIDIA
		default:
			vendor = GpuUnknown
		}

		byPath := fmt.Sprintf("/dev/dri/by-path/pci-%s-render", pciSlot)

		slog.Debug("found GPU render node", "node", renderNode, "vendor", vendor, "pci", pciSlot)
		nodes = append(nodes, GpuRenderNode{
			Vendor:     vendor,
			PCISlot:    pciSlot,
			RenderNode: renderNode,
			ByPath:     byPath,
		})
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].PCISlot < nodes[j].PCISlot
	})
	return nodes
}

// SelectGPUForVenus picks the best GPU for Venus Vulkan.
// Priority: AMD > Intel > NVIDIA (experimental, driver 590+).
func SelectGPUForVenus(nodes []GpuRenderNode) *GpuRenderNode {
	for i := range nodes {
		if nodes[i].Vendor == GpuAMD {
			return &nodes[i]
		}
	}
	for i := range nodes {
		if nodes[i].Vendor == GpuIntel {
			return &nodes[i]
		}
	}
	// NVIDIA experimental (driver 590+)
	for i := range nodes {
		if nodes[i].Vendor == GpuNVIDIA {
			if checkNVIDIADriverVersion() >= 590 {
				slog.Warn("using NVIDIA GPU for Venus (experimental, driver 590+)")
				return &nodes[i]
			}
		}
	}
	return nil
}

// GetVulkanICDPath returns the Vulkan ICD file path for a GPU vendor.
func GetVulkanICDPath(vendor GpuVendor) string {
	var filename string
	switch vendor {
	case GpuAMD:
		filename = "radeon_icd.x86_64.json"
	case GpuIntel:
		filename = "intel_icd.x86_64.json"
	case GpuNVIDIA:
		filename = "nvidia_icd.json"
	default:
		return ""
	}

	path := filepath.Join(config.VulkanICDDir, filename)
	if _, err := os.Stat(path); err != nil {
		slog.Warn("Vulkan ICD file not found", "path", path)
		return ""
	}
	return path
}

// DetectUSBDevice validates and looks up a USB device by vendor:product address.
func DetectUSBDevice(address string) (*config.UsbDevice, error) {
	parts := strings.SplitN(address, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid USB address format %q (expected vendor:product)", address)
	}
	vendorID := strings.ToLower(parts[0])
	productID := strings.ToLower(parts[1])

	if len(vendorID) != 4 || len(productID) != 4 {
		return nil, fmt.Errorf("invalid USB IDs %q (expected 4 hex digits each)", address)
	}

	cmd := exec.Command("lsusb", "-d", address)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("USB device %s not found", address)
	}

	line := strings.SplitN(string(output), "\n", 2)[0]
	if line == "" {
		return nil, fmt.Errorf("USB device %s not found", address)
	}

	bus, device := parseUSBBusDevice(line)
	description := parseUSBDescription(line)

	slog.Info("found USB device", "description", description, "bus", bus, "device", device)
	return &config.UsbDevice{
		VendorID:    vendorID,
		ProductID:   productID,
		Description: description,
		Bus:         bus,
		Device:      device,
	}, nil
}

func parseUSBBusDevice(line string) (*uint8, *uint8) {
	fields := strings.Fields(line)
	var bus, device *uint8
	for i, f := range fields {
		if f == "Bus" && i+1 < len(fields) {
			if v := parseUint8(fields[i+1]); v != nil {
				bus = v
			}
		}
		if f == "Device" && i+1 < len(fields) {
			s := strings.TrimSuffix(fields[i+1], ":")
			if v := parseUint8(s); v != nil {
				device = v
			}
		}
	}
	return bus, device
}

func parseUSBDescription(line string) string {
	idx := strings.Index(line, "ID ")
	if idx < 0 {
		return "Unknown USB device"
	}
	after := line[idx+3:]
	space := strings.IndexByte(after, ' ')
	if space < 0 {
		return "Unknown USB device"
	}
	return strings.TrimSpace(after[space+1:])
}

func parseUint8(s string) *uint8 {
	var v uint8
	for _, c := range s {
		if c < '0' || c > '9' {
			return nil
		}
		v = v*10 + uint8(c-'0')
	}
	return &v
}

func checkNVIDIADriverVersion() int {
	output, err := exec.Command("nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader").Output()
	if err != nil {
		return 0
	}
	version := strings.TrimSpace(string(output))
	// Parse major version from "590.48.01"
	parts := strings.SplitN(version, ".", 2)
	if len(parts) == 0 {
		return 0
	}
	var major int
	for _, c := range parts[0] {
		if c >= '0' && c <= '9' {
			major = major*10 + int(c-'0')
		}
	}
	return major
}
