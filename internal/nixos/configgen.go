package nixos

import (
	"bytes"
	"crypto/rand"
	"embed"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/trojas-gnister/waybox/internal/config"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// NixTemplateData holds all data needed to render the NixOS configuration.
type NixTemplateData struct {
	Config         *config.AppVMConfig
	HashedPassword string
	SSHPublicKey   string
	MappedPackages []string
	NetworkMode    string // "Nat", "None", "Bridge"
	VenusEnabled   bool
	AudioEnabled   bool
	HasFlatpak     bool
	AutoLogin      bool
	CustomNixConfig string
	WaypipePort    uint32
	AudioPort      uint32
	LauncherPort   uint32
}

// GenerateConfigurationNix produces a complete configuration.nix from the given config.
func GenerateConfigurationNix(cfg *config.AppVMConfig) (string, error) {
	sshKey, err := getSSHPublicKey()
	if err != nil {
		return "", fmt.Errorf("getting SSH public key: %w", err)
	}

	hashedPw := hashPassword(cfg.UserPassword)

	// Map packages and add waypipe + socat for display/audio
	pkgs := MapPackages(cfg.SystemPackages)
	if !cfg.Headless {
		pkgs = addUnique(pkgs, "waypipe")
		pkgs = addUnique(pkgs, "socat")
	}

	// Add Venus packages
	venusEnabled := cfg.GraphicsBackend == config.GraphicsVirtioGPU && !cfg.Headless
	if venusEnabled {
		pkgs = addUnique(pkgs, "mesa")
		pkgs = addUnique(pkgs, "vulkan-loader")
		pkgs = addUnique(pkgs, "vulkan-tools")
	}

	customNix := ""
	if cfg.CustomNixConfig != nil {
		customNix = *cfg.CustomNixConfig
	}

	data := NixTemplateData{
		Config:          cfg,
		HashedPassword:  hashedPw,
		SSHPublicKey:    escapeNixString(sshKey),
		MappedPackages:  pkgs,
		NetworkMode:     cfg.NetworkMode.Mode,
		VenusEnabled:    venusEnabled,
		AudioEnabled:    cfg.EnableAudio && !cfg.Headless,
		HasFlatpak:      len(cfg.FlatpakPackages) > 0 && !cfg.Headless,
		AutoLogin:       cfg.EnableAutoLogin && !cfg.Headless,
		CustomNixConfig: customNix,
		WaypipePort:     cfg.WaypipePort,
		AudioPort:       cfg.AudioPort,
		LauncherPort:    config.DefaultLauncherPort,
	}

	funcMap := template.FuncMap{
		"sanitizeServiceName": sanitizeServiceName,
	}

	tmpl, err := template.New("base.nix.tmpl").Funcs(funcMap).ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return "", fmt.Errorf("parsing NixOS templates: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing NixOS template: %w", err)
	}

	result := buf.String()

	// Validate syntax if nix-instantiate is available
	if err := validateNixSyntax(result); err != nil {
		slog.Warn("nix syntax validation failed", "error", err)
	}

	slog.Debug("generated NixOS configuration", "bytes", len(result))
	return result, nil
}

// getSSHPublicKey finds or generates an SSH public key.
func getSSHPublicKey() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	sshDir := filepath.Join(home, ".ssh")

	// Check existing keys in priority order
	for _, keyType := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
		pubPath := filepath.Join(sshDir, keyType+".pub")
		data, err := os.ReadFile(pubPath)
		if err == nil {
			return strings.TrimSpace(string(data)), nil
		}
	}

	// Generate new ed25519 key
	slog.Info("no SSH key found, generating ed25519 key")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return "", err
	}

	keyPath := filepath.Join(sshDir, "id_ed25519")
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-N", "", "-q")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("generating SSH key: %w", err)
	}

	data, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// hashPassword creates a SHA-512 crypt hash for use in NixOS hashedPassword.
func hashPassword(password string) string {
	salt := generateSalt(16)

	// Try mkpasswd first
	cmd := exec.Command("mkpasswd", "--method=sha-512", "--salt", salt, password)
	if output, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(output))
	}

	// Fallback to openssl
	cmd = exec.Command("openssl", "passwd", "-6", "-salt", salt, password)
	if output, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(output))
	}

	// Last resort placeholder
	return fmt.Sprintf("$6$%s$placeholder", salt)
}

func generateSalt(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789./"
	salt := make([]byte, length)
	for i := range salt {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		salt[i] = charset[n.Int64()]
	}
	return string(salt)
}

// validateNixSyntax checks Nix syntax by shelling out to nix-instantiate.
func validateNixSyntax(nixSource string) error {
	cmd := exec.Command("nix-instantiate", "--parse", "-")
	cmd.Stdin = strings.NewReader(nixSource)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nix syntax error: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func escapeNixString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func sanitizeServiceName(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, ".", "-"))
}

func addUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}
