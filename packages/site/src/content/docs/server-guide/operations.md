# Operations

This page covers production deployment, backup and disaster recovery, and change management for Mantle.

## Production Deployment

### Using the Helm Chart

The recommended way to run Mantle in production is with the included Helm chart. It configures health probes, resource limits, and environment variables:

```bash
helm install mantle charts/mantle \
  --set database.url="postgres://mantle:secret@db.internal:5432/mantle?sslmode=require" \
  --set encryption.key="your-hex-key"
```

The Helm chart configures:

- Liveness probe pointing to `/healthz`
- Readiness probe pointing to `/readyz`
- `SIGTERM` as the termination signal (aligns with Mantle's graceful shutdown)

### Health Probes

Configure your load balancer or orchestrator to use the health endpoints:

| Probe | Endpoint | Recommended Interval |
|---|---|---|
| Liveness | `GET /healthz` | 10s |
| Readiness | `GET /readyz` | 5s |

The readiness probe returns a non-200 status when the database connection is lost, which causes the load balancer to stop routing traffic to the unhealthy instance.

### Environment Variables

In production, pass configuration through environment variables rather than config files:

```bash
export MANTLE_DATABASE_URL="postgres://mantle:secret@db.internal:5432/mantle?sslmode=require"
export MANTLE_API_ADDRESS=":8080"
export MANTLE_ENCRYPTION_KEY="your-64-char-hex-key"
export MANTLE_LOG_LEVEL="warn"
mantle serve
```

See [Configuration](/docs/configuration) for the full list of environment variables.

### Migrations

The server runs migrations automatically on startup. You do not need a separate `mantle init` step in your deployment pipeline. This is safe to run with multiple replicas -- migrations use database-level locking to prevent conflicts.

## Backup and Disaster Recovery

Postgres is Mantle's single point of state. All workflow definitions, execution history, step checkpoints, encrypted credentials, and audit events live in the database. Recovery from any failure depends on having a good database backup and access to your encryption key.

### What to Back Up

| Asset | Location | Notes |
|---|---|---|
| Postgres database | Your database host | Contains all Mantle state: definitions, executions, credentials, audit events |
| Encryption key | `MANTLE_ENCRYPTION_KEY` env var or `mantle.yaml` | Required to decrypt credentials; store separately from database backups |
| `mantle.yaml` | Configuration file on disk | Can be reconstructed, but easier to back up |
| Workflow YAML files | Version control (Git) | Authoritative source; can be re-applied with `mantle apply` |

**Critical:** Store the encryption key separately from database backups. If an attacker obtains both the database dump and the encryption key, they can decrypt all stored credentials.

### Recommended Backup Approach

**Managed Postgres (RDS, Cloud SQL, Azure Database):**

- Enable automated daily snapshots with your cloud provider
- Enable WAL archiving (point-in-time recovery) for near-zero RPO
- Retain snapshots according to your compliance requirements

**Self-hosted Postgres:**

- Schedule `pg_dump` on a cron job (e.g., daily or hourly depending on RPO requirements):

```bash
pg_dump -Fc -h localhost -U mantle mantle > /backups/mantle-$(date +%Y%m%d-%H%M%S).dump
```

- Configure WAL archiving for continuous point-in-time recovery
- Ship backups to off-site storage (S3, GCS, or another region)

### Recovery Procedure

1. **Restore Postgres from backup.** Use your cloud provider's restore flow or `pg_restore`:

```bash
pg_restore -h localhost -U mantle -d mantle /backups/mantle-20260322-120000.dump
```

2. **Verify migration state.** Run `mantle init` to confirm all migrations are applied:

```bash
mantle init
```

3. **Verify credentials.** Confirm the encryption key matches the one used when the backup was taken:

```bash
mantle secrets list
```

4. **Resume the server:**

```bash
mantle serve
```

5. **Re-apply workflow YAML files if needed.** Workflow definitions are stored in Postgres, so a database restore brings them back. However, if you need to apply changes that were made after the backup was taken, re-apply from version control:

```bash
mantle apply workflows/*.yaml
```

### RPO and RTO Guidance

| Backup Strategy | Recovery Point Objective (RPO) | Recovery Time Objective (RTO) |
|---|---|---|
| Daily `pg_dump` | Up to 24 hours of data loss | Minutes (restore + restart) |
| Hourly `pg_dump` | Up to 1 hour of data loss | Minutes |
| WAL archiving (PITR) | Near-zero (seconds of data loss) | Minutes to restore to a point in time |
| Managed snapshots + WAL | Near-zero | Depends on cloud provider restore time |

For production deployments, WAL archiving combined with periodic base backups gives the best balance of RPO and operational simplicity.

## Change Management

All changes to Mantle's codebase and deployment follow a controlled process.

1. **Pull request review.** All changes go through PR review on GitHub. At least one approval is required before merging to `main`.
2. **CI must pass.** Every PR runs the full CI pipeline: `go test`, `go vet`, `golangci-lint`, `govulncheck`, and `gosec`. PRs with failing checks are not merged.
3. **Production deployments.** Deploy via Helm with migration hooks. The Helm chart runs `mantle init` as a pre-install/pre-upgrade hook to apply database migrations before the new binary starts serving traffic.
4. **Rollback.** If a deployment introduces a problem, roll back with `helm rollback` and verify the migration state:

```bash
helm rollback mantle <revision>
mantle migrate status
```

Mantle migrations are forward-only by design. Rolling back the binary to an older version is safe as long as the database schema is compatible. Check `mantle migrate status` to confirm.
