// Audio bridge between host and guest VM

use crate::config::VSOCK_AUDIO_PORT;
use crate::error::{Result, WayboxError};
use std::process::{Child, Command, Stdio};

pub fn start_bridge(_vsock_cid: u32) -> Result<Child> {
    let pulse_socket = get_pulse_socket();
    let child = Command::new("socat")
        .arg(format!("VSOCK-LISTEN:{VSOCK_AUDIO_PORT},reuseaddr,fork"))
        .arg(format!("UNIX-CONNECT:{pulse_socket}"))
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .map_err(|e| WayboxError::Audio(format!("failed to start audio bridge: {e}")))?;
    log::info!("Audio bridge started on vsock port {VSOCK_AUDIO_PORT}");
    Ok(child)
}

fn get_pulse_socket() -> String {
    if let Ok(runtime_dir) = std::env::var("XDG_RUNTIME_DIR") {
        let path = format!("{runtime_dir}/pulse/native");
        if std::path::Path::new(&path).exists() {
            return path;
        }
    }
    let uid_output = Command::new("id").arg("-u").output().ok();
    let uid = uid_output
        .and_then(|o| String::from_utf8(o.stdout).ok())
        .map(|s| s.trim().to_string())
        .unwrap_or_else(|| "1000".to_string());
    format!("/run/user/{uid}/pulse/native")
}
