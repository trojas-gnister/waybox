package libvirt

import "testing"

func TestParseIPFromDomifaddr(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantIP string
	}{
		{
			"standard NAT output",
			" Name       MAC address          Protocol     Address\n" +
				"-------------------------------------------------------------------------------\n" +
				" vnet0      52:54:00:a1:b2:c3    ipv4         192.168.122.45/24\n",
			"192.168.122.45",
		},
		{
			"no ipv4 line",
			" Name       MAC address          Protocol     Address\n",
			"",
		},
		{
			"empty output",
			"",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseIPFromDomifaddr(tt.input)
			if got != tt.wantIP {
				t.Errorf("ParseIPFromDomifaddr() = %q, want %q", got, tt.wantIP)
			}
		})
	}
}

func TestParseIPFromDomifaddrAgent(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantIP string
	}{
		{
			"skips loopback",
			" Name       MAC address          Protocol     Address\n" +
				" lo         00:00:00:00:00:00    ipv4         127.0.0.1/8\n" +
				" enp1s0     52:54:00:a1:b2:c3    ipv4         192.168.1.100/24\n",
			"192.168.1.100",
		},
		{
			"skips link-local",
			" Name       MAC address          Protocol     Address\n" +
				" eth0       52:54:00:a1:b2:c3    ipv4         169.254.1.1/16\n" +
				" eth1       52:54:00:d4:e5:f6    ipv4         10.0.0.5/24\n",
			"10.0.0.5",
		},
		{
			"only loopback",
			" lo         00:00:00:00:00:00    ipv4         127.0.0.1/8\n",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseIPFromDomifaddrAgent(tt.input)
			if got != tt.wantIP {
				t.Errorf("ParseIPFromDomifaddrAgent() = %q, want %q", got, tt.wantIP)
			}
		})
	}
}

func TestParseVsockCID(t *testing.T) {
	tests := []struct {
		name    string
		xml     string
		wantCID uint32
		wantOK  bool
	}{
		{
			"address attribute (libvirt default)",
			`<vsock model='virtio'><cid auto='yes' address='3'/></vsock>`,
			3,
			true,
		},
		{
			"value attribute",
			`<vsock model='virtio'><cid auto='yes' value='3'/></vsock>`,
			3,
			true,
		},
		{
			"higher CID with address",
			`<vsock><cid auto='yes' address='42'/></vsock>`,
			42,
			true,
		},
		{
			"no vsock section",
			`<domain><name>test</name></domain>`,
			0,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cid, ok := ParseVsockCID(tt.xml)
			if ok != tt.wantOK || cid != tt.wantCID {
				t.Errorf("ParseVsockCID() = (%d, %v), want (%d, %v)", cid, ok, tt.wantCID, tt.wantOK)
			}
		})
	}
}

func TestUSBHostdevXML(t *testing.T) {
	xml, err := USBHostdevXML("046d", "c52b")
	if err != nil {
		t.Fatal(err)
	}
	if len(xml) == 0 {
		t.Fatal("empty XML output")
	}
	// Should contain the vendor/product IDs with 0x prefix
	if !contains(xml, "0x046d") || !contains(xml, "0xc52b") {
		t.Errorf("XML missing vendor/product IDs: %s", xml)
	}
}

func TestNetworkInterfaceXML(t *testing.T) {
	xml, err := NetworkInterfaceXML("52:54:00:a1:b2:c3", "default")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(xml, "52:54:00:a1:b2:c3") || !contains(xml, "default") {
		t.Errorf("XML missing expected values: %s", xml)
	}
}

func TestBridgeInterfaceXML(t *testing.T) {
	xml, err := BridgeInterfaceXML("52:54:00:a1:b2:c3", "br0")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(xml, "br0") || !contains(xml, "virtio") {
		t.Errorf("XML missing expected values: %s", xml)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
