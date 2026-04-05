use crate::error::{Result, WayboxError};

/// Validate a VM name: 1–64 chars, starts with a letter, rest are
/// alphanumeric, hyphens, or underscores.
pub fn validate_vm_name(name: &str) -> Result<()> {
    if name.is_empty() {
        return Err(WayboxError::InvalidName {
            name: name.to_string(),
            reason: "name must not be empty".to_string(),
        });
    }
    if name.len() > 64 {
        return Err(WayboxError::InvalidName {
            name: name.to_string(),
            reason: "name must be 64 characters or fewer".to_string(),
        });
    }
    let mut chars = name.chars();
    let first = chars.next().unwrap();
    if !first.is_ascii_alphabetic() {
        return Err(WayboxError::InvalidName {
            name: name.to_string(),
            reason: "name must start with a letter".to_string(),
        });
    }
    for ch in chars {
        if !ch.is_ascii_alphanumeric() && ch != '-' && ch != '_' {
            return Err(WayboxError::InvalidName {
                name: name.to_string(),
                reason: format!("invalid character '{ch}': only letters, digits, hyphens, and underscores are allowed"),
            });
        }
    }
    Ok(())
}

/// Validate a USB device ID in the form `XXXX:XXXX` (4 hex digits each).
pub fn validate_usb_id(id: &str) -> Result<()> {
    let err = || WayboxError::InvalidUsbId { id: id.to_string() };

    let parts: Vec<&str> = id.split(':').collect();
    if parts.len() != 2 {
        return Err(err());
    }
    let (vendor, product) = (parts[0], parts[1]);
    if vendor.len() != 4 || product.len() != 4 {
        return Err(err());
    }
    if !vendor.chars().all(|c| c.is_ascii_hexdigit()) {
        return Err(err());
    }
    if !product.chars().all(|c| c.is_ascii_hexdigit()) {
        return Err(err());
    }
    Ok(())
}

/// Validate and split a host:guest share path specification.
/// Both paths must be absolute (start with `/`).
/// Returns `(host_path, guest_path)`.
pub fn validate_share_path(share: &str) -> Result<(String, String)> {
    let err = |reason: &str| WayboxError::InvalidSharePath {
        path: share.to_string(),
        reason: reason.to_string(),
    };

    if share.is_empty() {
        return Err(err("share path must not be empty"));
    }

    // Split on the first colon only so absolute paths on either side work.
    let (host, guest) = share.split_once(':').ok_or_else(|| err("expected format host_path:guest_path"))?;

    if !host.starts_with('/') {
        return Err(err("host path must be absolute"));
    }
    if !guest.starts_with('/') {
        return Err(err("guest path must be absolute"));
    }

    Ok((host.to_string(), guest.to_string()))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_valid_vm_names() {
        assert!(validate_vm_name("firefox-vm").is_ok());
        assert!(validate_vm_name("my_vm_01").is_ok());
        assert!(validate_vm_name("a").is_ok());
    }

    #[test]
    fn test_invalid_vm_names() {
        assert!(validate_vm_name("").is_err());
        assert!(validate_vm_name("123start").is_err());
        assert!(validate_vm_name("-leading").is_err());
        assert!(validate_vm_name("has space").is_err());
        assert!(validate_vm_name("no/slashes").is_err());
        assert!(validate_vm_name("no.dots").is_err());
        assert!(validate_vm_name(&"a".repeat(65)).is_err());
    }

    #[test]
    fn test_valid_usb_ids() {
        assert!(validate_usb_id("046d:c52b").is_ok());
        assert!(validate_usb_id("1234:abcd").is_ok());
        assert!(validate_usb_id("ABCD:1234").is_ok());
    }

    #[test]
    fn test_invalid_usb_ids() {
        assert!(validate_usb_id("").is_err());
        assert!(validate_usb_id("046d").is_err());
        assert!(validate_usb_id("046d:").is_err());
        assert!(validate_usb_id("046d:c52bx").is_err());
        assert!(validate_usb_id("zzzz:0000").is_err());
        assert!(validate_usb_id("046d:c52b:extra").is_err());
    }

    #[test]
    fn test_valid_share_paths() {
        let result = validate_share_path("/home/user/docs:/mnt/docs").unwrap();
        assert_eq!(result, ("/home/user/docs".to_string(), "/mnt/docs".to_string()));
    }

    #[test]
    fn test_invalid_share_paths() {
        assert!(validate_share_path("relative:/mnt/docs").is_err());
        assert!(validate_share_path("/home/user:relative").is_err());
        assert!(validate_share_path("no-colon").is_err());
        assert!(validate_share_path("").is_err());
    }
}
