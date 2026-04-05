pub mod domain;

use crate::error::{Result, WayboxError};
use virt::connect::Connect;
use virt::domain::Domain;

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum DomainState {
    Running,
    Shutoff,
    Paused,
    Shutdown,
    Other(u32),
}

impl From<u32> for DomainState {
    fn from(state: u32) -> Self {
        match state {
            1 => DomainState::Running,
            3 => DomainState::Paused,
            4 => DomainState::Shutdown,
            5 => DomainState::Shutoff,
            other => DomainState::Other(other),
        }
    }
}

impl std::fmt::Display for DomainState {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            DomainState::Running => write!(f, "running"),
            DomainState::Shutoff => write!(f, "shutoff"),
            DomainState::Paused => write!(f, "paused"),
            DomainState::Shutdown => write!(f, "shutting down"),
            DomainState::Other(n) => write!(f, "unknown state {n}"),
        }
    }
}

pub struct DomainInfo {
    pub name: String,
    pub state: DomainState,
}

pub struct LibvirtConnection {
    conn: Connect,
}

impl LibvirtConnection {
    pub fn new() -> Result<Self> {
        let conn = Connect::open(Some("qemu:///system"))?;
        Ok(Self { conn })
    }

    pub fn define_vm(&self, xml: &str) -> Result<()> {
        Domain::define_xml(&self.conn, xml)?;
        Ok(())
    }

    pub fn start_vm(&self, name: &str) -> Result<()> {
        let domain = self.lookup(name)?;
        if domain.is_active()? {
            return Err(WayboxError::VmWrongState {
                name: name.to_string(),
                state: "running".to_string(),
            });
        }
        domain.create()?;
        Ok(())
    }

    pub fn stop_vm(&self, name: &str) -> Result<()> {
        let domain = self.lookup(name)?;
        if !domain.is_active()? {
            return Err(WayboxError::VmWrongState {
                name: name.to_string(),
                state: "shutoff".to_string(),
            });
        }
        domain.shutdown()?;
        Ok(())
    }

    pub fn destroy_vm(&self, name: &str) -> Result<()> {
        let domain = self.lookup(name)?;
        if domain.is_active()? {
            let _ = domain.destroy();
        }
        domain.undefine()?;
        Ok(())
    }

    pub fn list_domains(&self) -> Result<Vec<DomainInfo>> {
        let domains = self.conn.list_all_domains(0)?;
        let mut infos = Vec::new();
        for domain in domains {
            let name = domain.get_name()?;
            let (state_raw, _) = domain.get_state()?;
            infos.push(DomainInfo {
                name,
                state: DomainState::from(state_raw),
            });
        }
        Ok(infos)
    }

    pub fn get_domain_state(&self, name: &str) -> Result<DomainState> {
        let domain = self.lookup(name)?;
        let (state_raw, _) = domain.get_state()?;
        Ok(DomainState::from(state_raw))
    }

    fn lookup(&self, name: &str) -> Result<Domain> {
        Domain::lookup_by_name(&self.conn, name).map_err(|_| WayboxError::VmNotFound {
            name: name.to_string(),
        })
    }
}
