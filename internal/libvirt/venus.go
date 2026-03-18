package libvirt

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// ReplaceXMLBlock replaces an XML block (inclusive of start/end tags) with a replacement string.
func ReplaceXMLBlock(xml, startTag, endTag, replacement string) string {
	var result strings.Builder
	result.Grow(len(xml))
	skipping := false
	replaced := false

	for _, line := range strings.Split(xml, "\n") {
		trimmed := strings.TrimSpace(line)
		if !skipping && strings.HasPrefix(trimmed, startTag) {
			skipping = true
		}
		if skipping {
			if strings.HasPrefix(trimmed, endTag) {
				skipping = false
				if !replaced {
					result.WriteString(replacement)
					result.WriteByte('\n')
					replaced = true
				}
			}
			continue
		}
		result.WriteString(line)
		result.WriteByte('\n')
	}
	return result.String()
}

// EnableVenusVulkan post-processes a VM's libvirt XML to enable Venus Vulkan.
//
// virt-install doesn't support blob=on and venus=on flags, so we modify the
// XML after creation: add qemu namespace, replace <video> with type='none',
// and inject QEMU commandline args for virtio-vga-gl with Venus.
func EnableVenusVulkan(virsh Virsh, vmName string, memoryMB uint64, gpuByPath string, icdPath string) error {
	slog.Info("enabling Venus Vulkan", "vm", vmName)

	xml, err := virsh.DumpXML(vmName)
	if err != nil {
		return fmt.Errorf("dumping VM XML: %w", err)
	}

	// Add qemu namespace to domain element
	xml = strings.Replace(xml,
		"<domain type='kvm'>",
		"<domain type='kvm' xmlns:qemu='http://libvirt.org/schemas/domain/qemu/1.0'>",
		1)

	// Replace libvirt-managed <video> with type='none'
	xml = ReplaceXMLBlock(xml, "<video>", "</video>",
		"    <video>\n      <model type='none'/>\n    </video>")

	// Memory in KiB for QEMU args
	memKB := memoryMB * 1024
	memSize := fmt.Sprintf("%dK", memKB)
	deviceArg := fmt.Sprintf("virtio-vga-gl,hostmem=%s,blob=true,venus=true", memSize)
	memfdArg := fmt.Sprintf("memory-backend-memfd,id=mem1,size=%s", memSize)

	// Build ICD env line if available
	icdEnv := ""
	if icdPath != "" {
		slog.Info("setting VK_ICD_FILENAMES", "path", icdPath)
		icdEnv = fmt.Sprintf("\n    <qemu:env name='VK_ICD_FILENAMES' value='%s'/>", icdPath)
	}

	qemuBlock := fmt.Sprintf(`  <qemu:commandline>
    <qemu:arg value='-device'/>
    <qemu:arg value='%s'/>
    <qemu:arg value='-object'/>
    <qemu:arg value='%s'/>
    <qemu:arg value='-machine'/>
    <qemu:arg value='memory-backend=mem1'/>
    <qemu:arg value='-vga'/>
    <qemu:arg value='none'/>%s
  </qemu:commandline>
</domain>`, deviceArg, memfdArg, icdEnv)

	xml = strings.Replace(xml, "</domain>", qemuBlock, 1)

	// Write modified XML to temp file
	tmpFile, err := os.CreateTemp("", "waybox-venus-*.xml")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(xml); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing XML: %w", err)
	}
	tmpFile.Close()

	// Redefine the VM
	if err := virsh.Undefine(vmName, false); err != nil {
		return fmt.Errorf("undefining VM: %w", err)
	}
	if err := virsh.Define(tmpFile.Name()); err != nil {
		return fmt.Errorf("defining VM with Venus XML: %w", err)
	}

	slog.Info("Venus Vulkan enabled (blob=on, venus=on)")
	return nil
}
