package libvirt

import (
	"encoding/xml"
	"fmt"
)

// HostdevUSB represents a libvirt USB hostdev XML element for device passthrough.
type HostdevUSB struct {
	XMLName xml.Name      `xml:"hostdev"`
	Mode    string        `xml:"mode,attr"`
	Type    string        `xml:"type,attr"`
	Managed string        `xml:"managed,attr"`
	Source  HostdevSource `xml:"source"`
}

type HostdevSource struct {
	Vendor  HostdevID `xml:"vendor"`
	Product HostdevID `xml:"product"`
}

type HostdevID struct {
	ID string `xml:"id,attr"`
}

// USBHostdevXML generates a libvirt-compatible USB hostdev XML string.
func USBHostdevXML(vendorID, productID string) (string, error) {
	dev := HostdevUSB{
		Mode:    "subsystem",
		Type:    "usb",
		Managed: "yes",
		Source: HostdevSource{
			Vendor:  HostdevID{ID: "0x" + vendorID},
			Product: HostdevID{ID: "0x" + productID},
		},
	}

	data, err := xml.MarshalIndent(dev, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshalling USB hostdev XML: %w", err)
	}
	return string(data), nil
}

// InterfaceXML represents a libvirt network interface XML element.
type InterfaceXML struct {
	XMLName xml.Name        `xml:"interface"`
	Type    string          `xml:"type,attr"`
	MAC     InterfaceMAC    `xml:"mac"`
	Source  InterfaceSource `xml:"source"`
	Model   *InterfaceModel `xml:"model,omitempty"`
}

type InterfaceMAC struct {
	Address string `xml:"address,attr"`
}

type InterfaceSource struct {
	Network string `xml:"network,attr,omitempty"`
	Bridge  string `xml:"bridge,attr,omitempty"`
}

type InterfaceModel struct {
	Type string `xml:"type,attr"`
}

// NetworkInterfaceXML generates a NAT network interface XML string.
func NetworkInterfaceXML(mac, network string) (string, error) {
	iface := InterfaceXML{
		Type:   "network",
		MAC:    InterfaceMAC{Address: mac},
		Source: InterfaceSource{Network: network},
	}
	data, err := xml.MarshalIndent(iface, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshalling network interface XML: %w", err)
	}
	return string(data), nil
}

// BridgeInterfaceXML generates a bridged network interface XML string.
func BridgeInterfaceXML(mac, bridge string) (string, error) {
	iface := InterfaceXML{
		Type:   "bridge",
		MAC:    InterfaceMAC{Address: mac},
		Source: InterfaceSource{Bridge: bridge},
		Model:  &InterfaceModel{Type: "virtio"},
	}
	data, err := xml.MarshalIndent(iface, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshalling bridge interface XML: %w", err)
	}
	return string(data), nil
}
