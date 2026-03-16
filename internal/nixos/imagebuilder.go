package nixos

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Custom qcow format for nixos-generate: auto-sized with growPartition
const qcowCustomNix = `{ config, lib, pkgs, modulesPath, ... }: {
  imports = [
    "${toString modulesPath}/profiles/qemu-guest.nix"
  ];

  fileSystems."/" = {
    device = "/dev/disk/by-label/nixos";
    autoResize = true;
    fsType = "ext4";
  };

  boot.growPartition = true;
  boot.kernelParams = ["console=ttyS0"];
  boot.loader.grub.device =
    if (pkgs.stdenv.system == "x86_64-linux")
    then (lib.mkDefault "/dev/vda")
    else (lib.mkDefault "nodev");

  boot.loader.grub.efiSupport = lib.mkIf (pkgs.stdenv.system != "x86_64-linux") (lib.mkDefault true);
  boot.loader.grub.efiInstallAsRemovable = lib.mkIf (pkgs.stdenv.system != "x86_64-linux") (lib.mkDefault true);
  boot.loader.timeout = 0;

  system.build.qcow = import "${toString modulesPath}/../lib/make-disk-image.nix" {
    inherit lib config pkgs;
    diskSize = "auto";
    additionalSpace = "2048M";
    format = "qcow2";
    partitionTableType = "hybrid";
  };

  formatAttr = "qcow";
  fileExtension = ".qcow2";
}`

// BuildImage creates a qcow2 VM image from a NixOS configuration string.
//
// Steps:
//  1. Write configuration.nix and custom qcow format to a temp directory
//  2. Run nixos-generate to build the image
//  3. Copy the image to the VM directory and resize it
func BuildImage(nixConfig, vmName, vmDir string, diskSizeGB uint64) (string, error) {
	tmpDir := filepath.Join(os.TempDir(), vmName+"-nixos")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write configuration.nix
	configPath := filepath.Join(tmpDir, "configuration.nix")
	if err := os.WriteFile(configPath, []byte(nixConfig), 0644); err != nil {
		return "", fmt.Errorf("writing configuration.nix: %w", err)
	}
	slog.Debug("wrote NixOS configuration", "path", configPath)

	// Write custom qcow format
	formatPath := filepath.Join(tmpDir, "qcow-custom.nix")
	if err := os.WriteFile(formatPath, []byte(qcowCustomNix), 0644); err != nil {
		return "", fmt.Errorf("writing qcow-custom.nix: %w", err)
	}
	slog.Debug("wrote custom qcow format", "path", formatPath)

	// Build image with nixos-generate
	slog.Info("building NixOS qcow2 image (this may take a few minutes on first build)...")
	cmd := exec.Command("nixos-generate", "--format-path", formatPath, "-c", configPath)
	cmd.Env = append(os.Environ(), "NIXPKGS_ALLOW_UNFREE=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("nixos-generate failed: %s", strings.TrimSpace(string(output)))
	}

	// nixos-generate prints the output path on stdout
	resultPath := strings.TrimSpace(string(output))
	// The last line of output is the path
	lines := strings.Split(resultPath, "\n")
	resultPath = strings.TrimSpace(lines[len(lines)-1])

	if _, err := os.Stat(resultPath); err != nil {
		return "", fmt.Errorf("nixos-generate output not found at %s", resultPath)
	}

	// Ensure VM directory exists
	if err := sudoRun("mkdir", "-p", vmDir); err != nil {
		return "", fmt.Errorf("creating VM directory: %w", err)
	}

	dest := filepath.Join(vmDir, vmName+".qcow2")

	// Remove existing disk
	_ = sudoRun("rm", "-f", dest)

	// Copy image (Nix store files are read-only hardlinks, can't move)
	if err := sudoRun("cp", "--no-preserve=mode,ownership", resultPath, dest); err != nil {
		return "", fmt.Errorf("copying image to libvirt directory: %w", err)
	}

	// Set permissions
	if err := sudoRun("chmod", "0644", dest); err != nil {
		return "", fmt.Errorf("setting image permissions: %w", err)
	}

	// Resize to user's desired disk size
	slog.Info("resizing image", "size_gb", diskSizeGB)
	if err := sudoRun("qemu-img", "resize", dest, fmt.Sprintf("%dG", diskSizeGB)); err != nil {
		return "", fmt.Errorf("resizing image: %w", err)
	}

	slog.Info("NixOS image built", "path", dest)
	return dest, nil
}

func sudoRun(args ...string) error {
	cmd := exec.Command("sudo", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return nil
}
