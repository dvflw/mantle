# Mantle

Headless AI workflow automation platform — BYOK, IaC-first, enterprise-grade, open source.

## Development

### Prerequisites

- Go 1.22+
- Docker & Docker Compose

### Setup

```bash
# Clone the repo
git clone https://github.com/dvflw/mantle.git
cd mantle

# Start Postgres
docker-compose up -d

# Build
make build

# Verify
./mantle version
```

### Common Commands

```bash
make build      # Build binary with version info
make test       # Run tests
make lint       # Run golangci-lint
make run        # Run without building (go run)
make dev        # Start docker-compose services
make migrate    # Run database migrations (placeholder)
make clean      # Remove built binary
```

## Configuration

Mantle reads configuration from `mantle.yaml`, environment variables, and CLI flags (highest precedence).

```yaml
database:
  url: postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable

api:
  address: ":8080"

log:
  level: info
```

Environment variables use the `MANTLE_` prefix:

- `MANTLE_DATABASE_URL`
- `MANTLE_API_ADDRESS`
- `MANTLE_LOG_LEVEL`

## License

BSL/SSPL-style — source available, no commercial resale of forks.
