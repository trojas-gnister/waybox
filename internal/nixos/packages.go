package nixos

// MapPackage converts a Fedora/common package name to its nixpkgs equivalent.
// Returns empty string for packages handled by NixOS service options.
func MapPackage(name string) string {
	switch name {
	// Handled by NixOS services, not packages
	case "qemu-guest-agent", "pipewire", "wireplumber", "pipewire-pulse":
		return ""

	// Direct mappings
	case "openssh-server":
		return "openssh"
	case "mesa-vulkan-drivers":
		return "mesa"

	// GStreamer
	case "gstreamer1":
		return "gst_all_1.gstreamer"
	case "gstreamer1-plugins-base":
		return "gst_all_1.gst-plugins-base"
	case "gstreamer1-plugins-good":
		return "gst_all_1.gst-plugins-good"
	case "gstreamer1-plugins-bad-free":
		return "gst_all_1.gst-plugins-bad"
	case "gstreamer1-plugins-ugly-free":
		return "gst_all_1.gst-plugins-ugly"

	// Pass through as-is
	default:
		return name
	}
}

// MapPackages converts a slice of package names, filtering out empty results.
func MapPackages(names []string) []string {
	var result []string
	seen := make(map[string]bool)
	for _, name := range names {
		mapped := MapPackage(name)
		if mapped != "" && !seen[mapped] {
			result = append(result, mapped)
			seen[mapped] = true
		}
	}
	return result
}
