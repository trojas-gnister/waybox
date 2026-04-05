use waybox::config::{NetworkMode, SharedFolder, UsbDevice, WayboxConfig};

#[test]
fn test_config_full_roundtrip() {
    let config = WayboxConfig {
        name: "integration-test-vm".to_string(),
        memory_mb: 4096,
        vcpus: 4,
        disk_gb: 40,
        system_packages: vec!["firefox".to_string(), "git".to_string(), "htop".to_string()],
        flatpak_packages: vec!["org.mozilla.firefox".to_string()],
        usb_devices: vec![UsbDevice { vendor: "046d".to_string(), product: "c52b".to_string() }],
        shared_folders: vec![SharedFolder { host_path: "/tmp/test-share".to_string(), guest_path: "/mnt/share".to_string() }],
        network_mode: NetworkMode::Airgapped,
        vsock_cid: 42,
        headless: false,
        share_readonly: true,
    };

    let toml_str = toml::to_string_pretty(&config).unwrap();
    let loaded: WayboxConfig = toml::from_str(&toml_str).unwrap();

    assert_eq!(config.name, loaded.name);
    assert_eq!(config.memory_mb, loaded.memory_mb);
    assert_eq!(config.vcpus, loaded.vcpus);
    assert_eq!(config.disk_gb, loaded.disk_gb);
    assert_eq!(config.system_packages, loaded.system_packages);
    assert_eq!(config.flatpak_packages, loaded.flatpak_packages);
    assert_eq!(config.usb_devices, loaded.usb_devices);
    assert_eq!(config.shared_folders, loaded.shared_folders);
    assert_eq!(config.network_mode, loaded.network_mode);
    assert_eq!(config.vsock_cid, loaded.vsock_cid);
    assert_eq!(config.headless, loaded.headless);
    assert_eq!(config.share_readonly, loaded.share_readonly);
}

#[test]
fn test_config_minimal_with_defaults() {
    let toml_str = r#"
        name = "minimal-vm"
        vsock_cid = 3
    "#;
    let config: WayboxConfig = toml::from_str(toml_str).unwrap();
    assert_eq!(config.name, "minimal-vm");
    assert_eq!(config.memory_mb, 2048);
    assert_eq!(config.vcpus, 2);
    assert_eq!(config.disk_gb, 20);
    assert_eq!(config.network_mode, NetworkMode::Nat);
    assert!(config.system_packages.is_empty());
    assert!(!config.headless);
}

#[test]
fn test_domain_xml_generation() {
    let config = WayboxConfig {
        name: "xml-test".to_string(),
        memory_mb: 2048, vcpus: 2, disk_gb: 20,
        system_packages: vec![], flatpak_packages: vec![],
        usb_devices: vec![UsbDevice { vendor: "046d".to_string(), product: "c52b".to_string() }],
        shared_folders: vec![SharedFolder { host_path: "/tmp/share".to_string(), guest_path: "/mnt/share".to_string() }],
        network_mode: NetworkMode::Nat,
        vsock_cid: 10, headless: false, share_readonly: false,
    };
    let xml = waybox::libvirt::domain::generate_domain_xml(&config);
    assert!(xml.starts_with("<domain type='kvm'>"));
    assert!(xml.contains("<name>xml-test</name>"));
    assert!(xml.contains("<memory unit='MiB'>2048</memory>"));
    assert!(xml.contains("<vcpu>2</vcpu>"));
    assert!(xml.contains("val='10'"));
    assert!(xml.contains("0x046d"));
    assert!(xml.contains("virtiofs"));
    assert!(xml.contains("<interface type='network'>"));
    assert!(xml.contains("<video>"));
    assert!(xml.contains("<serial type='pty'>"));
}

#[test]
fn test_nixos_config_rendering() {
    let config = WayboxConfig {
        name: "nix-test".to_string(),
        memory_mb: 2048, vcpus: 2, disk_gb: 20,
        system_packages: vec!["firefox".to_string()],
        flatpak_packages: vec!["org.mozilla.firefox".to_string()],
        usb_devices: vec![],
        shared_folders: vec![SharedFolder { host_path: "/tmp".to_string(), guest_path: "/mnt/tmp".to_string() }],
        network_mode: NetworkMode::Nat,
        vsock_cid: 3, headless: false, share_readonly: true,
    };
    let rendered = waybox::nixos::render_nixos_config(&config, "test-password").unwrap();
    assert!(rendered.base.contains("nix-test"));
    assert!(rendered.base.contains("test-password"));
    assert!(rendered.base.contains("firefox"));
    assert!(rendered.waypipe.is_some());
    assert!(rendered.audio.is_some());
    assert!(rendered.venus.is_some());
    assert!(rendered.virtiofs.is_some());
    assert!(rendered.flatpak.is_some());

    // Check virtiofs content via as_deref to avoid partial move
    assert!(rendered.virtiofs.as_deref().unwrap().contains("\"ro\""));
    assert!(rendered.flatpak.as_deref().unwrap().contains("org.mozilla.firefox"));

    let config_nix = waybox::nixos::generate_configuration_nix(&rendered);
    assert!(config_nix.contains("./base.nix"));
    assert!(config_nix.contains("./waypipe.nix"));
    assert!(config_nix.contains("./virtiofs.nix"));
    assert!(config_nix.contains("./flatpak.nix"));
}
