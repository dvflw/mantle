# Deployment Guide

This guide covers installing Mantle, running it in production, and hardening your deployment. For trigger configuration and the REST API, see the [Server Guide](server-guide.md). For authentication setup, see the [Authentication & RBAC Guide](authentication-guide.md).

## Prerequisites

Before deploying Mantle, you need:

- **Postgres 14+** -- Mantle's single point of state. All workflow definitions, executions, credentials, and audit events live in the database.
- **Encryption key** -- a 32-byte hex-encoded key for encrypting credentials at rest. See the [Secrets Guide](secrets-guide.md) for details.
- **Domain and TLS** -- required for production. Mantle can terminate TLS directly or run behind a reverse proxy.

## Installation Methods

### Binary Download

Download the latest release from [GitHub Releases](https://github.com/dvflw/mantle/releases):

```bash
# Linux (amd64)
curl -Lo mantle https://github.com/dvflw/mantle/releases/latest/download/mantle-linux-amd64
chmod +x mantle
sudo mv mantle /usr/local/bin/

# macOS (Apple Silicon)
curl -Lo mantle https://github.com/dvflw/mantle/releases/latest/download/mantle-darwin-arm64
chmod +x mantle
sudo mv mantle /usr/local/bin/
```

Verify the installation:

```bash
mantle version
# mantle v0.1.0 (791fa83, built 2026-03-18T00:00:00Z)
```

### Go Install

If you have Go 1.25+ installed:

```bash
go install github.com/dvflw/mantle/cmd/mantle@latest
```

The binary is placed in `$GOPATH/bin` (or `$HOME/go/bin` by default).

### Docker

Pull the official image:

```bash
docker pull ghcr.io/dvflw/mantle:0.1.0
```

Run with environment variables:

```bash
docker run -d \
  -p 8080:8080 \
  -e MANTLE_DATABASE_URL="postgres://mantle:secret@host.docker.internal:5432/mantle?sslmode=disable" \
  -e MANTLE_ENCRYPTION_KEY="your-64-char-hex-key" \
  ghcr.io/dvflw/mantle:0.1.0 serve
```

### Helm Chart

For Kubernetes deployments, use the included Helm chart. See [Production Deployment (Kubernetes/Helm)](#production-deployment-kuberneteshelm) below.

## Quick Start (Docker Compose)

The fastest way to get Mantle running locally with Postgres:

```bash
git clone https://github.com/dvflw/mantle.git && cd mantle
docker compose up -d
```

This starts Postgres 16 on `localhost:5432` with user `mantle`, password `mantle`, and database `mantle`.

Run migrations and start the server:

```bash
export MANTLE_DATABASE_URL="postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable"
export MANTLE_ENCRYPTION_KEY=$(openssl rand -hex 32)

mantle init
mantle serve
```

```
Running migrations...
Migrations complete.
Starting server on :8080
Cron scheduler started (poll interval: 30s)
```

Mantle is now running at `http://localhost:8080`. See the [Getting Started](getting-started.md) guide for your first workflow.

## Production Deployment (Kubernetes/Helm)

The recommended way to run Mantle in production is with the included Helm chart.

### Basic Install

```bash
helm install mantle charts/mantle \
  --set database.url="postgres://mantle:secret@db.internal:5432/mantle?sslmode=require" \
  --set encryption.key="your-64-char-hex-key" \
  --set replicaCount=3
```

### Values Reference

| Value | Default | Description |
|---|---|---|
| `image.repository` | `ghcr.io/dvflw/mantle` | Container image repository |
| `image.tag` | Chart `appVersion` | Container image tag |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy |
| `replicaCount` | `1` | Number of server replicas |
| `database.url` | -- | Postgres connection string (required) |
| `encryption.key` | -- | 32-byte hex encryption key (required) |
| `resources.requests.cpu` | `100m` | CPU request |
| `resources.requests.memory` | `128Mi` | Memory request |
| `resources.limits.cpu` | `500m` | CPU limit |
| `resources.limits.memory` | `512Mi` | Memory limit |
| `probes.liveness.path` | `/healthz` | Liveness probe path |
| `probes.readiness.path` | `/readyz` | Readiness probe path |
| `pdb.enabled` | `false` | Enable PodDisruptionBudget |
| `pdb.minAvailable` | `1` | Minimum available pods during disruption |
| `securityContext.runAsNonRoot` | `true` | Run as non-root user |
| `securityContext.readOnlyRootFilesystem` | `true` | Read-only root filesystem |
| `ingress.enabled` | `false` | Enable Ingress resource |
| `ingress.className` | `""` | Ingress class name |
| `ingress.hosts` | `[]` | Ingress host rules |
| `ingress.tls` | `[]` | Ingress TLS configuration |

### TLS Termination

**Option 1: Ingress (recommended for Kubernetes).** Configure TLS at the Ingress level:

```yaml
# values.yaml
ingress:
  enabled: true
  className: nginx
  hosts:
    - host: mantle.company.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: mantle-tls
      hosts:
        - mantle.company.com
```

**Option 2: Native TLS.** Mantle can terminate TLS directly:

```bash
mantle serve --tls-cert /etc/mantle/tls.crt --tls-key /etc/mantle/tls.key
```

Or via environment variables:

```bash
export MANTLE_TLS_CERT="/etc/mantle/tls.crt"
export MANTLE_TLS_KEY="/etc/mantle/tls.key"
```

### Database Migrations

The Helm chart includes a pre-install/pre-upgrade hook Job that runs `mantle init` before the new version starts serving traffic. You do not need to run migrations separately. The migration job uses database-level locking, so it is safe with multiple replicas.

## Configuration Checklist

Use this checklist when deploying to production. Every item maps to an environment variable or `mantle.yaml` field.

| Setting | Env Var | Required | Notes |
|---|---|---|---|
| Database URL | `MANTLE_DATABASE_URL` | Yes | Use `sslmode=require` in production |
| Encryption key | `MANTLE_ENCRYPTION_KEY` | Yes | 32 bytes, hex-encoded (64 characters) |
| API listen address | `MANTLE_API_ADDRESS` | No | Default `:8080` |
| TLS certificate | `MANTLE_TLS_CERT` | No | Path to PEM-encoded certificate |
| TLS private key | `MANTLE_TLS_KEY` | No | Path to PEM-encoded private key |
| OIDC issuer URL | `MANTLE_AUTH_OIDC_ISSUER_URL` | No | Required if using SSO |
| OIDC client ID | `MANTLE_AUTH_OIDC_CLIENT_ID` | No | Required if using SSO |
| OIDC audience | `MANTLE_AUTH_OIDC_AUDIENCE` | No | Required if using SSO |
| OIDC allowed domains | `MANTLE_AUTH_OIDC_ALLOWED_DOMAINS` | No | Comma-separated domain list |
| AWS region | `AWS_REGION` | No | Required if using AWS Secrets Manager |
| GCP project | `MANTLE_GCP_PROJECT` | No | Required if using GCP Secret Manager |
| Azure vault URL | `MANTLE_AZURE_VAULT_URL` | No | Required if using Azure Key Vault |
| Execution retention | `MANTLE_RETENTION_EXECUTION_DAYS` | No | Default `90`. Days to keep completed executions |
| Audit retention | `MANTLE_RETENTION_AUDIT_DAYS` | No | Default `365`. Days to keep audit events |
| Allowed AI base URLs | `MANTLE_AI_ALLOWED_BASE_URLS` | No | Comma-separated. Restricts which AI endpoints connectors can call |
| Log level | `MANTLE_LOG_LEVEL` | No | Default `info`. Options: `debug`, `info`, `warn`, `error` |

## Production Hardening

### Enable TLS

Never run Mantle over plain HTTP in production. Use one of:

- Ingress with TLS termination (see [TLS Termination](#tls-termination) above)
- Native TLS with `--tls-cert` and `--tls-key`
- A reverse proxy (nginx, Caddy, HAProxy) that terminates TLS

### Set Resource Limits

Always set CPU and memory limits to prevent a single workflow execution from consuming all available resources:

```yaml
# values.yaml
resources:
  requests:
    cpu: 250m
    memory: 256Mi
  limits:
    cpu: "1"
    memory: 1Gi
```

### Configure PodDisruptionBudget

For high availability, enable the PDB to ensure at least one replica stays available during node maintenance:

```yaml
# values.yaml
replicaCount: 3
pdb:
  enabled: true
  minAvailable: 1
```

### Set Retention Policies

Configure retention to prevent unbounded storage growth:

```yaml
# mantle.yaml
retention:
  execution_days: 90
  audit_days: 365
```

Or via environment variables:

```bash
export MANTLE_RETENTION_EXECUTION_DAYS=90
export MANTLE_RETENTION_AUDIT_DAYS=365
```

Mantle runs a background cleanup job that deletes completed executions and audit events older than the configured retention period.

### Restrict AI Base URLs

If your workflows use the AI connector, restrict which endpoints can be called to prevent exfiltration:

```yaml
# mantle.yaml
ai:
  allowed_base_urls:
    - "https://api.openai.com"
    - "https://api.anthropic.com"
```

Requests to any other base URL are rejected at execution time.

### Monitor with Prometheus

Scrape the `/metrics` endpoint with Prometheus or a compatible collector:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: mantle
    static_configs:
      - targets: ["mantle:8080"]
```

See the [Observability Guide](observability-guide.md) for metric names, example PromQL queries, and Grafana dashboard configuration.

### Set Up Database Backups

Postgres is Mantle's single point of state. Set up automated backups:

- **Managed Postgres (RDS, Cloud SQL, Azure Database):** Enable automated snapshots and WAL archiving
- **Self-hosted Postgres:** Schedule `pg_dump` and configure WAL archiving

See the [Server Guide](server-guide.md#backup-and-disaster-recovery) for detailed backup procedures, recovery steps, and RPO/RTO guidance.

### Security Context

The Helm chart defaults to a secure pod configuration:

```yaml
securityContext:
  runAsNonRoot: true
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop:
      - ALL
```

Do not override these defaults unless you have a specific requirement.

## Further Reading

- [Getting Started](getting-started.md) -- first workflow in under five minutes
- [Server Guide](server-guide.md) -- triggers, REST API, graceful shutdown, backup and recovery
- [Authentication & RBAC Guide](authentication-guide.md) -- API keys, OIDC/SSO, roles, and teams
- [Secrets Guide](secrets-guide.md) -- credential encryption, cloud backends, key rotation
- [Configuration](configuration.md) -- full configuration reference
- [Observability Guide](observability-guide.md) -- Prometheus metrics, audit trail, structured logging
