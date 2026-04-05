pub mod passwords;
pub mod validation;

use serde::{Deserialize, Serialize};

use crate::error::{IoContext, Result, WayboxError};

// ── Constants ──────────────────────────────────────────────────────────────

pub const VSOCK_DISPLAY_PORT: u32 = 5000;
pub const VSOCK_AUDIO_PORT: u32 = 5001;
pub const VSOCK_CONTROL_PORT: u32 = 5002;
pub const VSOCK_CID_MIN: u32 = 3;

// ── Supporting types ───────────────────────────────────────────────────────

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub enum NetworkMode {
    Nat,
    Airgapped,
}

impl Default for NetworkMode {
    fn default() -> Self {
        NetworkMode::Nat
    }
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct UsbDevice {
    pub vendor: String,
    pub product: String,
}

impl UsbDevice {
    pub fn from_id(id: &str) -> Result<Self> {
        validation::validate_usb_id(id)?;
        let (vendor, product) = id.split_once(':').unwrap();
        Ok(Self {
            vendor: vendor.to_lowercase(),
            product: product.to_lowercase(),
        })
    }

    pub fn id(&self) -> String {
        format!("{}:{}", self.vendor, self.product)
    }
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct SharedFolder {
    pub host_path: String,
    pub guest_path: String,
}

// ── WayboxConfig ───────────────────────────────────────────────────────────

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct WayboxConfig {
    pub name: String,
    #[serde(default = "default_memory")]
    pub memory_mb: u32,
    #[serde(default = "default_vcpus")]
    pub vcpus: u32,
    #[serde(default = "default_disk")]
    pub disk_gb: u32,
    #[serde(default)]
    pub system_packages: Vec<String>,
    #[serde(default)]
    pub flatpak_packages: Vec<String>,
    #[serde(default)]
    pub usb_devices: Vec<UsbDevice>,
    #[serde(default)]
    pub shared_folders: Vec<SharedFolder>,
    #[serde(default)]
    pub network_mode: NetworkMode,
    pub vsock_cid: u32,
    #[serde(default)]
    pub headless: bool,
    #[serde(default)]
    pub share_readonly: bool,
}

fn default_memory() -> u32 {
    2048
}
fn default_vcpus() -> u32 {
    2
}
fn default_disk() -> u32 {
    20
}

// ── Directory helpers ──────────────────────────────────────────────────────

/// Return `~/.config/waybox` or `~/.local/share/waybox` depending on `kind`.
/// `kind` must be either `"config"` or `"data"`.
fn dirs_path(kind: &str) -> Result<std::path::PathBuf> {
    let home = std::env::var("HOME").map_err(|_| WayboxError::Io {
        context: "reading $HOME".to_string(),
        source: std::io::Error::new(std::io::ErrorKind::NotFound, "$HOME not set"),
    })?;
    let path = match kind {
        "config" => std::path::PathBuf::from(&home)
            .join(".config")
            .join("waybox"),
        "data" => std::path::PathBuf::from(&home)
            .join(".local")
            .join("share")
            .join("waybox"),
        other => {
            return Err(WayboxError::Io {
                context: format!("dirs_path called with unknown kind '{other}'"),
                source: std::io::Error::new(std::io::ErrorKind::InvalidInput, "unknown kind"),
            })
        }
    };
    Ok(path)
}

// ── WayboxConfig methods ───────────────────────────────────────────────────

impl WayboxConfig {
    /// Validate the name and all USB device IDs stored in this config.
    pub fn validate(&self) -> Result<()> {
        validation::validate_vm_name(&self.name)?;
        for dev in &self.usb_devices {
            let id = dev.id();
            validation::validate_usb_id(&id)?;
        }
        Ok(())
    }

    /// `~/.config/waybox/<name>.toml`
    pub fn config_path(&self) -> Result<std::path::PathBuf> {
        Ok(Self::config_dir()?.join(format!("{}.toml", self.name)))
    }

    /// `~/.config/waybox/`
    pub fn config_dir() -> Result<std::path::PathBuf> {
        dirs_path("config")
    }

    /// `~/.local/share/waybox/images/<name>.qcow2`
    pub fn image_path(&self) -> Result<std::path::PathBuf> {
        Ok(Self::images_dir()?.join(format!("{}.qcow2", self.name)))
    }

    /// `~/.local/share/waybox/images/`
    pub fn images_dir() -> Result<std::path::PathBuf> {
        Ok(dirs_path("data")?.join("images"))
    }

    /// Serialize to TOML and write to `config_path`.
    /// Creates parent directories if they don't exist.
    pub fn save(&self) -> Result<()> {
        let path = self.config_path()?;
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)
                .io_context("creating config directory")?;
        }
        let toml_str = toml::to_string_pretty(self)?;
        std::fs::write(&path, toml_str)
            .io_context(&format!("writing config to {}", path.display()))?;
        Ok(())
    }

    /// Read and deserialize a config by VM name.
    pub fn load(name: &str) -> Result<Self> {
        let config_dir = Self::config_dir()?;
        let path = config_dir.join(format!("{name}.toml"));
        if !path.exists() {
            return Err(WayboxError::ConfigNotFound(name.to_string()));
        }
        let contents = std::fs::read_to_string(&path)
            .io_context(&format!("reading config from {}", path.display()))?;
        let config: WayboxConfig = toml::from_str(&contents)?;
        Ok(config)
    }

    /// Return all VM configs found in the config directory.
    /// Skips `vm-passwords.toml` and any files that fail to parse.
    pub fn list_all() -> Result<Vec<Self>> {
        let config_dir = Self::config_dir()?;
        if !config_dir.exists() {
            return Ok(vec![]);
        }
        let mut configs = Vec::new();
        let entries = std::fs::read_dir(&config_dir)
            .io_context(&format!("reading config directory {}", config_dir.display()))?;
        for entry in entries {
            let entry = entry.io_context("reading directory entry")?;
            let path = entry.path();
            if path.extension().and_then(|e| e.to_str()) != Some("toml") {
                continue;
            }
            if path.file_name().and_then(|n| n.to_str()) == Some("vm-passwords.toml") {
                continue;
            }
            let contents = match std::fs::read_to_string(&path) {
                Ok(c) => c,
                Err(_) => continue,
            };
            if let Ok(config) = toml::from_str::<WayboxConfig>(&contents) {
                configs.push(config);
            }
        }
        Ok(configs)
    }

    /// Remove this VM's config file.
    pub fn delete(&self) -> Result<()> {
        let path = self.config_path()?;
        if !path.exists() {
            return Err(WayboxError::ConfigNotFound(self.name.clone()));
        }
        std::fs::remove_file(&path)
            .io_context(&format!("deleting config at {}", path.display()))?;
        Ok(())
    }

    /// Scan existing configs and return the next free CID starting from
    /// `VSOCK_CID_MIN` (3).
    pub fn next_available_cid() -> Result<u32> {
        let configs = Self::list_all()?;
        let used: std::collections::HashSet<u32> = configs.iter().map(|c| c.vsock_cid).collect();
        let mut cid = VSOCK_CID_MIN;
        while used.contains(&cid) {
            cid += 1;
        }
        Ok(cid)
    }
}

// ── Tests ──────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use serial_test::serial;

    fn test_config() -> WayboxConfig {
        WayboxConfig {
            name: "test-vm".to_string(),
            memory_mb: 2048,
            vcpus: 2,
            disk_gb: 20,
            system_packages: vec!["firefox".to_string()],
            flatpak_packages: vec![],
            usb_devices: vec![],
            shared_folders: vec![],
            network_mode: NetworkMode::Nat,
            vsock_cid: 3,
            headless: false,
            share_readonly: false,
        }
    }

    #[test]
    fn test_config_defaults_via_serde() {
        let toml_str = r#"
            name = "minimal"
            vsock_cid = 3
        "#;
        let config: WayboxConfig = toml::from_str(toml_str).unwrap();
        assert_eq!(config.memory_mb, 2048);
        assert_eq!(config.vcpus, 2);
        assert_eq!(config.disk_gb, 20);
        assert_eq!(config.network_mode, NetworkMode::Nat);
        assert!(!config.headless);
    }

    #[test]
    fn test_config_toml_roundtrip() {
        let config = test_config();
        let serialized = toml::to_string_pretty(&config).unwrap();
        let deserialized: WayboxConfig = toml::from_str(&serialized).unwrap();
        assert_eq!(config, deserialized);
    }

    #[test]
    fn test_usb_device_from_id() {
        let dev = UsbDevice::from_id("046D:C52B").unwrap();
        assert_eq!(dev.vendor, "046d");
        assert_eq!(dev.product, "c52b");
        assert_eq!(dev.id(), "046d:c52b");
    }

    #[test]
    fn test_config_validation_passes() {
        let config = test_config();
        assert!(config.validate().is_ok());
    }

    #[test]
    fn test_config_validation_rejects_bad_name() {
        let mut config = test_config();
        config.name = "123bad".to_string();
        assert!(config.validate().is_err());
    }

    #[test]
    #[serial]
    fn test_save_load_delete() {
        // Use a temp dir as HOME so we don't pollute the real config dir.
        let tmp = tempfile::tempdir().unwrap();
        std::env::set_var("HOME", tmp.path());

        let config = test_config();
        config.save().unwrap();

        let loaded = WayboxConfig::load("test-vm").unwrap();
        assert_eq!(config, loaded);

        // list_all should find it
        let all = WayboxConfig::list_all().unwrap();
        assert_eq!(all.len(), 1);
        assert_eq!(all[0].name, "test-vm");

        config.delete().unwrap();
        assert!(WayboxConfig::load("test-vm").is_err());
    }

    #[test]
    #[serial]
    fn test_next_available_cid_empty() {
        let tmp = tempfile::tempdir().unwrap();
        std::env::set_var("HOME", tmp.path());
        let cid = WayboxConfig::next_available_cid().unwrap();
        assert_eq!(cid, VSOCK_CID_MIN);
    }

    #[test]
    #[serial]
    fn test_next_available_cid_with_existing() {
        let tmp = tempfile::tempdir().unwrap();
        std::env::set_var("HOME", tmp.path());

        let mut c1 = test_config();
        c1.vsock_cid = 3;
        c1.save().unwrap();

        let mut c2 = test_config();
        c2.name = "other-vm".to_string();
        c2.vsock_cid = 4;
        c2.save().unwrap();

        let cid = WayboxConfig::next_available_cid().unwrap();
        assert_eq!(cid, 5);
    }

    #[test]
    #[serial]
    fn test_config_not_found() {
        let tmp = tempfile::tempdir().unwrap();
        std::env::set_var("HOME", tmp.path());
        let result = WayboxConfig::load("nonexistent");
        assert!(matches!(result, Err(WayboxError::ConfigNotFound(_))));
    }
}
