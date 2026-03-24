package netutil_test

import (
	"testing"

	"github.com/dvflw/mantle/internal/netutil"
	"github.com/stretchr/testify/assert"
)

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected bool
	}{
		{"localhost", "localhost", true},
		{"localhost-upper", "LOCALHOST", true},
		{"localhost-mixed", "Localhost", true},
		{"ipv4-loopback", "127.0.0.1", true},
		{"ipv4-loopback-range", "127.0.0.2", true},
		{"ipv6-loopback", "::1", true},
		{"remote-host", "db.example.com", false},
		{"private-ipv4", "10.0.0.1", false},
		{"lan-ipv4", "192.168.1.1", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, netutil.IsLoopback(tt.host))
		})
	}
}
