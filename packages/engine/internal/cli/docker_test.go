package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDockerRunArgs(t *testing.T) {
	args := dockerRunArgs()
	assert.Equal(t, []string{
		"run", "-d",
		"--name", "mantle-postgres",
		"-p", "5432:5432",
		"-e", "POSTGRES_USER=mantle",
		"-e", "POSTGRES_PASSWORD=mantle",
		"-e", "POSTGRES_DB=mantle",
		"-v", "mantle-pgdata:/var/lib/postgresql/data",
		"postgres:16-alpine",
	}, args)
}

func TestParseHostFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"standard", "postgres://mantle:mantle@localhost:5432/mantle", "localhost"},
		{"remote", "postgres://user:pass@db.example.com:5432/mydb", "db.example.com"},
		{"ipv4", "postgres://user:pass@10.0.0.1:5432/mydb", "10.0.0.1"},
		{"ipv6", "postgres://user:pass@[::1]:5432/mydb", "::1"},
		{"no-port", "postgres://user:pass@myhost/mydb", "myhost"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseHostFromURL(tt.url))
		})
	}
}
