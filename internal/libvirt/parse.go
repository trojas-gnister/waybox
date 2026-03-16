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
// Looks for: <cid auto='yes' value='N'/>
func ParseVsockCID(xml string) (uint32, bool) {
	// Simple string parsing — look for cid value in vsock section
	idx := strings.Index(xml, "<cid ")
	if idx < 0 {
		return 0, false
	}
	snippet := xml[idx:]
	valIdx := strings.Index(snippet, "value='")
	if valIdx < 0 {
		return 0, false
	}
	start := valIdx + len("value='")
	end := strings.Index(snippet[start:], "'")
	if end < 0 {
		return 0, false
	}
	cidStr := snippet[start : start+end]
	var cid uint32
	for _, c := range cidStr {
		if c < '0' || c > '9' {
			return 0, false
		}
		cid = cid*10 + uint32(c-'0')
	}
	return cid, true
}

func isValidIPv4(ip string) bool {
	return net.ParseIP(ip) != nil && strings.Contains(ip, ".")
}
