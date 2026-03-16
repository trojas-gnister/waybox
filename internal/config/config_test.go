package config

import (
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

func TestValidateVMName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"my-vm", false},
		{"test_123", false},
		{"a", false},
		{"", true},
		{"-leading-dash", true},
		{".leading-dot", true},
		{"has spaces", true},
		{"has/slash", true},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true}, // 67 chars
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVMName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVMName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestValidateUSBDevice(t *testing.T) {
	tests := []struct {
		vendor  string
		product string
		wantErr bool
	}{
		{"046d", "c52b", false},
		{"1234", "5678", false},
		{"ABCD", "ef01", false},
		{"046", "c52b", true},   // too short
		{"046dd", "c52b", true}, // too long
		{"046g", "c52b", true},  // non-hex
	}

	for _, tt := range tests {
		dev := &UsbDevice{VendorID: tt.vendor, ProductID: tt.product}
		err := ValidateUSBDevice(dev)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateUSBDevice(%s:%s) error = %v, wantErr %v", tt.vendor, tt.product, err, tt.wantErr)
		}
	}
}

func TestValidateSharedFolder(t *testing.T) {
	tests := []struct {
		host    string
		guest   string
		wantErr bool
	}{
		{"/home/user/docs", "/mnt/shared", false},
		{"relative/path", "/mnt/shared", true},
		{"/home/user/docs", "relative", true},
		{"/home/../etc/passwd", "/mnt/shared", true},
		{"/home/user", "/mnt/../etc", true},
	}

	for _, tt := range tests {
		f := &SharedFolder{HostPath: tt.host, GuestPath: tt.guest, Tag: "test"}
		err := ValidateSharedFolder(f)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateSharedFolder(%s, %s) error = %v, wantErr %v", tt.host, tt.guest, err, tt.wantErr)
		}
	}
}

func TestBuilderDefaults(t *testing.T) {
	cfg, err := NewBuilder("test-vm").Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	if cfg.Name != "test-vm" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-vm")
	}
	if cfg.MemoryMB != DefaultMemoryMB {
		t.Errorf("MemoryMB = %d, want %d", cfg.MemoryMB, DefaultMemoryMB)
	}
	if cfg.VCPUs != DefaultVCPUs {
		t.Errorf("VCPUs = %d, want %d", cfg.VCPUs, DefaultVCPUs)
	}
	if cfg.DiskSizeGB != DefaultDiskSizeGB {
		t.Errorf("DiskSizeGB = %d, want %d", cfg.DiskSizeGB, DefaultDiskSizeGB)
	}
	if !cfg.EnableVsock {
		t.Error("EnableVsock should be true by default")
	}
	if cfg.GraphicsBackend != GraphicsVirtioGPU {
		t.Errorf("GraphicsBackend = %q, want %q", cfg.GraphicsBackend, GraphicsVirtioGPU)
	}
	if cfg.WaypipePort != DefaultWaypipePort {
		t.Errorf("WaypipePort = %d, want %d", cfg.WaypipePort, DefaultWaypipePort)
	}
	if len(cfg.UserPassword) != PasswordLength {
		t.Errorf("UserPassword length = %d, want %d", len(cfg.UserPassword), PasswordLength)
	}
}

func TestBuilderHeadless(t *testing.T) {
	cfg, err := NewBuilder("headless-vm").Headless(true).Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	if cfg.GraphicsBackend != GraphicsVNCOnly {
		t.Errorf("headless GraphicsBackend = %q, want %q", cfg.GraphicsBackend, GraphicsVNCOnly)
	}
	if cfg.EnableAudio {
		t.Error("headless should have EnableAudio = false")
	}
	if cfg.EnableAutoLogin {
		t.Error("headless should have EnableAutoLogin = false")
	}
}

func TestGraphicsBackendTOMLRoundTrip(t *testing.T) {
	type wrapper struct {
		Backend GraphicsBackend `toml:"backend"`
	}

	orig := wrapper{Backend: GraphicsVirtioGPU}
	data, err := toml.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}

	var decoded wrapper
	if err := toml.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Backend != orig.Backend {
		t.Errorf("round-trip: got %q, want %q", decoded.Backend, orig.Backend)
	}
}

func TestGraphicsBackendMigration(t *testing.T) {
	type wrapper struct {
		Backend GraphicsBackend `toml:"backend"`
	}

	data := []byte(`backend = "QxlSpice"`)
	var w wrapper
	if err := toml.Unmarshal(data, &w); err != nil {
		t.Fatal(err)
	}
	if w.Backend != GraphicsVirtioGPU {
		t.Errorf("QxlSpice migration: got %q, want %q", w.Backend, GraphicsVirtioGPU)
	}
}

func TestGeneratePasswordUniqueness(t *testing.T) {
	p1 := GeneratePassword()
	p2 := GeneratePassword()
	if p1 == p2 {
		t.Error("two generated passwords should not be identical")
	}
	if len(p1) != PasswordLength {
		t.Errorf("password length = %d, want %d", len(p1), PasswordLength)
	}
}
