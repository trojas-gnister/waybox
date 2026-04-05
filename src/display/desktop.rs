// .desktop shortcut generation for VM applications

use crate::error::{Result, WayboxError};
use std::path::PathBuf;

pub struct GuestApp {
    pub name: String,
    pub exec_command: String,
    pub icon: Option<String>,
}

pub fn desktop_file_path(vm_name: &str, app_name: &str) -> PathBuf {
    let home = std::env::var("HOME").unwrap_or_else(|_| "/tmp".to_string());
    PathBuf::from(home)
        .join(".local/share/applications")
        .join(format!("waybox-{vm_name}-{}.desktop", sanitize_filename(app_name)))
}

pub fn generate_desktop_entry(vm_name: &str, app: &GuestApp) -> String {
    let mut entry = format!(
        "[Desktop Entry]\nType=Application\nName={name} ({vm})\nExec=waybox launch {vm} {cmd}\nTerminal=false\nCategories=X-Waybox;\n",
        name = app.name, vm = vm_name, cmd = app.exec_command,
    );
    if let Some(ref icon) = app.icon {
        entry.push_str(&format!("Icon={icon}\n"));
    }
    entry
}

pub fn write_desktop_files(vm_name: &str, apps: &[GuestApp]) -> Result<Vec<PathBuf>> {
    let mut paths = Vec::new();
    for app in apps {
        let path = desktop_file_path(vm_name, &app.name);
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent).map_err(|e| WayboxError::Io {
                context: format!("creating desktop dir {:?}", parent),
                source: e,
            })?;
        }
        let content = generate_desktop_entry(vm_name, app);
        std::fs::write(&path, content).map_err(|e| WayboxError::Io {
            context: format!("writing desktop file {:?}", path),
            source: e,
        })?;
        log::info!("Created desktop file: {:?}", path);
        paths.push(path);
    }
    Ok(paths)
}

pub fn remove_desktop_files(vm_name: &str) -> Result<()> {
    let home = std::env::var("HOME").unwrap_or_else(|_| "/tmp".to_string());
    let apps_dir = PathBuf::from(home).join(".local/share/applications");
    if !apps_dir.exists() {
        return Ok(());
    }
    let prefix = format!("waybox-{vm_name}-");
    let entries = std::fs::read_dir(&apps_dir).map_err(|e| WayboxError::Io {
        context: format!("reading {:?}", apps_dir),
        source: e,
    })?;
    for entry in entries {
        let entry = entry.map_err(|e| WayboxError::Io {
            context: "reading dir entry".to_string(),
            source: e,
        })?;
        if let Some(name) = entry.file_name().to_str() {
            if name.starts_with(&prefix) && name.ends_with(".desktop") {
                std::fs::remove_file(entry.path()).map_err(|e| WayboxError::Io {
                    context: format!("removing {:?}", entry.path()),
                    source: e,
                })?;
            }
        }
    }
    Ok(())
}

fn sanitize_filename(name: &str) -> String {
    name.chars()
        .map(|c| {
            if c.is_ascii_alphanumeric() || c == '-' || c == '_' {
                c
            } else {
                '_'
            }
        })
        .collect::<String>()
        .to_lowercase()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_generate_desktop_entry_basic() {
        let app = GuestApp {
            name: "Firefox".to_string(),
            exec_command: "firefox".to_string(),
            icon: Some("firefox".to_string()),
        };
        let entry = generate_desktop_entry("browser-vm", &app);
        assert!(entry.contains("[Desktop Entry]"));
        assert!(entry.contains("Name=Firefox (browser-vm)"));
        assert!(entry.contains("Exec=waybox launch browser-vm firefox"));
        assert!(entry.contains("Icon=firefox"));
        assert!(entry.contains("Type=Application"));
    }

    #[test]
    fn test_generate_desktop_entry_no_icon() {
        let app = GuestApp {
            name: "htop".to_string(),
            exec_command: "htop".to_string(),
            icon: None,
        };
        let entry = generate_desktop_entry("dev-vm", &app);
        assert!(entry.contains("Name=htop (dev-vm)"));
        assert!(!entry.contains("Icon="));
    }

    #[test]
    fn test_desktop_file_path() {
        let path = desktop_file_path("test-vm", "Firefox");
        assert!(path.to_string_lossy().contains("waybox-test-vm-firefox.desktop"));
    }

    #[test]
    fn test_sanitize_filename() {
        assert_eq!(sanitize_filename("Firefox Web Browser"), "firefox_web_browser");
        assert_eq!(sanitize_filename("vim-full"), "vim-full");
        assert_eq!(sanitize_filename("a/b:c"), "a_b_c");
    }
}
