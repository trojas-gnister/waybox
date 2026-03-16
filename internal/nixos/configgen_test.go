package nixos

import (
	"strings"
	"testing"

	"github.com/trojas-gnister/waybox/internal/config"
)

func TestMapPackage(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"openssh-server", "openssh"},
		{"git", "git"},
		{"firefox", "firefox"},
		{"mesa-vulkan-drivers", "mesa"},
		{"qemu-guest-agent", ""},
		{"pipewire", ""},
		{"gstreamer1", "gst_all_1.gstreamer"},
		{"some-unknown-pkg", "some-unknown-pkg"},
	}

	for _, tt := range tests {
		got := MapPackage(tt.input)
		if got != tt.want {
			t.Errorf("MapPackage(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMapPackagesDedup(t *testing.T) {
	input := []string{"git", "git", "firefox", "pipewire"}
	got := MapPackages(input)

	// Should have git, firefox (no pipewire, no duplicate git)
	if len(got) != 2 {
		t.Errorf("MapPackages() returned %d items, want 2: %v", len(got), got)
	}
}

func TestGenerateConfigurationNix_BasicStructure(t *testing.T) {
	cfg, err := config.NewBuilder("test-vm").
		Headless(true).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Override password to make test deterministic
	cfg.UserPassword = "testpassword"

	nix, err := GenerateConfigurationNix(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Check essential sections are present
	checks := []string{
		`networking.hostName = "test-vm"`,
		`services.openssh.enable = true`,
		`services.qemuGuest.enable = true`,
		`system.stateVersion = "24.11"`,
		`boot.loader.grub.device = "/dev/vda"`,
		`boot.kernelModules = [ "vhost_vsock" ]`,
	}

	for _, check := range checks {
		if !strings.Contains(nix, check) {
			t.Errorf("generated config missing: %s", check)
		}
	}

	// Headless should NOT have venus, audio, waypipe, or auto-login
	shouldNotContain := []string{
		"hardware.graphics",
		"services.pipewire",
		"waypipe-server",
		"autologinUser",
	}
	for _, s := range shouldNotContain {
		if strings.Contains(nix, s) {
			t.Errorf("headless config should not contain: %s", s)
		}
	}
}

func TestGenerateConfigurationNix_GUIMode(t *testing.T) {
	cfg, err := config.NewBuilder("gui-vm").Build()
	if err != nil {
		t.Fatal(err)
	}
	cfg.UserPassword = "testpassword"

	nix, err := GenerateConfigurationNix(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// GUI mode should include Venus, audio, waypipe, auto-login
	checks := []string{
		"hardware.graphics",
		"MESA_LOADER_DRIVER_OVERRIDE",
		"services.pipewire",
		"waypipe-server",
		"autologinUser",
		"waypipe",
		"socat",
	}
	for _, check := range checks {
		if !strings.Contains(nix, check) {
			t.Errorf("GUI config missing: %s", check)
		}
	}
}

func TestGenerateConfigurationNix_Flatpak(t *testing.T) {
	cfg, err := config.NewBuilder("flatpak-vm").
		AddFlatpakPackage("org.mozilla.firefox").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	cfg.UserPassword = "testpassword"

	nix, err := GenerateConfigurationNix(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(nix, "services.flatpak.enable = true") {
		t.Error("missing flatpak service")
	}
	if !strings.Contains(nix, "org.mozilla.firefox") {
		t.Error("missing flatpak package install")
	}
}

func TestGenerateConfigurationNix_SharedFolders(t *testing.T) {
	cfg, err := config.NewBuilder("share-vm").
		AddSharedFolder(config.SharedFolder{
			HostPath:  "/home/user/docs",
			GuestPath: "/mnt/shared",
			Tag:       "share0",
			ReadOnly:  true,
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	cfg.UserPassword = "testpassword"

	nix, err := GenerateConfigurationNix(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(nix, `fileSystems."/mnt/shared"`) {
		t.Error("missing shared folder mount")
	}
	if !strings.Contains(nix, `"ro"`) {
		t.Error("missing readonly option")
	}
}

func TestGenerateConfigurationNix_NoNetwork(t *testing.T) {
	cfg, err := config.NewBuilder("airgapped-vm").
		NoNetwork().
		Build()
	if err != nil {
		t.Fatal(err)
	}
	cfg.UserPassword = "testpassword"

	nix, err := GenerateConfigurationNix(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(nix, "networking.useDHCP = false") {
		t.Error("airgapped VM should disable DHCP")
	}
}

func TestSanitizeServiceName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"org.mozilla.firefox", "org-mozilla-firefox"},
		{"com.spotify.Client", "com-spotify-client"},
	}
	for _, tt := range tests {
		got := sanitizeServiceName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeServiceName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
