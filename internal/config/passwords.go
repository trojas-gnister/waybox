package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

// VMPasswords stores VM passwords in a TOML file.
type VMPasswords struct {
	VMs map[string]string `toml:"vms"`
}

// NewVMPasswords creates an empty password store.
func NewVMPasswords() *VMPasswords {
	return &VMPasswords{VMs: make(map[string]string)}
}

// LoadPasswords loads from disk, creating a new store if the file doesn't exist.
func LoadPasswords() (*VMPasswords, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, PasswordFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewVMPasswords(), nil
		}
		return nil, err
	}

	var p VMPasswords
	if err := toml.Unmarshal(data, &p); err != nil {
		slog.Warn("corrupt password file, starting fresh", "error", err)
		return NewVMPasswords(), nil
	}
	if p.VMs == nil {
		p.VMs = make(map[string]string)
	}
	return &p, nil
}

// Save writes the passwords to disk.
func (p *VMPasswords) Save() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := toml.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshalling passwords: %w", err)
	}

	path := filepath.Join(dir, PasswordFileName)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	slog.Info("passwords saved", "path", path)
	return nil
}

// Add stores or updates a VM password.
func (p *VMPasswords) Add(vmName, password string) {
	p.VMs[vmName] = password
}

// Get returns the password for a VM, or empty string if not found.
func (p *VMPasswords) Get(vmName string) (string, bool) {
	pw, ok := p.VMs[vmName]
	return pw, ok
}

// Remove deletes a VM's password.
func (p *VMPasswords) Remove(vmName string) {
	delete(p.VMs, vmName)
}
