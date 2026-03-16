package config

import "fmt"

// ValidateVMName checks that a VM name is valid.
// Names must be 1-64 chars, alphanumeric plus hyphens/underscores,
// and cannot start with '-' or '.'.
func ValidateVMName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name cannot be empty", ErrInvalidVMName)
	}
	if len(name) > MaxVMNameLength {
		return fmt.Errorf("%w: name must be %d characters or less", ErrInvalidVMName, MaxVMNameLength)
	}
	for _, c := range name {
		if !isAlphanumeric(c) && c != '-' && c != '_' {
			return fmt.Errorf("%w: must contain only alphanumeric characters, hyphens, or underscores", ErrInvalidVMName)
		}
	}
	if name[0] == '-' || name[0] == '.' {
		return fmt.Errorf("%w: cannot start with '-' or '.'", ErrInvalidVMName)
	}
	return nil
}

// ValidateUSBDevice checks that vendor_id and product_id are valid 4-digit hex strings.
func ValidateUSBDevice(dev *UsbDevice) error {
	if !isValidHexID(dev.VendorID) {
		return fmt.Errorf("%w: vendor_id %q must be exactly 4 hex digits", ErrInvalidUSBID, dev.VendorID)
	}
	if !isValidHexID(dev.ProductID) {
		return fmt.Errorf("%w: product_id %q must be exactly 4 hex digits", ErrInvalidUSBID, dev.ProductID)
	}
	return nil
}

// ValidateSharedFolder checks that paths are absolute and don't contain "..".
func ValidateSharedFolder(f *SharedFolder) error {
	if len(f.HostPath) == 0 || f.HostPath[0] != '/' {
		return fmt.Errorf("%w: host_path %q must be absolute", ErrInvalidPath, f.HostPath)
	}
	if len(f.GuestPath) == 0 || f.GuestPath[0] != '/' {
		return fmt.Errorf("%w: guest_path %q must be absolute", ErrInvalidPath, f.GuestPath)
	}
	if containsDotDot(f.HostPath) || containsDotDot(f.GuestPath) {
		return fmt.Errorf("%w: paths cannot contain '..' (path traversal)", ErrInvalidPath)
	}
	return nil
}

func isAlphanumeric(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func isValidHexID(id string) bool {
	if len(id) != 4 {
		return false
	}
	for _, c := range id {
		if !isHexDigit(c) {
			return false
		}
	}
	return true
}

func isHexDigit(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func containsDotDot(path string) bool {
	for i := 0; i < len(path)-1; i++ {
		if path[i] == '.' && path[i+1] == '.' {
			return true
		}
	}
	return false
}
