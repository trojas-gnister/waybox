#[derive(Debug, thiserror::Error)]
pub enum WayboxError {
    // Config
    #[error("Invalid VM name '{name}': {reason}")]
    InvalidName { name: String, reason: String },

    #[error("Invalid USB device ID '{id}': expected vendor:product (e.g., 046d:c52b)")]
    InvalidUsbId { id: String },

    #[error("Invalid share path '{path}': {reason}")]
    InvalidSharePath { path: String, reason: String },

    #[error("Config not found for VM '{0}'")]
    ConfigNotFound(String),

    #[error("VM '{0}' already exists")]
    VmAlreadyExists(String),

    // Libvirt
    #[error("Libvirt error: {0}")]
    Libvirt(#[from] virt::error::Error),

    #[error("VM '{name}' not found in libvirt")]
    VmNotFound { name: String },

    #[error("VM '{name}' is already {state}")]
    VmWrongState { name: String, state: String },

    // NixOS
    #[error("NixOS image build failed: {0}")]
    ImageBuild(String),

    #[error("Prerequisite not found: {tool}. {hint}")]
    PrerequisiteNotFound { tool: String, hint: String },

    // Display
    #[error("waypipe session failed: {0}")]
    Waypipe(String),

    #[error("Audio bridge failed: {0}")]
    Audio(String),

    #[error("Guest not responding on vsock CID {cid}")]
    VsockTimeout { cid: u32 },

    // USB
    #[error("USB device {id} not found on host")]
    UsbNotFound { id: String },

    // Generic
    #[error("IO error ({context}): {source}")]
    Io {
        context: String,
        source: std::io::Error,
    },

    #[error("TOML serialization error: {0}")]
    TomlSerialize(#[from] toml::ser::Error),

    #[error("TOML deserialization error: {0}")]
    TomlDeserialize(#[from] toml::de::Error),

    #[error("Template rendering error: {0}")]
    Template(#[from] askama::Error),
}

pub type Result<T> = std::result::Result<T, WayboxError>;

/// Convenience trait for adding IO error context
pub trait IoContext<T> {
    fn io_context(self, context: &str) -> Result<T>;
}

impl<T> IoContext<T> for std::result::Result<T, std::io::Error> {
    fn io_context(self, context: &str) -> Result<T> {
        self.map_err(|source| WayboxError::Io {
            context: context.to_string(),
            source,
        })
    }
}
