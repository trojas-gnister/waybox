// Password storage and retrieval for VM credentials

use crate::error::{Result, WayboxError};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::path::PathBuf;

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct PasswordStore {
    #[serde(default)]
    pub passwords: HashMap<String, String>,
}

/// Generate a cryptographically random alphanumeric password
pub fn generate_password(length: usize) -> String {
    use rand::Rng;
    rand::thread_rng()
        .sample_iter(&rand::distributions::Alphanumeric)
        .take(length)
        .map(char::from)
        .collect()
}

impl PasswordStore {
    pub fn path() -> Result<PathBuf> {
        Ok(crate::config::WayboxConfig::config_dir()?.join("vm-passwords.toml"))
    }

    pub fn load() -> Result<Self> {
        let path = Self::path()?;
        if !path.exists() {
            return Ok(Self::default());
        }
        let content = std::fs::read_to_string(&path).map_err(|e| WayboxError::Io {
            context: format!("reading password store {:?}", path),
            source: e,
        })?;
        let store: Self = toml::from_str(&content)?;
        Ok(store)
    }

    pub fn save(&self) -> Result<()> {
        let path = Self::path()?;
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent).map_err(|e| WayboxError::Io {
                context: format!("creating password store dir {:?}", parent),
                source: e,
            })?;
        }
        let content = toml::to_string_pretty(self)?;
        std::fs::write(&path, content).map_err(|e| WayboxError::Io {
            context: format!("writing password store {:?}", path),
            source: e,
        })?;
        Ok(())
    }

    pub fn set(&mut self, vm_name: &str, password: &str) {
        self.passwords.insert(vm_name.to_string(), password.to_string());
    }

    pub fn get(&self, vm_name: &str) -> Option<&str> {
        self.passwords.get(vm_name).map(|s| s.as_str())
    }

    pub fn remove(&mut self, vm_name: &str) {
        self.passwords.remove(vm_name);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_generate_password_length() {
        let pw = generate_password(16);
        assert_eq!(pw.len(), 16);
    }

    #[test]
    fn test_generate_password_is_alphanumeric() {
        let pw = generate_password(100);
        assert!(pw.chars().all(|c| c.is_ascii_alphanumeric()));
    }

    #[test]
    fn test_generate_password_uniqueness() {
        let pw1 = generate_password(32);
        let pw2 = generate_password(32);
        assert_ne!(pw1, pw2);
    }

    #[test]
    fn test_password_store_set_get() {
        let mut store = PasswordStore::default();
        store.set("test-vm", "secret123");
        assert_eq!(store.get("test-vm"), Some("secret123"));
        assert_eq!(store.get("nonexistent"), None);
    }

    #[test]
    fn test_password_store_remove() {
        let mut store = PasswordStore::default();
        store.set("test-vm", "secret123");
        store.remove("test-vm");
        assert_eq!(store.get("test-vm"), None);
    }

    #[test]
    fn test_password_store_toml_roundtrip() {
        let mut store = PasswordStore::default();
        store.set("vm1", "pass1");
        store.set("vm2", "pass2");
        let serialized = toml::to_string_pretty(&store).unwrap();
        let deserialized: PasswordStore = toml::from_str(&serialized).unwrap();
        assert_eq!(deserialized.get("vm1"), Some("pass1"));
        assert_eq!(deserialized.get("vm2"), Some("pass2"));
    }
}
