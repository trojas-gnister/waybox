pub mod image;
pub mod packages;

use askama::Template;
use crate::config::{SharedFolder, WayboxConfig, VSOCK_AUDIO_PORT, VSOCK_CONTROL_PORT, VSOCK_DISPLAY_PORT};
use crate::error::Result;

#[derive(Template)]
#[template(path = "base.nix.tmpl", escape = "none")]
struct BaseTemplate<'a> {
    name: &'a str,
    password: &'a str,
    system_packages: &'a [String],
}

#[derive(Template)]
#[template(path = "waypipe.nix.tmpl", escape = "none")]
struct WaypipeTemplate {
    display_port: u32,
    control_port: u32,
}

#[derive(Template)]
#[template(path = "audio.nix.tmpl", escape = "none")]
struct AudioTemplate {
    audio_port: u32,
}

#[derive(Template)]
#[template(path = "venus.nix.tmpl", escape = "none")]
struct VenusTemplate;

#[derive(Template)]
#[template(path = "virtiofs.nix.tmpl", escape = "none")]
struct VirtiofsTemplate<'a> {
    shared_folders: &'a [SharedFolder],
    readonly: bool,
}

#[derive(Template)]
#[template(path = "flatpak.nix.tmpl", escape = "none")]
struct FlatpakTemplate<'a> {
    flatpak_packages: &'a [String],
}

pub struct RenderedNixosConfig {
    pub base: String,
    pub waypipe: Option<String>,
    pub audio: Option<String>,
    pub venus: Option<String>,
    pub virtiofs: Option<String>,
    pub flatpak: Option<String>,
}

pub fn render_nixos_config(config: &WayboxConfig, password: &str) -> Result<RenderedNixosConfig> {
    let base = BaseTemplate {
        name: &config.name,
        password,
        system_packages: &config.system_packages,
    }.render()?;

    let waypipe = if !config.headless {
        Some(WaypipeTemplate { display_port: VSOCK_DISPLAY_PORT, control_port: VSOCK_CONTROL_PORT }.render()?)
    } else { None };

    let audio = if !config.headless {
        Some(AudioTemplate { audio_port: VSOCK_AUDIO_PORT }.render()?)
    } else { None };

    let venus = if !config.headless {
        Some(VenusTemplate.render()?)
    } else { None };

    let virtiofs = if !config.shared_folders.is_empty() {
        Some(VirtiofsTemplate { shared_folders: &config.shared_folders, readonly: config.share_readonly }.render()?)
    } else { None };

    let flatpak = if !config.flatpak_packages.is_empty() {
        Some(FlatpakTemplate { flatpak_packages: &config.flatpak_packages }.render()?)
    } else { None };

    Ok(RenderedNixosConfig { base, waypipe, audio, venus, virtiofs, flatpak })
}

pub fn generate_configuration_nix(rendered: &RenderedNixosConfig) -> String {
    let mut imports = vec!["./base.nix".to_string()];
    if rendered.waypipe.is_some() { imports.push("./waypipe.nix".to_string()); }
    if rendered.audio.is_some() { imports.push("./audio.nix".to_string()); }
    if rendered.venus.is_some() { imports.push("./venus.nix".to_string()); }
    if rendered.virtiofs.is_some() { imports.push("./virtiofs.nix".to_string()); }
    if rendered.flatpak.is_some() { imports.push("./flatpak.nix".to_string()); }

    let import_lines: String = imports.iter().map(|p| format!("    {p}")).collect::<Vec<_>>().join("\n");
    format!("{{ config, pkgs, ... }}:\n{{\n  imports = [\n{import_lines}\n  ];\n}}\n")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::{NetworkMode, SharedFolder, WayboxConfig};

    fn test_config() -> WayboxConfig {
        WayboxConfig {
            name: "test-vm".to_string(),
            memory_mb: 2048, vcpus: 2, disk_gb: 20,
            system_packages: vec!["firefox".to_string(), "git".to_string()],
            flatpak_packages: vec![],
            usb_devices: vec![], shared_folders: vec![],
            network_mode: NetworkMode::Nat,
            vsock_cid: 3, headless: false, share_readonly: false,
        }
    }

    #[test]
    fn test_base_template_renders_name_and_packages() {
        let rendered = render_nixos_config(&test_config(), "testpass123").unwrap();
        assert!(rendered.base.contains("networking.hostName = \"test-vm\""));
        assert!(rendered.base.contains("initialPassword = \"testpass123\""));
        assert!(rendered.base.contains("firefox"));
        assert!(rendered.base.contains("git"));
    }

    #[test]
    fn test_waypipe_template_renders_ports() {
        let rendered = render_nixos_config(&test_config(), "pw").unwrap();
        let waypipe = rendered.waypipe.unwrap();
        assert!(waypipe.contains("VSOCK-LISTEN:5002"));
        assert!(waypipe.contains("2:5000"));
    }

    #[test]
    fn test_headless_skips_display_templates() {
        let mut config = test_config();
        config.headless = true;
        let rendered = render_nixos_config(&config, "pw").unwrap();
        assert!(rendered.waypipe.is_none());
        assert!(rendered.audio.is_none());
        assert!(rendered.venus.is_none());
    }

    #[test]
    fn test_virtiofs_template_renders_shares() {
        let mut config = test_config();
        config.shared_folders.push(SharedFolder { host_path: "/home/user/docs".to_string(), guest_path: "/mnt/docs".to_string() });
        let rendered = render_nixos_config(&config, "pw").unwrap();
        let virtiofs = rendered.virtiofs.unwrap();
        assert!(virtiofs.contains("/mnt/docs"));
        assert!(virtiofs.contains("fs0"));
        assert!(virtiofs.contains("virtiofs"));
    }

    #[test]
    fn test_virtiofs_readonly() {
        let mut config = test_config();
        config.shared_folders.push(SharedFolder { host_path: "/tmp".to_string(), guest_path: "/mnt/tmp".to_string() });
        config.share_readonly = true;
        let rendered = render_nixos_config(&config, "pw").unwrap();
        let virtiofs = rendered.virtiofs.unwrap();
        assert!(virtiofs.contains("\"ro\""));
    }

    #[test]
    fn test_no_shares_skips_virtiofs() {
        let rendered = render_nixos_config(&test_config(), "pw").unwrap();
        assert!(rendered.virtiofs.is_none());
    }

    #[test]
    fn test_flatpak_template_renders_packages() {
        let mut config = test_config();
        config.flatpak_packages.push("org.mozilla.firefox".to_string());
        let rendered = render_nixos_config(&config, "pw").unwrap();
        let flatpak = rendered.flatpak.unwrap();
        assert!(flatpak.contains("org.mozilla.firefox"));
        assert!(flatpak.contains("flathub"));
    }

    #[test]
    fn test_no_flatpaks_skips_template() {
        let rendered = render_nixos_config(&test_config(), "pw").unwrap();
        assert!(rendered.flatpak.is_none());
    }

    #[test]
    fn test_configuration_nix_imports_all_modules() {
        let rendered = render_nixos_config(&test_config(), "pw").unwrap();
        let config_nix = generate_configuration_nix(&rendered);
        assert!(config_nix.contains("./base.nix"));
        assert!(config_nix.contains("./waypipe.nix"));
        assert!(config_nix.contains("./audio.nix"));
        assert!(config_nix.contains("./venus.nix"));
        assert!(!config_nix.contains("./virtiofs.nix"));
        assert!(!config_nix.contains("./flatpak.nix"));
    }

    #[test]
    fn test_configuration_nix_headless_minimal() {
        let mut config = test_config();
        config.headless = true;
        let rendered = render_nixos_config(&config, "pw").unwrap();
        let config_nix = generate_configuration_nix(&rendered);
        assert!(config_nix.contains("./base.nix"));
        assert!(!config_nix.contains("./waypipe.nix"));
        assert!(!config_nix.contains("./venus.nix"));
    }
}
