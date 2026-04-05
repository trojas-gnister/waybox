use crate::config::passwords::{self, PasswordStore};
use crate::config::WayboxConfig;
use crate::display::desktop;
use crate::display::DisplaySession;
use crate::error::{Result, WayboxError};
use crate::libvirt::domain::generate_domain_xml;
use crate::libvirt::LibvirtConnection;
use crate::nixos;

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
        println!(
            "VM '{}' is running. Launch apps with: waybox launch {} <command>",
            name, name
        );
        std::mem::forget(session); // Detach (processes continue in background)
    } else {
        println!(
            "VM '{}' started (headless). Connect with: waybox console {}",
            name, name
        );
    }
    Ok(())
}

pub fn stop_vm(name: &str) -> Result<()> {
    let conn = LibvirtConnection::new()?;
    conn.stop_vm(name)?;
    println!("VM '{}' is shutting down", name);
    Ok(())
}

pub fn destroy_vm(name: &str) -> Result<()> {
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
