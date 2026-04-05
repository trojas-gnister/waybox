// Libvirt domain XML generation and management

use crate::config::{NetworkMode, WayboxConfig};

pub fn generate_domain_xml(config: &WayboxConfig) -> String {
    let image_path = config
        .image_path()
        .map(|p| p.to_string_lossy().into_owned())
        .unwrap_or_else(|_| format!("{}.qcow2", config.name));

    // Network interface XML
    let network_xml = match config.network_mode {
        NetworkMode::Nat => "    <interface type='network'>\n      <source network='default'/>\n      <model type='virtio'/>\n    </interface>\n".to_string(),
        NetworkMode::Airgapped => String::new(),
    };

    // USB device XML
    let usb_xml: String = config
        .usb_devices
        .iter()
        .map(|dev| {
            format!(
                "    <hostdev mode='subsystem' type='usb' managed='yes'>\n      <source>\n        <vendor id='0x{vendor}'/>\n        <product id='0x{product}'/>\n      </source>\n    </hostdev>\n",
                vendor = dev.vendor,
                product = dev.product,
            )
        })
        .collect();

    // Shared folders XML and memoryBacking
    let memory_backing_xml = if config.shared_folders.is_empty() {
        String::new()
    } else {
        "  <memoryBacking>\n    <source type='memfd'/>\n    <access mode='shared'/>\n  </memoryBacking>\n".to_string()
    };

    let shared_folders_xml: String = config
        .shared_folders
        .iter()
        .enumerate()
        .map(|(i, folder)| {
            let readonly_tag = if config.share_readonly {
                "      <readonly/>\n"
            } else {
                ""
            };
            format!(
                "    <filesystem type='mount' accessmode='passthrough'>\n      <driver type='virtiofs'/>\n      <source dir='{host_path}'/>\n      <target dir='fs{i}'/>\n{readonly}    </filesystem>\n",
                host_path = folder.host_path,
                i = i,
                readonly = readonly_tag,
            )
        })
        .collect();

    // Video XML
    let video_xml = if config.headless {
        String::new()
    } else {
        "    <video>\n      <model type='virtio' heads='1' 3d='yes'/>\n    </video>\n".to_string()
    };

    format!(
        "<domain type='kvm'>\n\
           <name>{name}</name>\n\
           <memory unit='MiB'>{memory_mb}</memory>\n\
           <vcpu>{vcpus}</vcpu>\n\
         {memory_backing}\
           <os>\n\
             <type arch='x86_64' machine='q35'>hvm</type>\n\
         </os>\n\
         <devices>\n\
             <disk type='file' device='disk'>\n\
               <driver name='qemu' type='qcow2'/>\n\
               <source file='{image_path}'/>\n\
               <target dev='vda' bus='virtio'/>\n\
             </disk>\n\
         {network}\
         {usb}\
         {shared_folders}\
         {video}\
             <serial type='pty'>\n\
               <target port='0'/>\n\
             </serial>\n\
             <console type='pty'>\n\
               <target type='serial' port='0'/>\n\
             </console>\n\
             <vsock model='virtio'>\n\
               <cid auto='no' val='{vsock_cid}'/>\n\
             </vsock>\n\
         </devices>\n\
         </domain>",
        name = config.name,
        memory_mb = config.memory_mb,
        vcpus = config.vcpus,
        memory_backing = memory_backing_xml,
        image_path = image_path,
        network = network_xml,
        usb = usb_xml,
        shared_folders = shared_folders_xml,
        video = video_xml,
        vsock_cid = config.vsock_cid,
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::{NetworkMode, SharedFolder, UsbDevice, WayboxConfig};

    fn base_config() -> WayboxConfig {
        WayboxConfig {
            name: "test-vm".to_string(),
            memory_mb: 4096,
            vcpus: 4,
            disk_gb: 20,
            system_packages: vec![],
            flatpak_packages: vec![],
            usb_devices: vec![],
            shared_folders: vec![],
            network_mode: NetworkMode::Nat,
            vsock_cid: 5,
            headless: false,
            share_readonly: false,
        }
    }

    #[test]
    fn test_xml_contains_basic_elements() {
        let xml = generate_domain_xml(&base_config());
        assert!(xml.contains("<domain type='kvm'>"));
        assert!(xml.contains("<name>test-vm</name>"));
        assert!(xml.contains("<memory unit='MiB'>4096</memory>"));
        assert!(xml.contains("<vcpu>4</vcpu>"));
    }

    #[test]
    fn test_xml_contains_vsock() {
        let xml = generate_domain_xml(&base_config());
        assert!(xml.contains("<vsock model='virtio'>"));
        assert!(xml.contains("<cid auto='no' val='5'/>"));
    }

    #[test]
    fn test_xml_contains_virtio_gpu() {
        let xml = generate_domain_xml(&base_config());
        assert!(xml.contains("<model type='virtio'"));
        assert!(xml.contains("3d='yes'"));
    }

    #[test]
    fn test_xml_nat_has_network_interface() {
        let xml = generate_domain_xml(&base_config());
        assert!(xml.contains("<interface type='network'>"));
        assert!(xml.contains("<source network='default'/>"));
    }

    #[test]
    fn test_xml_airgapped_has_no_interface() {
        let mut config = base_config();
        config.network_mode = NetworkMode::Airgapped;
        let xml = generate_domain_xml(&config);
        assert!(!xml.contains("<interface"));
    }

    #[test]
    fn test_xml_includes_usb_device() {
        let mut config = base_config();
        config.usb_devices.push(UsbDevice {
            vendor: "046d".to_string(),
            product: "c52b".to_string(),
        });
        let xml = generate_domain_xml(&config);
        assert!(xml.contains("<hostdev mode='subsystem' type='usb'"));
        assert!(xml.contains("<vendor id='0x046d'/>"));
        assert!(xml.contains("<product id='0xc52b'/>"));
    }

    #[test]
    fn test_xml_includes_shared_folder() {
        let mut config = base_config();
        config.shared_folders.push(SharedFolder {
            host_path: "/home/user/docs".to_string(),
            guest_path: "/mnt/docs".to_string(),
        });
        let xml = generate_domain_xml(&config);
        assert!(xml.contains("<filesystem type='mount' accessmode='passthrough'>"));
        assert!(xml.contains("/home/user/docs"));
        assert!(xml.contains("target dir='fs0'"));
    }

    #[test]
    fn test_xml_headless_no_video() {
        let mut config = base_config();
        config.headless = true;
        let xml = generate_domain_xml(&config);
        assert!(!xml.contains("<video>"));
    }

    #[test]
    fn test_xml_has_serial_console() {
        let xml = generate_domain_xml(&base_config());
        assert!(xml.contains("<serial type='pty'>"));
        assert!(xml.contains("<console type='pty'>"));
    }

    #[test]
    fn test_xml_disk_points_to_image() {
        let xml = generate_domain_xml(&base_config());
        assert!(xml.contains("test-vm.qcow2"));
        assert!(xml.contains("<driver name='qemu' type='qcow2'/>"));
    }

    #[test]
    fn test_xml_single_devices_block() {
        let xml = generate_domain_xml(&base_config());
        // Libvirt only supports one <devices> block; vsock must be inside it.
        let open_count = xml.matches("<devices>").count();
        let close_count = xml.matches("</devices>").count();
        assert_eq!(open_count, 1, "expected exactly one <devices> opening tag");
        assert_eq!(close_count, 1, "expected exactly one </devices> closing tag");
        // vsock must appear before </devices>
        let vsock_pos = xml.find("<vsock").unwrap();
        let close_devices_pos = xml.find("</devices>").unwrap();
        assert!(vsock_pos < close_devices_pos, "vsock must be inside the <devices> block");
    }
}
