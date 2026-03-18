package waypipe

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/trojas-gnister/waybox/internal/config"
)

// GenerateDesktopFiles creates .desktop files for all apps in a VM config.
func GenerateDesktopFiles(cfg *config.AppVMConfig) error {
	if cfg.Headless {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	appsDir := filepath.Join(home, ".local", "share", "applications")
	if err := os.MkdirAll(appsDir, 0755); err != nil {
		return err
	}

	// System packages that are known GUI apps
	for _, pkg := range cfg.SystemPackages {
		name, icon := guiAppInfo(pkg)
		if name == "" {
			continue
		}
		if err := writeDesktopFile(appsDir, cfg.Name, pkg, name, icon); err != nil {
			slog.Warn("failed to write desktop file", "app", pkg, "error", err)
		}
	}

	// Flatpak packages
	for _, pkg := range cfg.FlatpakPackages {
		name := deriveFlatpakName(pkg)
		cmd := fmt.Sprintf("flatpak run %s", pkg)
		if err := writeDesktopFile(appsDir, cfg.Name, cmd, name, ""); err != nil {
			slog.Warn("failed to write desktop file", "app", pkg, "error", err)
		}
	}

	return nil
}

func writeDesktopFile(appsDir, vmName, appCmd, appName, icon string) error {
	slug := slugify(appCmd)
	filename := fmt.Sprintf("vm-%s-%s.desktop", vmName, slug)
	path := filepath.Join(appsDir, filename)

	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=%s (%s)
Exec=waybox launch %s %s
Icon=%s
Categories=VM;
Comment=Launch %s in isolated VM %s
`, appName, vmName, vmName, appCmd, icon, appName, vmName)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	slog.Debug("wrote desktop file", "path", path)
	return nil
}

// guiAppInfo returns (display name, icon) for known GUI packages.
// Returns empty name for non-GUI packages.
func guiAppInfo(pkg string) (string, string) {
	switch pkg {
	case "firefox":
		return "Firefox", "firefox"
	case "chromium":
		return "Chromium", "chromium"
	case "gimp":
		return "GIMP", "gimp"
	case "libreoffice":
		return "LibreOffice", "libreoffice-startcenter"
	case "vlc":
		return "VLC", "vlc"
	case "thunderbird":
		return "Thunderbird", "thunderbird"
	case "inkscape":
		return "Inkscape", "inkscape"
	case "blender":
		return "Blender", "blender"
	case "krita":
		return "Krita", "krita"
	case "steam":
		return "Steam", "steam"
	case "signal-desktop":
		return "Signal", "signal-desktop"
	case "discord":
		return "Discord", "discord"
	case "spotify":
		return "Spotify", "spotify"
	case "obs-studio":
		return "OBS Studio", "com.obsproject.Studio"
	case "mpv":
		return "mpv", "mpv"
	case "evince":
		return "Evince", "org.gnome.Evince"
	default:
		return "", ""
	}
}

// deriveFlatpakName extracts a human-readable name from a flatpak ID.
func deriveFlatpakName(flatpakID string) string {
	// Well-known flatpak IDs
	known := map[string]string{
		"org.mozilla.firefox":              "Firefox",
		"io.gitlab.librewolf-community":    "LibreWolf",
		"org.chromium.Chromium":            "Chromium",
		"com.google.Chrome":               "Chrome",
		"org.gimp.GIMP":                   "GIMP",
		"org.libreoffice.LibreOffice":      "LibreOffice",
		"org.videolan.VLC":                "VLC",
		"org.mozilla.Thunderbird":          "Thunderbird",
		"org.inkscape.Inkscape":            "Inkscape",
		"org.blender.Blender":             "Blender",
		"org.kde.krita":                   "Krita",
		"com.valvesoftware.Steam":          "Steam",
		"org.signal.Signal":               "Signal",
		"com.discordapp.Discord":           "Discord",
		"com.spotify.Client":              "Spotify",
		"com.obsproject.Studio":            "OBS Studio",
	}

	if name, ok := known[flatpakID]; ok {
		return name
	}

	// Fallback: use last segment, title-cased
	parts := strings.Split(flatpakID, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return flatpakID
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, s)
	// Collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
