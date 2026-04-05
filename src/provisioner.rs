use crate::config::passwords::{self, PasswordStore};
use crate::config::WayboxConfig;
use crate::display::desktop;
use crate::display::DisplaySession;
use crate::error::{IoContext, Result, WayboxError};
use crate::libvirt::domain::generate_domain_xml;
use crate::libvirt::LibvirtConnection;
use crate::nixos;

// ── PID file helpers ───────────────────────────────────────────────────────

#[derive(serde::Serialize, serde::Deserialize)]
struct SessionPids {
    waypipe_pid: u32,
    audio_pid: u32,
}

fn pid_file_path(vm_name: &str) -> Result<std::path::PathBuf> {
    let home = std::env::var("HOME").map_err(|_| WayboxError::Io {
        context: "reading $HOME".to_string(),
        source: std::io::Error::new(std::io::ErrorKind::NotFound, "$HOME not set"),
    })?;
    Ok(std::path::PathBuf::from(home)
        .join(".local")
        .join("share")
        .join("waybox")
        .join(format!("{vm_name}.pid")))
}

fn write_pid_file(vm_name: &str, waypipe_pid: u32, audio_pid: u32) -> Result<()> {
    let path = pid_file_path(vm_name)?;
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent).io_context("creating waybox data directory")?;
    }
    let pids = SessionPids { waypipe_pid, audio_pid };
    let json = serde_json::to_string(&pids).map_err(|e| WayboxError::Io {
        context: "serializing PID file".to_string(),
        source: std::io::Error::other(e.to_string()),
    })?;
    std::fs::write(&path, json)
        .io_context(&format!("writing PID file {}", path.display()))?;
    Ok(())
}

fn kill_session_processes(vm_name: &str) -> Result<()> {
    let path = pid_file_path(vm_name)?;
    if !path.exists() {
        return Ok(());
    }
    let json = std::fs::read_to_string(&path)
        .io_context(&format!("reading PID file {}", path.display()))?;
    let pids: SessionPids = serde_json::from_str(&json).map_err(|e| WayboxError::Io {
        context: "parsing PID file".to_string(),
        source: std::io::Error::new(std::io::ErrorKind::InvalidData, e.to_string()),
    })?;
    // Best-effort: send SIGTERM to both processes.
    for pid in [pids.waypipe_pid, pids.audio_pid] {
        unsafe {
            libc::kill(pid as libc::pid_t, libc::SIGTERM);
        }
    }
    std::fs::remove_file(&path).ok();
    Ok(())
}

pub fn check_prerequisites() -> Result<()> {
    nixos::image::check_prerequisites()?;

    std::process::Command::new("which")
        .arg("waypipe")
        .output()
        .ok()
        .filter(|o| o.status.success())
        .ok_or_else(|| WayboxError::PrerequisiteNotFound {
            tool: "waypipe".to_string(),
            hint: "Install waypipe for Wayland forwarding".to_string(),
        })?;

    std::process::Command::new("which")
        .arg("socat")
        .output()
        .ok()
        .filter(|o| o.status.success())
        .ok_or_else(|| WayboxError::PrerequisiteNotFound {
            tool: "socat".to_string(),
            hint: "Install socat for vsock bridging".to_string(),
        })?;

    let vsock_loaded = std::path::Path::new("/dev/vsock").exists()
        || std::process::Command::new("lsmod")
            .output()
            .map(|o| String::from_utf8_lossy(&o.stdout).contains("vhost_vsock"))
            .unwrap_or(false);

    if !vsock_loaded {
        return Err(WayboxError::PrerequisiteNotFound {
            tool: "vhost_vsock".to_string(),
            hint: "Load the kernel module: sudo modprobe vhost_vsock".to_string(),
        });
    }

    // Verify that libvirtd is reachable.
    LibvirtConnection::new().map_err(|_| WayboxError::PrerequisiteNotFound {
        tool: "libvirtd".to_string(),
        hint: "Ensure libvirtd is running: sudo systemctl start libvirtd".to_string(),
    })?;

    Ok(())
}

pub fn create_vm(config: &WayboxConfig) -> Result<()> {
    log::info!("Creating VM '{}'", config.name);
    check_prerequisites()?;

    let config_path = config.config_path()?;
    if config_path.exists() {
        return Err(WayboxError::VmAlreadyExists(config.name.clone()));
    }

    let password = passwords::generate_password(16);
    let mut store = PasswordStore::load()?;
    store.set(&config.name, &password);
    store.save()?;

    config.save()?;

    println!("Rendering NixOS configuration...");
    let rendered = nixos::render_nixos_config(config, &password)?;

    println!("Building NixOS image (this may take a while)...");
    let image_path = nixos::image::build_image(config, &rendered)?;
    println!("Image built: {}", image_path.display());

    let xml = generate_domain_xml(config);
    let conn = LibvirtConnection::new()?;
    conn.define_vm(&xml)?;

    println!("VM '{}' created successfully!", config.name);
    println!("  vsock CID: {}", config.vsock_cid);
    println!("  Password:  {}", password);
    println!("\nStart with: waybox start {}", config.name);
    Ok(())
}

pub fn start_vm(name: &str) -> Result<()> {
    let config = WayboxConfig::load(name)?;
    let conn = LibvirtConnection::new()?;
    conn.start_vm(name)?;

    if !config.headless {
        let mut session = DisplaySession::new(name, config.vsock_cid);
        println!("Starting display session...");
        std::thread::sleep(std::time::Duration::from_secs(5));
        session.start()?;

        // Record PIDs so stop_vm can kill the background processes later.
        let (waypipe_pid, audio_pid) = session.process_ids();
        write_pid_file(name, waypipe_pid.unwrap_or(0), audio_pid.unwrap_or(0))?;

        println!(
            "VM '{}' is running. Launch apps with: waybox launch {} <command>",
            name, name
        );
        std::mem::forget(session); // Detach — processes continue in background; PIDs recorded above
    } else {
        println!(
            "VM '{}' started (headless). Connect with: waybox console {}",
            name, name
        );
    }
    Ok(())
}

pub fn stop_vm(name: &str) -> Result<()> {
    // Kill waypipe/audio processes from the PID file before shutting down the VM.
    kill_session_processes(name)?;
    let conn = LibvirtConnection::new()?;
    conn.stop_vm(name)?;
    println!("VM '{}' is shutting down", name);
    Ok(())
}

pub fn destroy_vm(name: &str) -> Result<()> {
    // Kill any lingering display session processes and clean up PID file.
    kill_session_processes(name)?;

    let conn = LibvirtConnection::new()?;
    match conn.destroy_vm(name) {
        Ok(_) => log::info!("VM '{}' removed from libvirt", name),
        Err(e) => log::warn!("Could not remove '{}' from libvirt: {}", name, e),
    }

    let config = WayboxConfig::load(name).ok();
    if let Some(ref config) = config {
        if let Ok(image) = config.image_path() {
            if image.exists() {
                std::fs::remove_file(&image).ok();
            }
        }
    }

    desktop::remove_desktop_files(name)?;

    let mut store = PasswordStore::load()?;
    store.remove(name);
    store.save()?;

    if let Some(config) = config {
        config.delete()?;
    }

    println!("VM '{}' destroyed", name);
    Ok(())
}
