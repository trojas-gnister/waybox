use crate::config::WayboxConfig;
use crate::error::{Result, WayboxError};
use std::path::{Path, PathBuf};
use std::process::Command;

pub fn check_prerequisites() -> Result<()> {
    check_in_path("nix", "Install Nix: https://nixos.org/download")?;
    check_in_path(
        "nixos-generate",
        "Install nixos-generators: nix-env -iA nixpkgs.nixos-generators",
    )?;
    Ok(())
}

fn check_in_path(tool: &str, hint: &str) -> Result<()> {
    match Command::new("which").arg(tool).output() {
        Ok(output) if output.status.success() => Ok(()),
        _ => Err(WayboxError::PrerequisiteNotFound {
            tool: tool.to_string(),
            hint: hint.to_string(),
        }),
    }
}

pub fn build_image(
    config: &WayboxConfig,
    rendered: &super::RenderedNixosConfig,
) -> Result<PathBuf> {
    let build_dir = std::env::temp_dir().join(format!("waybox-build-{}", config.name));
    std::fs::create_dir_all(&build_dir).map_err(|e| WayboxError::Io {
        context: format!("creating build dir {:?}", build_dir),
        source: e,
    })?;

    // Write all rendered config files
    write_file(&build_dir, "base.nix", &rendered.base)?;
    if let Some(ref content) = rendered.waypipe {
        write_file(&build_dir, "waypipe.nix", content)?;
    }
    if let Some(ref content) = rendered.audio {
        write_file(&build_dir, "audio.nix", content)?;
    }
    if let Some(ref content) = rendered.venus {
        write_file(&build_dir, "venus.nix", content)?;
    }
    if let Some(ref content) = rendered.virtiofs {
        write_file(&build_dir, "virtiofs.nix", content)?;
    }
    if let Some(ref content) = rendered.flatpak {
        write_file(&build_dir, "flatpak.nix", content)?;
    }

    let configuration_nix = super::generate_configuration_nix(rendered);
    write_file(&build_dir, "configuration.nix", &configuration_nix)?;

    log::info!(
        "Building NixOS image from {:?}",
        build_dir.join("configuration.nix")
    );

    let output = Command::new("nixos-generate")
        .arg("--format")
        .arg("qcow")
        .arg("--configuration")
        .arg(build_dir.join("configuration.nix"))
        .output()
        .map_err(|e| WayboxError::Io {
            context: "running nixos-generate".to_string(),
            source: e,
        })?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        let _ = std::fs::remove_dir_all(&build_dir);
        return Err(WayboxError::ImageBuild(stderr.to_string()));
    }

    let image_output = String::from_utf8_lossy(&output.stdout).trim().to_string();
    let built_image = PathBuf::from(&image_output);

    // Move image to final location
    let images_dir = WayboxConfig::images_dir()?;
    std::fs::create_dir_all(&images_dir).map_err(|e| WayboxError::Io {
        context: format!("creating images dir {:?}", images_dir),
        source: e,
    })?;
    let final_path = config.image_path()?;
    std::fs::copy(&built_image, &final_path).map_err(|e| WayboxError::Io {
        context: format!("copying image to {:?}", final_path),
        source: e,
    })?;

    let _ = std::fs::remove_dir_all(&build_dir);
    let _ = std::fs::remove_file(&built_image);

    log::info!("Image built: {:?}", final_path);
    Ok(final_path)
}

fn write_file(dir: &Path, name: &str, content: &str) -> Result<()> {
    let path = dir.join(name);
    std::fs::write(&path, content).map_err(|e| WayboxError::Io {
        context: format!("writing {:?}", path),
        source: e,
    })
}
