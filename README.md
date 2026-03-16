# waybox

Isolated application VMs with native Wayland integration.

waybox creates lightweight NixOS VMs where each VM serves a single application. Apps appear as regular windows on your host Wayland desktop via [waypipe](https://gitlab.freedesktop.org/mstoeckl/waypipe) over vsock — no remote desktop, no compositor in the guest, just native windows.

## Features

- **One app, one VM** — full isolation between applications
- **Native Wayland windows** — apps appear on your host desktop via waypipe
- **Venus Vulkan GPU acceleration** — near-native 3D graphics via virtio-gpu
- **Audio forwarding** — PipeWire audio over vsock
- **USB passthrough** — permanent or hot-plug device passthrough
- **Shared folders** — virtiofs mounts between host and guest
- **Declarative guests** — NixOS configuration, no manual setup
- **Desktop integration** — `.desktop` files generated for each app

## Architecture

```
Host (Arch/Linux)                       Guest (NixOS VM)
┌───────────────────────┐              ┌──────────────────────┐
│  waybox               │              │                      │
│                       │              │  waypipe server      │
│  waypipe client ◄─────┼── vsock ────►│       │              │
│                       │              │   application        │
│  PipeWire ◄───────────┼── vsock ────►│  audio bridge        │
│  libvirt (virsh) ─────┼── qemu ────►│  Venus Vulkan        │
│  USB passthrough ─────┼── libvirt ──►│  (virtio-gpu)        │
└───────────────────────┘              └──────────────────────┘
```

All transport uses vsock — no SSH tunnels, no TCP ports.

## Requirements

### Host
- Linux kernel >= 6.13 (blob/TTM fix for Venus)
- QEMU >= 9.2
- libvirt + virt-install
- Nix (for nixos-generators)
- waypipe
- socat
- Wayland compositor
- GPU: AMD (RADV) or Intel (ANV). NVIDIA 590+ experimental.
- `CONFIG_UDMABUF` enabled in kernel

### Guest (auto-configured)
- NixOS with latest kernel (>= 6.13)
- Mesa >= 26.0 (Vulkan 1.4, mesh shaders, ray tracing)

## Quick Start

```bash
# Create a VM with Firefox
waybox create --name firefox-vm --system firefox --memory 4096 --vcpus 4

# Start the VM
waybox start firefox-vm

# Launch Firefox — appears as a native window
waybox launch firefox-vm firefox
```

## Usage

```
waybox create     Create a new application VM
waybox start      Start a VM
waybox stop       Stop a VM gracefully
waybox destroy    Destroy a VM and delete its config
waybox list       List all configured VMs
waybox launch     Launch an app from a VM via waypipe
waybox console    Connect to VM serial console
waybox passwords  Show stored VM passwords
waybox usb-attach Hot-attach a USB device
waybox usb-detach Hot-detach a USB device
```

## Building

```bash
go build -o waybox .
```

## License

MIT
