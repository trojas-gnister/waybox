package vm

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/trojas-gnister/waybox/internal/config"
	"github.com/trojas-gnister/waybox/internal/libvirt"
	"github.com/trojas-gnister/waybox/internal/nixos"
)

// Provisioner orchestrates VM creation.
type Provisioner struct {
	Config *config.AppVMConfig
	Virsh  libvirt.Virsh
}

// NewProvisioner creates a provisioner for the given config.
func NewProvisioner(cfg *config.AppVMConfig) *Provisioner {
	return &Provisioner{
		Config: cfg,
		Virsh:  libvirt.NewVirsh(),
	}
}

// ProvisionVM executes the full VM creation workflow.
func (p *Provisioner) ProvisionVM() error {
	slog.Info("starting VM provisioning", "name", p.Config.Name)

	if err := p.checkPrerequisites(); err != nil {
		return err
	}

	// Generate NixOS configuration
	slog.Info("generating NixOS configuration")
	nixConfig, err := nixos.GenerateConfigurationNix(p.Config)
	if err != nil {
		return fmt.Errorf("generating NixOS config: %w", err)
	}

	// Build qcow2 image
	qcow2Path, err := nixos.BuildImage(nixConfig, p.Config.Name, p.Config.VMDir, p.Config.DiskSizeGB)
	if err != nil {
		return fmt.Errorf("building image: %w", err)
	}

	// Import VM via virt-install
	if err := p.importVM(qcow2Path); err != nil {
		// Clean up partial VM on failure
		p.cleanup()
		return fmt.Errorf("importing VM: %w", err)
	}

	// USB permanent passthrough
	if len(p.Config.USBDevices) > 0 && !p.Config.USBHotplug {
		if err := SetupUSBPermanent(p.Virsh, p.Config); err != nil {
			p.cleanup()
			return fmt.Errorf("USB passthrough: %w", err)
		}
	}

	// Remove network for airgapped VMs
	if p.Config.NetworkMode.Mode == "None" {
		slog.Info("removing network interface (airgapped mode)")
		if err := RemoveNetworkInterface(p.Virsh, p.Config.Name); err != nil {
			p.cleanup()
			return fmt.Errorf("removing network: %w", err)
		}
	}

	// Save config to disk
	if err := p.Config.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	// Save password
	passwords, err := config.LoadPasswords()
	if err != nil {
		slog.Warn("could not load password store", "error", err)
	} else {
		passwords.Add(p.Config.Name, p.Config.UserPassword)
		if err := passwords.Save(); err != nil {
			slog.Warn("could not save password", "error", err)
		}
	}

	slog.Info("VM provisioned successfully", "name", p.Config.Name)
	return nil
}

func (p *Provisioner) checkPrerequisites() error {
	slog.Info("checking prerequisites")

	required := []struct {
		cmd  string
		hint string
	}{
		{"virsh", "sudo pacman -S libvirt"},
		{"virt-install", "sudo pacman -S virt-install"},
		{"qemu-system-x86_64", "sudo pacman -S qemu-desktop"},
		{"qemu-img", "sudo pacman -S qemu-img"},
		{"nixos-generate", "nix-env -iA nixpkgs.nixos-generators"},
		{"dnsmasq", "sudo pacman -S dnsmasq"},
		{"waypipe", "sudo pacman -S waypipe"},
		{"socat", "sudo pacman -S socat"},
	}

	for _, r := range required {
		if _, err := exec.LookPath(r.cmd); err != nil {
			return fmt.Errorf("prerequisite missing: %s\n  Install with: %s", r.cmd, r.hint)
		}
		slog.Debug("found", "cmd", r.cmd)
	}

	// Check libvirtd is running
	output, err := exec.Command("systemctl", "is-active", "libvirtd").Output()
	if err != nil || strings.TrimSpace(string(output)) != "active" {
		slog.Info("starting libvirtd")
		if err := exec.Command("sudo", "systemctl", "start", "libvirtd").Run(); err != nil {
			return fmt.Errorf("failed to start libvirtd: %w\n  Enable with: sudo systemctl enable --now libvirtd", err)
		}
	}

	// Check default network is active (required for NAT mode)
	if p.Config.NetworkMode.Mode == "Nat" {
		if err := p.ensureDefaultNetwork(); err != nil {
			return err
		}
	}

	// Load vhost_vsock module
	if err := exec.Command("sudo", "modprobe", "vhost_vsock").Run(); err != nil {
		return fmt.Errorf("failed to load vhost_vsock kernel module\n  Check: find /lib/modules/$(uname -r) -name '*vsock*'\n  You may need to reboot after a kernel update")
	}

	// Check QEMU user groups for GPU access
	if p.Config.GraphicsBackend == config.GraphicsVirtioGPU && !p.Config.Headless {
		checkQemuUserGroups()
	}

	return nil
}

func (p *Provisioner) ensureDefaultNetwork() error {
	// Check if default network is active
	output, err := p.Virsh.RunSudoChecked("net-info", "default")
	if err != nil {
		// Network doesn't exist — try to define it
		slog.Info("defining default network")
		for _, path := range []string{
			"/usr/share/libvirt/networks/default.xml",
			"/etc/libvirt/qemu/networks/default.xml",
		} {
			if err := exec.Command("sudo", "virsh", "-c", "qemu:///system", "net-define", path).Run(); err == nil {
				break
			}
		}
	}

	// Check if active
	if output != "" && strings.Contains(output, "Active:") && !strings.Contains(output, "Active:          yes") {
		slog.Info("starting default network")
		if _, err := p.Virsh.RunSudoChecked("net-start", "default"); err != nil {
			return fmt.Errorf("failed to start default network\n  Try: sudo virsh net-start default\n  Or:  sudo virsh net-define /usr/share/libvirt/networks/default.xml && sudo virsh net-start default")
		}
		p.Virsh.RunSudoChecked("net-autostart", "default")
	}

	return nil
}

// cleanup removes a partially created VM after a failed provisioning attempt.
func (p *Provisioner) cleanup() {
	slog.Info("cleaning up failed provisioning", "name", p.Config.Name)
	p.Virsh.DestroyUnchecked(p.Config.Name)
	p.Virsh.Undefine(p.Config.Name, true)
	// Remove disk if it exists
	diskPath := fmt.Sprintf("%s/%s.qcow2", p.Config.VMDir, p.Config.Name)
	exec.Command("sudo", "rm", "-f", diskPath).Run()
}

func checkQemuUserGroups() {
	for _, user := range []string{"libvirt-qemu", "qemu"} {
		output, err := exec.Command("id", user).Output()
		if err != nil {
			continue
		}
		groups := string(output)
		var missing []string
		if !strings.Contains(groups, "render") {
			missing = append(missing, "render")
		}
		if !strings.Contains(groups, "video") {
			missing = append(missing, "video")
		}
		if len(missing) > 0 {
			slog.Warn("QEMU user missing groups for GPU access",
				"user", user,
				"missing", strings.Join(missing, ", "),
				"fix", fmt.Sprintf("sudo usermod -aG %s %s", strings.Join(missing, ","), user))
		}
		return
	}
}

func (p *Provisioner) importVM(qcow2Path string) error {
	slog.Info("importing NixOS image into libvirt")

	args := []string{
		"virt-install",
		"--name", p.Config.Name,
		"--memory", fmt.Sprintf("%d", p.Config.MemoryMB),
		"--vcpus", fmt.Sprintf("%d", p.Config.VCPUs),
		"--disk", fmt.Sprintf("path=%s,format=qcow2,bus=virtio", qcow2Path),
		"--import",
		"--os-variant", "nixos-unstable",
		"--noautoconsole",
	}

	// Graphics
	args = append(args, p.buildGraphicsArgs()...)

	// Network
	switch p.Config.NetworkMode.Mode {
	case "Bridge":
		args = append(args, "--network", fmt.Sprintf("bridge=%s,model=virtio", p.Config.NetworkMode.BridgeName))
	case "None":
		args = append(args, "--network", "none")
	default:
		args = append(args, "--network", "network=default,model=virtio")
	}

	// vsock always enabled
	args = append(args, "--vsock", "cid.auto=yes")

	// Sound
	if p.Config.EnableAudio {
		args = append(args, "--sound", "default")
	}

	// USB controller
	if p.Config.EnableUSBPassthrough {
		args = append(args, "--controller", "usb,model=qemu-xhci")
	}

	// Shared folders
	for _, folder := range p.Config.SharedFolders {
		args = append(args, "--filesystem",
			fmt.Sprintf("source=%s,target=%s,driver.type=virtiofs", folder.HostPath, folder.Tag))
	}
	if len(p.Config.SharedFolders) > 0 {
		args = append(args, "--memorybacking", "source.type=memfd,access.mode=shared")
	}

	cmd := exec.Command("sudo", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("virt-install failed: %s", strings.TrimSpace(string(output)))
	}

	// Stop the VM after import (user starts explicitly)
	time.Sleep(5 * time.Second)
	p.Virsh.DestroyUnchecked(p.Config.Name)

	// Enable Venus Vulkan
	if p.Config.GraphicsBackend == config.GraphicsVirtioGPU && !p.Config.Headless {
		gpuNodes := DetectGPURenderNodes()
		gpu := SelectGPUForVenus(gpuNodes)
		if gpu != nil {
			icdPath := GetVulkanICDPath(gpu.Vendor)
			if err := libvirt.EnableVenusVulkan(p.Virsh, p.Config.Name, p.Config.MemoryMB, gpu.ByPath, icdPath); err != nil {
				return fmt.Errorf("enabling Venus Vulkan: %w", err)
			}
		} else {
			slog.Warn("no suitable GPU found for Venus Vulkan")
		}
	}

	slog.Info("VM imported and ready")
	return nil
}

func (p *Provisioner) buildGraphicsArgs() []string {
	if p.Config.Headless {
		return []string{"--graphics", "none"}
	}

	switch p.Config.GraphicsBackend {
	case config.GraphicsVirtioGPU:
		gpuNodes := DetectGPURenderNodes()
		gpu := SelectGPUForVenus(gpuNodes)

		spiceArg := "spice,gl.enable=yes,listen=none"
		if gpu != nil {
			slog.Info("using GPU for SPICE", "node", gpu.ByPath, "vendor", gpu.Vendor)
			spiceArg = fmt.Sprintf("spice,gl.enable=yes,listen=none,rendernode=%s", gpu.ByPath)
		} else {
			slog.Warn("no GPU render node found, using default SPICE GL")
		}

		return []string{
			"--graphics", spiceArg,
			"--video", "virtio,model.heads=1,model.acceleration.accel3d=yes",
		}
	case config.GraphicsVNCOnly:
		return []string{"--graphics", "vnc,listen=127.0.0.1,port=5900"}
	default:
		return []string{"--graphics", "none"}
	}
}
