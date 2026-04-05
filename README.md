# waybox

Isolated application VMs with native Wayland integration.

waybox creates lightweight NixOS VMs where each VM serves a single application. Apps appear as regular windows on your host Wayland desktop via [waypipe](https://gitlab.freedesktop.org/mstoeckl/waypipe) over vsock — no remote desktop, no compositor in the guest, just native windows.

## Features

- **One app, one VM** — full isolation between applications
- **Native Wayland windows** — apps appear on your host desktop via waypipe
- **Venus Vulkan GPU acceleration** — near-native 3D graphics via virtio-gpu
- **Audio forwarding** — PipeWire audio over vsock
- **USB passthrough** — permanent device passthrough at VM creation
- **Shared folders** — virtiofs mounts between host and guest (read-write or read-only)
- **Declarative guests** — NixOS configuration via Askama templates, no manual setup
- **Desktop integration** — `.desktop` files generated for each app
- **Native libvirt bindings** — uses the `virt` crate directly, no virsh shell-outs

## Architecture

```
Host (Linux)                            Guest (NixOS VM)
┌───────────────────────┐              ┌──────────────────────┐
│  waybox               │              │                      │
│                       │              │  waypipe server      │
│  waypipe client ◄─────┼── vsock ────►│       │              │
│                       │              │   application        │
│  PipeWire ◄───────────┼── vsock ────►│  audio bridge        │
│  libvirt (virt crate) ┼── qemu ────►│  Venus Vulkan        │
│  USB passthrough ─────┼── libvirt ──►│  (virtio-gpu)        │
└───────────────────────┘              └──────────────────────┘
```

All transport uses vsock — no SSH tunnels, no TCP ports.

## Requirements

### Host
- Linux kernel >= 6.13 (blob/TTM fix for Venus)
- QEMU >= 9.2
- libvirt + libvirt-dev
- Nix (for nixos-generators)
- waypipe
- socat
- Wayland compositor
- `vhost_vsock` kernel module
- GPU: AMD (RADV) or Intel (ANV). NVIDIA 590+ experimental.
- `CONFIG_UDMABUF` enabled in kernel

### Guest (auto-configured)
- NixOS with latest kernel (>= 6.13)
- Mesa >= 26.0 (Vulkan 1.4, mesh shaders, ray tracing)

## Quick Start

```bash
# Build waybox
cargo build --release

# Create a VM with Firefox
waybox create --name firefox-vm --system firefox --memory 4096 --vcpus 4

# Start the VM
waybox start firefox-vm

# Launch Firefox — appears as a native window
waybox launch firefox-vm firefox

# Generate desktop shortcuts for all apps
waybox generate-shortcuts firefox-vm
```

## Usage

```
waybox create              Create a new application VM
waybox start <name>        Start a VM
waybox stop <name>         Stop a VM gracefully
waybox destroy <name>      Destroy a VM and delete its config
waybox list                List all configured VMs
waybox launch <name> <cmd> Launch an app from a VM via waypipe
waybox console <name>      Connect to VM serial console
waybox passwords           Show stored VM passwords
waybox generate-shortcuts  Generate .desktop files for a VM's apps
```

### Create options

```
--name <name>              VM name (required)
--system <pkg>...          NixOS system packages
--flatpak <pkg>...         Flathub application IDs
--memory <MB>              RAM in MB (default: 2048)
--vcpus <N>                vCPUs (default: 2)
--disk <GB>                Disk size in GB (default: 20)
--usb <vendor:product>...  USB devices to pass through
--share <host:guest>...    Shared folders (host_path:guest_path)
--share-readonly           Mount shared folders read-only
--no-network               Airgapped mode (vsock only)
--headless                 No display, serial console only
-y, --yes                  Skip confirmation prompts
```

## Building

```bash
# Requires libvirt-dev for the virt crate
cargo build --release
```

## Configuration

VM configs are stored as TOML files:

| Path | Contents |
|------|----------|
| `~/.config/waybox/<name>.toml` | VM configuration |
| `~/.config/waybox/vm-passwords.toml` | Credentials |
| `~/.local/share/waybox/images/<name>.qcow2` | VM disk images |
| `~/.local/share/applications/waybox-<name>-*.desktop` | App launchers |

## License

MIT
