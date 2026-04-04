# Waybox Design Specification

**Date:** 2026-04-04
**Status:** Approved
**Replaces:** vm-provisioner (Rust/Xpra) and waybox Go prototype

## Overview

Waybox is a Rust CLI tool for creating and managing lightweight KVM virtual machines that run applications as seamless, dedicated windows on the host desktop. It replaces vm-provisioner's X11/Xpra-based approach with a Wayland-native stack using waypipe over vsock.

### Design Principles

- **Single source of truth:** `WayboxConfig` drives both libvirt domain XML and NixOS guest configuration
- **Native bindings over shell-outs:** `virt` crate for libvirt, Askama for templates — no virsh parsing or string substitution
- **Compile-time safety:** Askama templates are checked at build time; typed config prevents invalid states
- **Minimal surface area:** Only features that serve the core use case — apps in VMs as host windows

## Language & Key Dependencies

- **Language:** Rust
- **Libvirt:** `virt` crate (native C API bindings)
- **CLI:** `clap` (derive)
- **Config:** `serde` + `toml`
- **Templates:** Askama (compile-time Jinja2-like templates)
- **Errors:** `thiserror`
- **Logging:** `log` + `env_logger`

## Feature Set

### Included

| Feature | Details |
|---------|---------|
| Guest OS | NixOS via nixos-generators (qcow2 images) |
| Display | waypipe over vsock (Wayland-native) |
| Audio | PipeWire over vsock |
| GPU | Venus/Vulkan (virtio-gpu, no IOMMU required) |
| USB | Permanent attachment at VM creation (no hot-plug) |
| Network | NAT (default) or airgapped (no network) |
| Shared folders | virtiofs (read-write or read-only) |
| Packages | Nix system packages + Flatpak apps |
| Desktop integration | .desktop file generation + launch command |
| Utilities | list, console, passwords |

### Excluded (vs vm-provisioner)

| Feature | Reason |
|---------|--------|
| PCI passthrough | Venus/Vulkan covers GPU needs without IOMMU complexity |
| USB hot-plug | Permanent-only simplifies the model |
| Bridged networking | NAT + airgapped covers all use cases; bridged was only needed for web streaming |
| CPU pinning | Without PCI passthrough gaming, the Linux scheduler handles allocation |
| Web streaming (Selkies) | Replaced by native waypipe |
| SPICE viewer | Replaced by waypipe |
| SSH communication | Replaced by vsock-only |
| Xpra | Replaced by waypipe (Wayland-native) |

## Architecture

```
+--------------------------------------------------+
|                 waybox CLI (clap)                 |
|   create | start | stop | destroy | launch |     |
|   list | console | passwords | generate-shortcuts|
+------------------------+-------------------------+
                         |
            +------------+------------+
            |                         |
    +-------v--------+    +----------v-----------+
    |  WayboxConfig  |    |  VMProvisioner       |
    |                |    |                      |
    | Typed config   |    | Orchestrates:        |
    | TOML storage   |    |  - NixOS image build |
    | Generates:     |    |  - libvirt define    |
    |  - libvirt XML |    |  - Device setup      |
    |  - NixOS config|    |  - Lifecycle ops     |
    +----------------+    +----------+-----------+
                                     |
             +-----------------------+--------------+
             |                       |              |
       +-----v------+    +----------v---+   +------v-------+
       | libvirt     |    | NixOS        |   | Display      |
       | (virt crate)|    | (Askama)     |   | (waypipe)    |
       |             |    |              |   |              |
       | Domain XML  |    | .nix.tmpl    |   | vsock session|
       | VM lifecycle|    | Image build  |   | Audio tunnel |
       | USB attach  |    | Package mgmt |   | .desktop gen |
       | Network cfg |    | Flatpak cfg  |   | App launch   |
       +-------------+    +--------------+   +--------------+
```

## Module Structure

```
waybox/
├── Cargo.toml
├── src/
│   ├── main.rs                  # Entry point, clap CLI definition
│   ├── lib.rs                   # Public API exports
│   ├── config/
│   │   ├── mod.rs               # WayboxConfig struct, builder, TOML serde
│   │   ├── validation.rs        # VM name, USB ID, path validation
│   │   └── passwords.rs         # Credential generation + storage
│   ├── libvirt/
│   │   ├── mod.rs               # virt crate connection management
│   │   ├── domain.rs            # Domain XML generation from WayboxConfig
│   │   ├── network.rs           # NAT / airgapped network setup
│   │   └── usb.rs               # USB device attachment at define time
│   ├── nixos/
│   │   ├── mod.rs               # NixOS config generation orchestrator
│   │   ├── image.rs             # nixos-generators image building
│   │   └── packages.rs          # Nix + Flatpak package mapping
│   ├── display/
│   │   ├── mod.rs               # Display session trait
│   │   ├── waypipe.rs           # waypipe vsock session management
│   │   ├── audio.rs             # PipeWire over vsock
│   │   └── desktop.rs           # .desktop file generation
│   └── error.rs                 # Typed error hierarchy (thiserror)
├── templates/
│   ├── base.nix.tmpl            # Core NixOS: boot, users, packages
│   ├── waypipe.nix.tmpl         # waypipe server + launcher daemon
│   ├── audio.nix.tmpl           # PipeWire configuration
│   ├── venus.nix.tmpl           # Vulkan GPU drivers
│   ├── virtiofs.nix.tmpl        # Shared folder mounts
│   └── flatpak.nix.tmpl         # Flatpak runtime + repos
```

## Configuration

### WayboxConfig

```rust
struct WayboxConfig {
    // Identity
    name: String,

    // Resources
    memory_mb: u32,          // default: 2048
    vcpus: u32,              // default: 2
    disk_gb: u32,            // default: 20

    // Packages
    system_packages: Vec<String>,    // nix packages
    flatpak_packages: Vec<String>,   // flathub apps

    // Devices
    usb_devices: Vec<UsbDevice>,     // vendor:product pairs
    shared_folders: Vec<SharedFolder>,

    // Network
    network_mode: NetworkMode,       // Nat | Airgapped

    // Display
    vsock_cid: u32,                  // auto-assigned: scan existing configs, pick next available starting from 3

    // Flags
    headless: bool,
    share_readonly: bool,
}
```

### Data Flow

```
CLI flags
   |
   v
WayboxConfig (validated, built)
   |
   +---> config/passwords.rs ---> ~/.config/waybox/vm-passwords.toml
   |
   +---> config/mod.rs ---> ~/.config/waybox/<name>.toml (saved)
   |
   +---> libvirt/domain.rs ---> Domain XML string ---> virt::Domain::define_xml()
   |
   +---> nixos/mod.rs ---> Complete NixOS config ---> nixos-generators ---> qcow2 image
   |
   +---> display/desktop.rs ---> ~/.local/share/applications/waybox-<name>-*.desktop
```

### Storage Paths

| Path | Contents |
|------|----------|
| `~/.config/waybox/<name>.toml` | VM configuration |
| `~/.config/waybox/vm-passwords.toml` | Credentials |
| `~/.local/share/waybox/images/<name>.qcow2` | VM disk images |
| `~/.local/share/applications/waybox-<name>-*.desktop` | App launchers |

### Provisioning Sequence

1. CLI parses flags, builds `WayboxConfig`, validates
2. Generate password, save config + credentials to TOML
3. Render NixOS templates (Askama) into temp dir, compose `configuration.nix`
4. Build qcow2 image via `nixos-generators`
5. Move image to `~/.local/share/waybox/images/<name>.qcow2`
6. Generate libvirt domain XML from config
7. Define domain via `virt::Domain::define_xml()`
8. USB devices are embedded in domain XML (no separate attach step)
9. Start VM, wait for vsock readiness
10. Generate `.desktop` files for installed apps

## Libvirt Integration

### Connection Management

```rust
struct LibvirtConnection {
    conn: virt::connect::Connect,
}

impl LibvirtConnection {
    fn new() -> Result<Self>                          // qemu:///system
    fn define_vm(&self, xml: &str) -> Result<()>
    fn start_vm(&self, name: &str) -> Result<()>
    fn stop_vm(&self, name: &str) -> Result<()>
    fn destroy_vm(&self, name: &str) -> Result<()>    // force stop + undefine + delete disk
    fn list_domains(&self) -> Result<Vec<DomainInfo>>
    fn get_domain_state(&self, name: &str) -> Result<DomainState>
}
```

### Domain XML Generation

`WayboxConfig` maps to libvirt domain XML:

| Config field | XML element |
|-------------|-------------|
| `memory_mb` | `<memory>` |
| `vcpus` | `<vcpu>` |
| `network_mode: Nat` | `<interface type="network">` (default network) |
| `network_mode: Airgapped` | No `<interface>` element |
| `vsock_cid` | `<vsock><cid auto="no" val="N"/></vsock>` |
| `shared_folders` | `<filesystem type="mount">` with virtiofs driver |
| `usb_devices` | `<hostdev mode="subsystem" type="usb">` |
| Venus GPU | `<video><model type="virtio" heads="1" 3d="yes"/></video>` |
| Disk image | `<disk type="file">` pointing to qcow2 |
| Serial console | `<serial type="pty">` + `<console type="pty">` |

### Network

- **NAT:** Uses libvirt `default` network. Verified as active via `virt` crate before VM creation.
- **Airgapped:** No `<interface>` element in domain XML. Guest communicates via vsock only.

## NixOS Guest Configuration

### Template Composition

Askama renders individual NixOS module files from `.nix.tmpl` templates. The orchestrator composes them into a build directory:

```
/tmp/waybox-build-<name>/
├── configuration.nix      # imports all modules below
├── base.nix               # from base.nix.tmpl
├── waypipe.nix            # from waypipe.nix.tmpl (skipped if headless)
├── audio.nix              # from audio.nix.tmpl (skipped if headless)
├── venus.nix              # from venus.nix.tmpl (skipped if headless)
├── virtiofs.nix           # from virtiofs.nix.tmpl (skipped if no shares)
└── flatpak.nix            # from flatpak.nix.tmpl (skipped if no flatpaks)
```

### Template Responsibilities

| Template | Configures |
|----------|-----------|
| `base.nix.tmpl` | Boot (UEFI/BIOS), user account, locale, system packages, vsock kernel module, serial console |
| `waypipe.nix.tmpl` | waypipe server on vsock, launcher daemon, Wayland session env vars (WAYLAND_DISPLAY, XDG_RUNTIME_DIR) |
| `audio.nix.tmpl` | PipeWire + WirePlumber, vsock audio bridge service |
| `venus.nix.tmpl` | Mesa Venus Vulkan driver, virtio-gpu DRM, Vulkan ICD loader |
| `virtiofs.nix.tmpl` | systemd mount units per shared folder, read-only flag |
| `flatpak.nix.tmpl` | Flatpak runtime, Flathub remote, pre-install listed apps on first boot |

### Image Building

```
WayboxConfig
    |
    v
Render templates to temp dir
    |
    v
nixos-generators --format qcow2 --configuration /tmp/waybox-build-<name>/configuration.nix
    |
    v
Move image to ~/.local/share/waybox/images/<name>.qcow2
    |
    v
Clean up temp dir
```

### Package Mapping

`nixos/packages.rs` maps user-friendly names to Nix attribute paths where they differ (e.g., `python3` to `python3Full`). Flatpak packages pass through as Flathub app IDs (e.g., `org.mozilla.firefox`).

## Display Layer

### waypipe Session Management

```
Host side                          Guest side (NixOS service)
---------                          --------------------------
waybox start <vm>                  systemd: waypipe-server.service
    |                                  |
    v                                  v
waypipe --vsock listen <CID>:<port>   waypipe --vsock connect <CID>:<port>
    |                                  |
    v                                  v
Receives Wayland                   Forwards app windows
compositor traffic                 from guest compositor
    |
    v
App windows appear on
host Wayland compositor
```

Host-side waypipe is a long-running child process. On `waybox start`, it spawns waypipe and stores the PID. On `waybox stop`, it terminates the process.

### Launch Command

```
waybox launch <vm> "firefox"
    |
    v
Send command to guest launcher daemon over vsock (control port)
    |
    v
Guest daemon spawns: waypipe --vsock connect <CID>:<port> -- firefox
    |
    v
Firefox window appears on host desktop
```

The guest runs a launcher daemon (configured in `waypipe.nix`) that listens on a dedicated vsock port for launch requests. Simple protocol: send command string, receive success/failure.

### Audio

```
Guest: PipeWire --> vsock audio bridge --> vsock port
Host:  vsock listener --> PipeWire on host
```

A dedicated vsock port carries audio. The guest bridge service captures PipeWire output and sends it over vsock. The host side connects it to the local PipeWire instance.

### Desktop File Generation

For each installed app, generates a `.desktop` file at `~/.local/share/applications/waybox-<vm>-<app>.desktop`. The `Exec` line runs `waybox launch <vm> "<app>"`. On `waybox destroy`, these files are cleaned up.

App discovery: query the guest over the control vsock port. The launcher daemon lists installed `.desktop` files and returns names, icons, and exec commands.

### vsock Port Allocation

| Port | Purpose |
|------|---------|
| 5000 | waypipe display |
| 5001 | Audio bridge |
| 5002 | Control (launch commands, app listing) |

Ports are fixed per VM. The CID differentiates VMs, not port numbers.

## Error Handling

Single flat enum using `thiserror`:

```rust
#[derive(Debug, thiserror::Error)]
enum WayboxError {
    // Config
    #[error("Invalid VM name '{0}': {1}")]
    InvalidName(String, String),
    #[error("Config not found for VM '{0}'")]
    ConfigNotFound(String),
    #[error("Config error: {0}")]
    ConfigIo(#[from] std::io::Error),

    // Libvirt
    #[error("Libvirt connection failed: {0}")]
    LibvirtConnect(String),
    #[error("VM '{0}' not found")]
    VmNotFound(String),
    #[error("VM '{0}' is already {1}")]
    VmWrongState(String, String),

    // NixOS
    #[error("NixOS image build failed: {0}")]
    ImageBuild(String),
    #[error("nixos-generators not found — is nix installed?")]
    NixNotFound,

    // Display
    #[error("waypipe session failed: {0}")]
    WaypipeError(String),
    #[error("Audio bridge failed: {0}")]
    AudioError(String),
    #[error("Guest not responding on vsock")]
    VsockTimeout,

    // USB
    #[error("USB device {0} not found")]
    UsbNotFound(String),
}
```

## CLI Interface

```
waybox create    --name <name>
                 --system <pkg>...
                 --flatpak <pkg>...
                 --memory <MB>           (default: 2048)
                 --vcpus <N>             (default: 2)
                 --disk <GB>             (default: 20)
                 --usb <vendor:product>...
                 --share <host>:<guest>...
                 --share-readonly
                 --no-network
                 --headless
                 -y, --yes

waybox start     <name>
waybox stop      <name>
waybox destroy   <name> [-y]
waybox launch    <name> <command>
waybox list
waybox console   <name>
waybox passwords
waybox generate-shortcuts <name>
```

## Prerequisites

Checked at startup before any operation:

- `libvirtd` running (verified via `virt` crate connection attempt)
- `nix` in PATH
- `nixos-generators` in PATH
- `waypipe` in PATH
- `vhost_vsock` kernel module loaded

## Testing Strategy

### Unit Tests (in-module `#[cfg(test)]`)

- **config/** — Validation logic (VM names, USB IDs, paths), builder defaults, TOML round-trip serialization
- **libvirt/domain.rs** — XML generation: given a `WayboxConfig`, assert produced XML contains correct elements
- **nixos/mod.rs** — Template rendering: given a config, assert rendered `.nix` files contain expected content
- **display/desktop.rs** — `.desktop` file content generation

### Integration Tests (`tests/`)

- **Prerequisite checker** — Verify detection of missing tools (mock PATH)
- **Config lifecycle** — Create config, save to TOML, load back, verify equality
- **NixOS composition** — Full template render, verify `configuration.nix` imports all modules

### Not Automatically Tested

- Actual VM creation (requires libvirtd, nix, hardware)
- waypipe session establishment (requires running VM)
- Audio bridge (requires PipeWire on both ends)

Testing boundary: everything up to calling an external system is tested; external calls are verified manually.
