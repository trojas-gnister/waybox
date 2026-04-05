pub mod audio;
pub mod desktop;
pub mod waypipe;

use crate::error::Result;

pub struct DisplaySession {
    pub vm_name: String,
    pub vsock_cid: u32,
    waypipe_process: Option<std::process::Child>,
    audio_process: Option<std::process::Child>,
}

impl DisplaySession {
    pub fn new(vm_name: &str, vsock_cid: u32) -> Self {
        Self {
            vm_name: vm_name.to_string(),
            vsock_cid,
            waypipe_process: None,
            audio_process: None,
        }
    }

    pub fn start(&mut self) -> Result<()> {
        self.waypipe_process = Some(waypipe::start_client(self.vsock_cid)?);
        self.audio_process = Some(audio::start_bridge(self.vsock_cid)?);
        log::info!(
            "Display session started for {} (CID {})",
            self.vm_name,
            self.vsock_cid
        );
        Ok(())
    }

    pub fn stop(&mut self) {
        if let Some(ref mut child) = self.waypipe_process {
            let _ = child.kill();
            let _ = child.wait();
        }
        self.waypipe_process = None;
        if let Some(ref mut child) = self.audio_process {
            let _ = child.kill();
            let _ = child.wait();
        }
        self.audio_process = None;
        log::info!("Display session stopped for {}", self.vm_name);
    }

    pub fn launch_app(&self, command: &str) -> Result<()> {
        waypipe::launch_in_guest(self.vsock_cid, command)
    }

    pub fn list_apps(&self) -> Result<Vec<desktop::GuestApp>> {
        waypipe::list_guest_apps(self.vsock_cid)
    }

    /// Return the OS process IDs of the waypipe and audio bridge processes,
    /// if they were started.  Used by the provisioner to write a PID file
    /// before detaching the session via `std::mem::forget`.
    pub fn process_ids(&self) -> (Option<u32>, Option<u32>) {
        (
            self.waypipe_process.as_ref().map(|c| c.id()),
            self.audio_process.as_ref().map(|c| c.id()),
        )
    }
}

impl Drop for DisplaySession {
    fn drop(&mut self) {
        self.stop();
    }
}
