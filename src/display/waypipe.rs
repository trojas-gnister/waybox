// waypipe session management for Wayland forwarding over vsock

use crate::config::{VSOCK_CONTROL_PORT, VSOCK_DISPLAY_PORT};
use crate::display::desktop::GuestApp;
use crate::error::{Result, WayboxError};
use std::io::Write;
use std::process::{Child, Command, Stdio};

pub fn start_client(_vsock_cid: u32) -> Result<Child> {
    check_waypipe_installed()?;
    let child = Command::new("waypipe")
        .arg("--vsock")
        .arg("-s")
        .arg(format!("{}", VSOCK_DISPLAY_PORT))
        .arg("client")
        .stdout(Stdio::null())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(|e| WayboxError::Waypipe(format!("failed to start waypipe: {e}")))?;
    log::info!("waypipe client started, listening on vsock port {VSOCK_DISPLAY_PORT}");
    Ok(child)
}

pub fn launch_in_guest(vsock_cid: u32, command: &str) -> Result<()> {
    let mut child = Command::new("socat")
        .arg("-")
        .arg(format!("VSOCK-CONNECT:{vsock_cid}:{VSOCK_CONTROL_PORT}"))
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(|e| WayboxError::Waypipe(format!("failed to connect to guest: {e}")))?;

    if let Some(ref mut stdin) = child.stdin {
        writeln!(stdin, "{command}")
            .map_err(|e| WayboxError::Waypipe(format!("failed to send command: {e}")))?;
    }
    drop(child.stdin.take());

    let output = child
        .wait_with_output()
        .map_err(|e| WayboxError::Waypipe(format!("failed to read response: {e}")))?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        return Err(WayboxError::Waypipe(format!("guest launch failed: {stderr}")));
    }
    log::info!("Launched '{command}' in guest CID {vsock_cid}");
    Ok(())
}

pub fn list_guest_apps(vsock_cid: u32) -> Result<Vec<GuestApp>> {
    let output = Command::new("socat")
        .arg("-")
        .arg(format!("VSOCK-CONNECT:{vsock_cid}:{VSOCK_CONTROL_PORT}"))
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .and_then(|mut child| {
            if let Some(ref mut stdin) = child.stdin {
                let _ = writeln!(stdin, "__list_apps__");
            }
            drop(child.stdin.take());
            child.wait_with_output()
        })
        .map_err(|e| WayboxError::Waypipe(format!("failed to query guest apps: {e}")))?;

    let stdout = String::from_utf8_lossy(&output.stdout);
    let apps = stdout
        .lines()
        .filter_map(|line| {
            let parts: Vec<&str> = line.splitn(3, '|').collect();
            if parts.len() >= 2 {
                Some(GuestApp {
                    name: parts[0].to_string(),
                    exec_command: parts[1].to_string(),
                    icon: parts
                        .get(2)
                        .filter(|s| !s.is_empty())
                        .map(|s| s.to_string()),
                })
            } else {
                None
            }
        })
        .collect();
    Ok(apps)
}

fn check_waypipe_installed() -> Result<()> {
    match Command::new("which").arg("waypipe").output() {
        Ok(output) if output.status.success() => Ok(()),
        _ => Err(WayboxError::PrerequisiteNotFound {
            tool: "waypipe".to_string(),
            hint: "Install waypipe for Wayland forwarding".to_string(),
        }),
    }
}
