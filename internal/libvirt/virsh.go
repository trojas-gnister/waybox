package libvirt

import (
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

const qemuURI = "qemu:///system"

// Errors
var (
	ErrVirshFailed  = errors.New("virsh command failed")
	ErrVMNotFound   = fmt.Errorf("%w: VM not found", ErrVirshFailed)
	ErrVMRunning    = fmt.Errorf("%w: VM is running", ErrVirshFailed)
)

// Virsh defines the interface for all libvirt interactions.
// The default implementation shells out to virsh; this interface
// enables future migration to the Go libvirt API bindings.
type Virsh interface {
	RunChecked(args ...string) (string, error)
	RunSudoChecked(args ...string) (string, error)
	RunSudoUnchecked(args ...string) bool
	Start(vmName string) error
	StartIfStopped(vmName string) bool
	Shutdown(vmName string) error
	ShutdownUnchecked(vmName string) bool
	Destroy(vmName string) error
	DestroyUnchecked(vmName string) bool
	Undefine(vmName string, removeStorage bool) error
	Define(xmlPath string) error
	DumpXML(vmName string) (string, error)
	DomainExists(vmName string) bool
	ListAll() (string, error)
	GetVMIP(vmName string) (string, bool)
	GetVMState(vmName string) (string, bool)
	IsVMRunning(vmName string) bool
	GetDisplay(vmName string) (string, bool)
	AttachDevice(vmName, xmlPath string, live, config bool) error
	DetachDevice(vmName, xmlPath string, live, config bool) error
	SetMemory(vmName string, memoryMB uint64, maxMemory bool) error
	SetVCPUs(vmName string, count uint32, maximum bool) error
}

// SubprocessVirsh implements Virsh by shelling out to the virsh command.
type SubprocessVirsh struct{}

// NewVirsh creates a new subprocess-based Virsh implementation.
func NewVirsh() Virsh {
	return &SubprocessVirsh{}
}

func (v *SubprocessVirsh) RunChecked(args ...string) (string, error) {
	fullArgs := append([]string{"-c", qemuURI}, args...)
	slog.Debug("virsh", "args", strings.Join(args, " "))

	cmd := exec.Command("virsh", fullArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: virsh %s: %s", ErrVirshFailed, strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func (v *SubprocessVirsh) RunSudoChecked(args ...string) (string, error) {
	fullArgs := append([]string{"virsh", "-c", qemuURI}, args...)
	slog.Debug("sudo virsh", "args", strings.Join(args, " "))

	cmd := exec.Command("sudo", fullArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: virsh %s: %s", ErrVirshFailed, strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func (v *SubprocessVirsh) RunSudoUnchecked(args ...string) bool {
	fullArgs := append([]string{"virsh", "-c", qemuURI}, args...)
	slog.Debug("sudo virsh (unchecked)", "args", strings.Join(args, " "))

	cmd := exec.Command("sudo", fullArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Debug("virsh unchecked failed", "args", strings.Join(args, " "), "output", strings.TrimSpace(string(output)))
		return false
	}
	return true
}

func (v *SubprocessVirsh) Start(vmName string) error {
	_, err := v.RunSudoChecked("start", vmName)
	return err
}

func (v *SubprocessVirsh) StartIfStopped(vmName string) bool {
	return v.RunSudoUnchecked("start", vmName)
}

func (v *SubprocessVirsh) Shutdown(vmName string) error {
	_, err := v.RunSudoChecked("shutdown", vmName)
	return err
}

func (v *SubprocessVirsh) ShutdownUnchecked(vmName string) bool {
	return v.RunSudoUnchecked("shutdown", vmName)
}

func (v *SubprocessVirsh) Destroy(vmName string) error {
	_, err := v.RunSudoChecked("destroy", vmName)
	return err
}

func (v *SubprocessVirsh) DestroyUnchecked(vmName string) bool {
	return v.RunSudoUnchecked("destroy", vmName)
}

func (v *SubprocessVirsh) Undefine(vmName string, removeStorage bool) error {
	args := []string{"undefine", vmName}
	if removeStorage {
		args = append(args, "--remove-all-storage", "--nvram")
	}
	_, err := v.RunSudoChecked(args...)
	return err
}

func (v *SubprocessVirsh) Define(xmlPath string) error {
	_, err := v.RunSudoChecked("define", xmlPath)
	return err
}

func (v *SubprocessVirsh) DumpXML(vmName string) (string, error) {
	return v.RunSudoChecked("dumpxml", vmName)
}

func (v *SubprocessVirsh) DomainExists(vmName string) bool {
	_, err := v.RunSudoChecked("dominfo", vmName)
	return err == nil
}

func (v *SubprocessVirsh) ListAll() (string, error) {
	return v.RunSudoChecked("list", "--all")
}

func (v *SubprocessVirsh) GetVMIP(vmName string) (string, bool) {
	// Try standard method (NAT)
	if output, err := v.RunSudoChecked("domifaddr", vmName); err == nil {
		if ip := ParseIPFromDomifaddr(output); ip != "" {
			return ip, true
		}
	}
	// Fall back to guest agent (bridged)
	if output, err := v.RunSudoChecked("domifaddr", vmName, "--source", "agent"); err == nil {
		if ip := ParseIPFromDomifaddrAgent(output); ip != "" {
			return ip, true
		}
	}
	return "", false
}

func (v *SubprocessVirsh) GetVMState(vmName string) (string, bool) {
	output, err := v.RunSudoChecked("domstate", vmName)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(output), true
}

func (v *SubprocessVirsh) IsVMRunning(vmName string) bool {
	state, ok := v.GetVMState(vmName)
	return ok && state == "running"
}

func (v *SubprocessVirsh) GetDisplay(vmName string) (string, bool) {
	output, err := v.RunChecked("domdisplay", vmName)
	if err != nil {
		return "", false
	}
	s := strings.TrimSpace(output)
	if s == "" {
		return "", false
	}
	return s, true
}

func (v *SubprocessVirsh) AttachDevice(vmName, xmlPath string, live, config bool) error {
	args := []string{"attach-device", vmName, xmlPath}
	if live {
		args = append(args, "--live")
	}
	if config {
		args = append(args, "--config")
	}
	_, err := v.RunSudoChecked(args...)
	return err
}

func (v *SubprocessVirsh) DetachDevice(vmName, xmlPath string, live, config bool) error {
	args := []string{"detach-device", vmName, xmlPath}
	if live {
		args = append(args, "--live")
	}
	if config {
		args = append(args, "--config")
	}
	_, err := v.RunSudoChecked(args...)
	return err
}

func (v *SubprocessVirsh) SetMemory(vmName string, memoryMB uint64, maxMemory bool) error {
	cmd := "setmem"
	if maxMemory {
		cmd = "setmaxmem"
	}
	_, err := v.RunSudoChecked(cmd, vmName, fmt.Sprintf("%dM", memoryMB), "--config")
	return err
}

func (v *SubprocessVirsh) SetVCPUs(vmName string, count uint32, maximum bool) error {
	flag := "--current"
	if maximum {
		flag = "--maximum"
	}
	_, err := v.RunSudoChecked("setvcpus", vmName, fmt.Sprintf("%d", count), "--config", flag)
	return err
}
