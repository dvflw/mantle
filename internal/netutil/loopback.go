package netutil

import (
	"net"
	"strings"
)

// IsLoopback returns true if host is a loopback address: localhost, 127.0.0.1, or ::1.
func IsLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
