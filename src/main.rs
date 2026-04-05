use clap::Parser;

#[derive(Parser)]
#[command(name = "waybox", about = "Isolated application VMs with native Wayland integration")]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(clap::Subcommand)]
enum Commands {
    /// Create a new application VM
    Create,
    /// Start a VM
    Start { name: String },
    /// Stop a VM gracefully
    Stop { name: String },
    /// Destroy a VM and delete its config
    Destroy { name: String },
    /// Launch an app from a VM via waypipe
    Launch { name: String, command: String },
    /// List all configured VMs
    List,
    /// Connect to VM serial console
    Console { name: String },
    /// Show stored VM passwords
    Passwords,
    /// Generate .desktop shortcuts for a VM's apps
    GenerateShortcuts { name: String },
}

fn main() {
    env_logger::init();
    let cli = Cli::parse();

    if let Err(e) = run(cli) {
        eprintln!("Error: {e}");
        std::process::exit(1);
    }
}

fn run(cli: Cli) -> waybox::error::Result<()> {
    match cli.command {
        _ => {
            eprintln!("Command not yet implemented");
            Ok(())
        }
    }
}
