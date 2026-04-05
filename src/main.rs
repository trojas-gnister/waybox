use clap::{Parser, Subcommand};

use waybox::config::{NetworkMode, SharedFolder, UsbDevice, WayboxConfig};
use waybox::config::passwords::PasswordStore;
use waybox::display::desktop;
use waybox::display::DisplaySession;
use waybox::libvirt::LibvirtConnection;
use waybox::provisioner;

// ── Top-level CLI ──────────────────────────────────────────────────────────

#[derive(Parser)]
#[command(
    name = "waybox",
    about = "Isolated application VMs with native Wayland integration",
    version
)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

// ── Sub-commands ───────────────────────────────────────────────────────────

#[derive(Subcommand)]
enum Commands {
    /// Create a new application VM
    Create {
        /// VM name (alphanumeric + hyphens)
        #[arg(long)]
        name: String,

        /// NixOS system packages to install
        #[arg(long = "system", value_name = "PKG")]
        system_packages: Vec<String>,

        /// Flathub application IDs to install
        #[arg(long = "flatpak", value_name = "PKG")]
        flatpak_packages: Vec<String>,

        /// RAM in MB (default: 2048)
        #[arg(long, default_value_t = 2048)]
        memory: u32,

        /// Number of virtual CPUs (default: 2)
        #[arg(long, default_value_t = 2)]
        vcpus: u32,

        /// Disk size in GB (default: 20)
        #[arg(long, default_value_t = 20)]
        disk: u32,

        /// USB devices to pass through (vendor:product, e.g. 046d:c52b)
        #[arg(long = "usb", value_name = "VENDOR:PRODUCT")]
        usb_devices: Vec<String>,

        /// Shared folders (host_path:guest_path)
        #[arg(long = "share", value_name = "HOST:GUEST")]
        shared_folders: Vec<String>,

        /// Mount shared folders read-only
        #[arg(long)]
        share_readonly: bool,

        /// Disable VM networking (air-gapped mode)
        #[arg(long)]
        no_network: bool,

        /// Create VM without a display (headless/server mode)
        #[arg(long)]
        headless: bool,

        /// Skip confirmation prompts
        #[arg(short = 'y', long = "yes")]
        yes: bool,
    },

    /// Start a VM
    Start {
        /// VM name
        name: String,
    },

    /// Stop a VM gracefully
    Stop {
        /// VM name
        name: String,
    },

    /// Destroy a VM and remove all associated resources
    Destroy {
        /// VM name
        name: String,

        /// Skip confirmation prompt
        #[arg(short = 'y', long = "yes")]
        yes: bool,
    },

    /// Launch an application inside a running VM via waypipe
    Launch {
        /// VM name
        name: String,

        /// Command to run inside the VM
        command: String,
    },

    /// List all configured VMs with their current state
    List,

    /// Attach to a VM's serial console (via virsh)
    Console {
        /// VM name
        name: String,
    },

    /// Show stored passwords for all VMs
    Passwords,

    /// Generate .desktop shortcuts for a VM's applications
    GenerateShortcuts {
        /// VM name
        name: String,
    },
}

// ── Entry point ────────────────────────────────────────────────────────────

fn main() {
    env_logger::init();
    let cli = Cli::parse();

    if let Err(e) = run(cli) {
        eprintln!("Error: {e}");
        std::process::exit(1);
    }
}

// ── Dispatch ───────────────────────────────────────────────────────────────

fn run(cli: Cli) -> waybox::error::Result<()> {
    match cli.command {
        Commands::Create {
            name,
            system_packages,
            flatpak_packages,
            memory,
            vcpus,
            disk,
            usb_devices,
            shared_folders,
            share_readonly,
            no_network,
            headless,
            yes,
        } => cmd_create(
            name,
            system_packages,
            flatpak_packages,
            memory,
            vcpus,
            disk,
            usb_devices,
            shared_folders,
            share_readonly,
            no_network,
            headless,
            yes,
        ),

        Commands::Start { name } => provisioner::start_vm(&name),

        Commands::Stop { name } => provisioner::stop_vm(&name),

        Commands::Destroy { name, yes } => cmd_destroy(&name, yes),

        Commands::Launch { name, command } => cmd_launch(&name, &command),

        Commands::List => cmd_list(),

        Commands::Console { name } => cmd_console(&name),

        Commands::Passwords => cmd_passwords(),

        Commands::GenerateShortcuts { name } => cmd_generate_shortcuts(&name),
    }
}

// ── Command implementations ────────────────────────────────────────────────

#[allow(clippy::too_many_arguments)]
fn cmd_create(
    name: String,
    system_packages: Vec<String>,
    flatpak_packages: Vec<String>,
    memory: u32,
    vcpus: u32,
    disk: u32,
    usb_raw: Vec<String>,
    shares_raw: Vec<String>,
    share_readonly: bool,
    no_network: bool,
    headless: bool,
    yes: bool,
) -> waybox::error::Result<()> {
    // Parse USB device IDs
    let usb_devices: waybox::error::Result<Vec<UsbDevice>> =
        usb_raw.iter().map(|s| UsbDevice::from_id(s)).collect();
    let usb_devices = usb_devices?;

    // Parse shared folder specs (host_path:guest_path)
    let shared_folders: waybox::error::Result<Vec<SharedFolder>> = shares_raw
        .iter()
        .map(|s| parse_share(s))
        .collect();
    let shared_folders = shared_folders?;

    let vsock_cid = WayboxConfig::next_available_cid()?;

    let config = WayboxConfig {
        name,
        memory_mb: memory,
        vcpus,
        disk_gb: disk,
        system_packages,
        flatpak_packages,
        usb_devices,
        shared_folders,
        network_mode: if no_network {
            NetworkMode::Airgapped
        } else {
            NetworkMode::Nat
        },
        vsock_cid,
        headless,
        share_readonly,
    };

    config.validate()?;

    if !yes {
        println!("About to create VM '{}':", config.name);
        println!("  Memory:   {} MB", config.memory_mb);
        println!("  vCPUs:    {}", config.vcpus);
        println!("  Disk:     {} GB", config.disk_gb);
        if !config.system_packages.is_empty() {
            println!("  Packages: {}", config.system_packages.join(", "));
        }
        if !config.flatpak_packages.is_empty() {
            println!("  Flatpaks: {}", config.flatpak_packages.join(", "));
        }
        println!("  Headless: {}", config.headless);
        println!("  Network:  {:?}", config.network_mode);
        print!("Proceed? [y/N] ");
        use std::io::Write;
        std::io::stdout().flush().ok();
        let mut input = String::new();
        std::io::stdin().read_line(&mut input).ok();
        if !matches!(input.trim().to_lowercase().as_str(), "y" | "yes") {
            println!("Aborted.");
            return Ok(());
        }
    }

    provisioner::create_vm(&config)
}

fn cmd_destroy(name: &str, yes: bool) -> waybox::error::Result<()> {
    if !yes {
        print!(
            "This will permanently destroy VM '{}' and all its data. Proceed? [y/N] ",
            name
        );
        use std::io::Write;
        std::io::stdout().flush().ok();
        let mut input = String::new();
        std::io::stdin().read_line(&mut input).ok();
        if !matches!(input.trim().to_lowercase().as_str(), "y" | "yes") {
            println!("Aborted.");
            return Ok(());
        }
    }
    provisioner::destroy_vm(name)
}

fn cmd_launch(name: &str, command: &str) -> waybox::error::Result<()> {
    let config = WayboxConfig::load(name)?;
    let session = DisplaySession::new(name, config.vsock_cid);
    session.launch_app(command)
}

fn cmd_list() -> waybox::error::Result<()> {
    let configs = WayboxConfig::list_all()?;

    if configs.is_empty() {
        println!("No VMs configured. Create one with: waybox create --name <name>");
        return Ok(());
    }

    // Optionally enrich with libvirt state (best-effort; don't fail if libvirt is down)
    let domain_states: std::collections::HashMap<String, String> =
        match LibvirtConnection::new() {
            Ok(conn) => match conn.list_domains() {
                Ok(domains) => domains
                    .into_iter()
                    .map(|d| (d.name, d.state.to_string()))
                    .collect(),
                Err(_) => Default::default(),
            },
            Err(_) => Default::default(),
        };

    // Header
    println!(
        "{:<20} {:>8} {:>6} {:>8} {:>10} {:>12}",
        "NAME", "MEM(MB)", "VCPUS", "DISK(GB)", "VSOCK CID", "STATE"
    );
    println!("{}", "-".repeat(70));

    let mut sorted = configs;
    sorted.sort_by(|a, b| a.name.cmp(&b.name));

    for cfg in &sorted {
        let state = domain_states
            .get(&cfg.name)
            .map(|s| s.as_str())
            .unwrap_or("unknown");
        println!(
            "{:<20} {:>8} {:>6} {:>8} {:>10} {:>12}",
            cfg.name, cfg.memory_mb, cfg.vcpus, cfg.disk_gb, cfg.vsock_cid, state
        );
    }

    Ok(())
}

fn cmd_console(name: &str) -> waybox::error::Result<()> {
    let status = std::process::Command::new("virsh")
        .arg("console")
        .arg(name)
        .status()
        .map_err(|e| waybox::error::WayboxError::Io {
            context: "launching virsh console".to_string(),
            source: e,
        })?;

    if !status.success() {
        eprintln!("virsh console exited with status: {}", status);
    }
    Ok(())
}

fn cmd_passwords() -> waybox::error::Result<()> {
    let store = PasswordStore::load()?;

    if store.passwords.is_empty() {
        println!("No passwords stored.");
        return Ok(());
    }

    println!("{:<20} {}", "VM NAME", "PASSWORD");
    println!("{}", "-".repeat(40));

    let mut entries: Vec<(&String, &String)> = store.passwords.iter().collect();
    entries.sort_by_key(|(k, _)| *k);

    for (vm, pw) in entries {
        println!("{:<20} {}", vm, pw);
    }

    Ok(())
}

fn cmd_generate_shortcuts(name: &str) -> waybox::error::Result<()> {
    let config = WayboxConfig::load(name)?;
    let session = DisplaySession::new(name, config.vsock_cid);

    println!("Querying guest apps for '{}'...", name);
    let apps = session.list_apps()?;

    if apps.is_empty() {
        println!("No apps found in VM '{}'.", name);
        return Ok(());
    }

    let paths = desktop::write_desktop_files(name, &apps)?;
    println!("Generated {} desktop file(s):", paths.len());
    for path in &paths {
        println!("  {}", path.display());
    }
    Ok(())
}

// ── Helpers ────────────────────────────────────────────────────────────────

/// Parse `host_path:guest_path` into a `SharedFolder`.
fn parse_share(s: &str) -> waybox::error::Result<SharedFolder> {
    match s.split_once(':') {
        Some((host, guest)) if !host.is_empty() && !guest.is_empty() => Ok(SharedFolder {
            host_path: host.to_string(),
            guest_path: guest.to_string(),
        }),
        _ => Err(waybox::error::WayboxError::InvalidSharePath {
            path: s.to_string(),
            reason: "expected format host_path:guest_path".to_string(),
        }),
    }
}
