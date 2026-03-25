package dbdefaults

// Runtime defaults — used by Docker auto-provisioning and config defaults.
// These match the default database URL in config.go.
const (
	PostgresImage = "postgres:16-alpine"
	User          = "mantle"
	Password      = "mantle"
	Database      = "mantle"
	ContainerName = "mantle-postgres"
)

// Test defaults — used by testcontainers setups.
const (
	TestDatabase = "mantle_test"
)
