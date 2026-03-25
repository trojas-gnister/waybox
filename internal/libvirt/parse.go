package libvirt

import (
	"net"
	"strings"
)

// ParseIPFromDomifaddr extracts an IPv4 address from virsh domifaddr output.
//
// Expected format:
//
//	Name       MAC address          Protocol     Address
//	vnet0      52:54:00:xx:xx:xx    ipv4         192.168.122.x/24
func ParseIPFromDomifaddr(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "ipv4") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				ip, _, _ := strings.Cut(fields[3], "/")
				if isValidIPv4(ip) {
					return ip
				}
			}
		}
	}
	return ""
}

// ParseIPFromDomifaddrAgent extracts an IPv4 address from guest agent output,
// skipping loopback and link-local addresses.
func ParseIPFromDomifaddrAgent(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "ipv4") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				ip, _, _ := strings.Cut(fields[3], "/")
				if strings.HasPrefix(ip, "127.") || strings.HasPrefix(ip, "169.254.") {
					continue
				}
				if isValidIPv4(ip) {
					return ip
				}
			}
		}
	}
	return ""
}

// ParseVsockCID extracts the vsock CID from a libvirt domain XML string.
// Looks for: <cid auto='yes' address='N'/> or <cid auto='yes' value='N'/>
func ParseVsockCID(xml string) (uint32, bool) {
	idx := strings.Index(xml, "<cid ")
	if idx < 0 {
		return 0, false
	}
	snippet := xml[idx:]

	// Try both "address=" and "value=" — libvirt uses "address" in practice
	for _, attr := range []string{"address='", `address="`, "value='", `value="`} {
		attrIdx := strings.Index(snippet, attr)
		if attrIdx < 0 {
			continue
		}
		start := attrIdx + len(attr)
		// Find closing quote (either ' or ")
		quote := attr[len(attr)-1]
		end := strings.IndexByte(snippet[start:], quote)
		if end < 0 {
			continue
		}
		cidStr := snippet[start : start+end]
		var cid uint32
		for _, c := range cidStr {
			if c < '0' || c > '9' {
				cid = 0
				break
			}
			cid = cid*10 + uint32(c-'0')
		}
		if cid > 0 {
			return cid, true
		}
	}
	return 0, false
}

func isValidIPv4(ip string) bool {
	return net.ParseIP(ip) != nil && strings.Contains(ip, ".")
}
