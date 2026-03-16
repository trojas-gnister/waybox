package config

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

// Constants
const (
	DefaultVMDir     = "/var/lib/libvirt/images"
	ConfigDirName    = ".config/waybox"
	PasswordFileName = "vm-passwords.toml"

	QemuURI          = "qemu:///system"
	DefaultVNCPort   = 5900
	DefaultSPICEPort = 5900

	DefaultDiskSizeGB = 20
	DefaultMemoryMB   = 2048
	DefaultVCPUs      = 2

	VMBootWaitSecs    = 5
	ShutdownWaitSecs  = 30

	PasswordLength  = 16
	PasswordCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	MaxVMNameLength = 64

	DefaultUserName    = "user"
	DefaultUserUID     = 1000
	DefaultPulseSocket = "/run/user/1000/pulse/native"

	NixOSChannel = "nixos-24.11"

	GPUVendorAMD    = "1002"
	GPUVendorIntel  = "8086"
	GPUVendorNVIDIA = "10de"
	VulkanICDDir    = "/usr/share/vulkan/icd.d"

	DefaultWaypipePort = 1100
	DefaultAudioPort   = 1200
)

// Errors
var (
	ErrInvalidConfig    = errors.New("invalid configuration")
	ErrInvalidVMName    = fmt.Errorf("%w: invalid VM name", ErrInvalidConfig)
	ErrInvalidUSBID     = fmt.Errorf("%w: invalid USB ID", ErrInvalidConfig)
	ErrInvalidPath      = fmt.Errorf("%w: invalid path", ErrInvalidConfig)
	ErrConfigNotFound   = fmt.Errorf("%w: config file not found", ErrInvalidConfig)
	ErrHomeNotSet       = fmt.Errorf("%w: HOME environment variable not set", ErrInvalidConfig)
)

// GraphicsBackend represents the VM graphics mode.
type GraphicsBackend string

const (
	GraphicsVirtioGPU GraphicsBackend = "VirtioGpu"
	GraphicsVNCOnly   GraphicsBackend = "VncOnly"
)

// UnmarshalText handles deserialization including migration from old QxlSpice.
func (g *GraphicsBackend) UnmarshalText(text []byte) error {
	switch string(text) {
	case "VirtioGpu":
		*g = GraphicsVirtioGPU
	case "VncOnly":
		*g = GraphicsVNCOnly
	case "QxlSpice":
		*g = GraphicsVirtioGPU // migrate
	default:
		return fmt.Errorf("unknown graphics backend: %q (supported: VirtioGpu, VncOnly)", string(text))
	}
	return nil
}

// MarshalText serializes the graphics backend.
func (g GraphicsBackend) MarshalText() ([]byte, error) {
	return []byte(g), nil
}

// NetworkMode represents how the VM connects to the network.
type NetworkMode struct {
	Mode       string `toml:"-" json:"-"`
	BridgeName string `toml:"-" json:"-"`
}

var (
	NetworkNat  = NetworkMode{Mode: "Nat"}
	NetworkNone = NetworkMode{Mode: "None"}
)

func NetworkBridge(name string) NetworkMode {
	return NetworkMode{Mode: "Bridge", BridgeName: name}
}

// MarshalTOML handles the Rust serde-compatible TOML format.
// Nat/None serialize as strings, Bridge as a map.
func (n NetworkMode) MarshalTOML() ([]byte, error) {
	switch n.Mode {
	case "Bridge":
		return []byte(fmt.Sprintf(`{Bridge = %q}`, n.BridgeName)), nil
	default:
		return []byte(fmt.Sprintf("%q", n.Mode)), nil
	}
}

// UnmarshalTOML handles both string ("Nat") and map ({Bridge = "br0"}) forms.
func (n *NetworkMode) UnmarshalTOML(data []byte) error {
	// Try string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		switch s {
		case "Nat":
			*n = NetworkNat
		case "None":
			*n = NetworkNone
		default:
			return fmt.Errorf("unknown network mode: %q", s)
		}
		return nil
	}

	// Try map form: {"Bridge": "br0"}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err == nil {
		if bridge, ok := m["Bridge"]; ok {
			*n = NetworkBridge(bridge)
			return nil
		}
	}

	return fmt.Errorf("cannot parse network mode from: %s", string(data))
}

// UsbDevice represents a USB device for passthrough.
type UsbDevice struct {
	VendorID    string `toml:"vendor_id"`
	ProductID   string `toml:"product_id"`
	Description string `toml:"description"`
	Bus         *uint8 `toml:"bus,omitempty"`
	Device      *uint8 `toml:"device,omitempty"`
}

// SharedFolder represents a virtiofs mount.
type SharedFolder struct {
	HostPath  string `toml:"host_path"`
	GuestPath string `toml:"guest_path"`
	Tag       string `toml:"tag"`
	ReadOnly  bool   `toml:"readonly"`
}

// AppVMConfig is the complete configuration for an application VM.
type AppVMConfig struct {
	Name      string `toml:"name"`
	MemoryMB  uint64 `toml:"memory_mb"`
	VCPUs     uint32 `toml:"vcpus"`
	DiskSizeGB uint64 `toml:"disk_size_gb"`
	VMDir     string `toml:"vm_dir"`

	SystemPackages  []string `toml:"system_packages"`
	FlatpakPackages []string `toml:"flatpak_packages"`
	AutoLaunchApps  []string `toml:"auto_launch_apps,omitempty"`

	GraphicsBackend    GraphicsBackend `toml:"graphics_backend"`
	EnableClipboard    bool            `toml:"enable_clipboard"`
	EnableAudio        bool            `toml:"enable_audio"`
	EnableUSBPassthrough bool          `toml:"enable_usb_passthrough"`
	EnableAutoLogin    bool            `toml:"enable_auto_login"`
	Headless           bool            `toml:"headless"`
	GrantDeviceAccess  bool            `toml:"grant_device_access,omitempty"`

	USBDevices []UsbDevice `toml:"usb_devices"`
	USBHotplug bool        `toml:"usb_hotplug"`

	SharedFolders []SharedFolder `toml:"shared_folders,omitempty"`

	NetworkMode   NetworkMode `toml:"network_mode"`
	FirewallRules []string    `toml:"firewall_rules"`

	VsockCID    *uint32 `toml:"vsock_cid,omitempty"`
	EnableVsock bool    `toml:"enable_vsock"`

	UserPassword string `toml:"user_password"`

	CustomNixConfig *string `toml:"custom_nix_config,omitempty"`

	// waybox-specific fields
	WaypipePort uint32 `toml:"waypipe_port,omitempty"`
	AudioPort   uint32 `toml:"audio_port,omitempty"`
}

// ConfigDir returns the path to the waybox config directory.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ErrHomeNotSet
	}
	return filepath.Join(home, ConfigDirName), nil
}

// ConfigPath returns the path to a specific VM's config file.
func ConfigPath(vmName string) (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, vmName+".toml"), nil
}

// Load reads a VM config from disk by name.
func Load(vmName string) (*AppVMConfig, error) {
	path, err := ConfigPath(vmName)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrConfigNotFound, vmName)
		}
		return nil, err
	}

	var cfg AppVMConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config for %s: %w", vmName, err)
	}
	return &cfg, nil
}

// Save writes the VM config to disk.
func (c *AppVMConfig) Save() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := toml.Marshal(c)
	if err != nil {
		return err
	}

	path := filepath.Join(dir, c.Name+".toml")
	return os.WriteFile(path, data, 0644)
}

// GeneratePassword creates a random password of PasswordLength characters.
func GeneratePassword() string {
	charset := []byte(PasswordCharset)
	password := make([]byte, PasswordLength)
	for i := range password {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		password[i] = charset[n.Int64()]
	}
	return string(password)
}

// Builder provides an ergonomic way to construct AppVMConfig.
type Builder struct {
	name              string
	memoryMB          uint64
	vcpus             uint32
	diskSizeGB        uint64
	systemPackages    []string
	flatpakPackages   []string
	headless          bool
	usbDevices        []UsbDevice
	usbHotplug        bool
	sharedFolders     []SharedFolder
	networkMode       NetworkMode
	grantDeviceAccess bool
	customNixConfig   *string
}

// NewBuilder creates a builder with sensible defaults.
func NewBuilder(name string) *Builder {
	return &Builder{
		name:        name,
		memoryMB:    DefaultMemoryMB,
		vcpus:       DefaultVCPUs,
		diskSizeGB:  DefaultDiskSizeGB,
		networkMode: NetworkNat,
	}
}

func (b *Builder) Memory(mb uint64) *Builder         { b.memoryMB = mb; return b }
func (b *Builder) VCPUs(n uint32) *Builder            { b.vcpus = n; return b }
func (b *Builder) DiskSize(gb uint64) *Builder        { b.diskSizeGB = gb; return b }
func (b *Builder) Headless(h bool) *Builder           { b.headless = h; return b }
func (b *Builder) USBHotplug(h bool) *Builder         { b.usbHotplug = h; return b }
func (b *Builder) GrantDeviceAccess(g bool) *Builder  { b.grantDeviceAccess = g; return b }

func (b *Builder) SystemPackages(pkgs []string) *Builder {
	b.systemPackages = pkgs
	return b
}

func (b *Builder) AddSystemPackage(pkg string) *Builder {
	b.systemPackages = append(b.systemPackages, pkg)
	return b
}

func (b *Builder) FlatpakPackages(pkgs []string) *Builder {
	b.flatpakPackages = pkgs
	return b
}

func (b *Builder) AddFlatpakPackage(pkg string) *Builder {
	b.flatpakPackages = append(b.flatpakPackages, pkg)
	return b
}

func (b *Builder) USBDevices(devs []UsbDevice) *Builder {
	b.usbDevices = devs
	return b
}

func (b *Builder) AddUSBDevice(dev UsbDevice) *Builder {
	b.usbDevices = append(b.usbDevices, dev)
	return b
}

func (b *Builder) SharedFolders(folders []SharedFolder) *Builder {
	b.sharedFolders = folders
	return b
}

func (b *Builder) AddSharedFolder(folder SharedFolder) *Builder {
	b.sharedFolders = append(b.sharedFolders, folder)
	return b
}

func (b *Builder) Network(mode NetworkMode) *Builder {
	b.networkMode = mode
	return b
}

func (b *Builder) NoNetwork() *Builder {
	b.networkMode = NetworkNone
	return b
}

func (b *Builder) Bridge(name string) *Builder {
	b.networkMode = NetworkBridge(name)
	return b
}

func (b *Builder) CustomNixConfig(cfg string) *Builder {
	b.customNixConfig = &cfg
	return b
}

// Build validates and returns the final AppVMConfig.
func (b *Builder) Build() (*AppVMConfig, error) {
	if err := ValidateVMName(b.name); err != nil {
		return nil, err
	}

	// Validate USB devices
	for _, dev := range b.usbDevices {
		if err := ValidateUSBDevice(&dev); err != nil {
			return nil, err
		}
	}

	// Validate shared folders
	for _, f := range b.sharedFolders {
		if err := ValidateSharedFolder(&f); err != nil {
			return nil, err
		}
	}

	// Default system packages — no WM needed, waypipe is the display layer
	defaultPkgs := []string{"git"}
	if !b.headless {
		defaultPkgs = append(defaultPkgs, "openssh-server")
	}
	allPkgs := append(defaultPkgs, b.systemPackages...)

	flatpakPkgs := b.flatpakPackages
	if b.headless {
		flatpakPkgs = nil
	}

	graphics := GraphicsVirtioGPU
	if b.headless {
		graphics = GraphicsVNCOnly
	}

	cfg := &AppVMConfig{
		Name:       b.name,
		MemoryMB:   b.memoryMB,
		VCPUs:      b.vcpus,
		DiskSizeGB: b.diskSizeGB,
		VMDir:      DefaultVMDir,

		SystemPackages:  allPkgs,
		FlatpakPackages: flatpakPkgs,
		AutoLaunchApps:  nil,

		GraphicsBackend:    graphics,
		EnableClipboard:    !b.headless,
		EnableAudio:        !b.headless,
		EnableUSBPassthrough: len(b.usbDevices) > 0,
		EnableAutoLogin:    !b.headless,
		Headless:           b.headless,
		GrantDeviceAccess:  b.grantDeviceAccess,

		USBDevices: b.usbDevices,
		USBHotplug: b.usbHotplug,

		SharedFolders: b.sharedFolders,

		NetworkMode: b.networkMode,
		FirewallRules: []string{
			"OUTPUT -p udp --dport 53 -j ACCEPT",
			"OUTPUT -p tcp --dport 53 -j ACCEPT",
			"OUTPUT -p tcp --dport 80 -j ACCEPT",
			"OUTPUT -p tcp --dport 443 -j ACCEPT",
		},

		// vsock always enabled in waybox
		VsockCID:    nil,
		EnableVsock: true,

		UserPassword: GeneratePassword(),

		CustomNixConfig: b.customNixConfig,

		WaypipePort: DefaultWaypipePort,
		AudioPort:   DefaultAudioPort,
	}

	return cfg, nil
}
