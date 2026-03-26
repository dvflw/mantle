# CLI Reference

Mantle is a single binary with subcommands for every stage of the workflow lifecycle. This page documents the global flags and general commands. See the subpages for command groups.

For installation instructions, see [Getting Started](/docs/getting-started).

## Global Flags

These flags are available on every command (except where noted):

| Flag | Env Var | Default | Description |
|---|---|---|---|
| `--config` | -- | `mantle.yaml` (in current directory) | Path to the configuration file. |
| `--database-url` | `MANTLE_DATABASE_URL` | `postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable` | Postgres connection URL. |
| `--api-address` | `MANTLE_API_ADDRESS` | `:8080` | API server listen address. |
| `--log-level` | `MANTLE_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error`. |
| `--api-key` | -- | -- | API key for authentication. Overrides cached credentials from `mantle login`. |
| `--output`, `-o` | -- | `text` | Output format: `text` or `json`. |

Flag precedence, highest to lowest: CLI flags, environment variables, config file, defaults. See [Configuration](/docs/configuration) for details.

---

## mantle version

Print version, commit hash, and build date. Does not require a database connection or config file.

```
Usage:
  mantle version
```

**Example:**

```bash
$ mantle version
mantle v0.1.0 (abc32ad, built 2026-03-18T00:00:00Z)
```

---

## mantle init

Run all pending database migrations to set up or upgrade the Mantle schema. This is the first command you run after installing Mantle and starting Postgres.

```
Usage:
  mantle init
```

**Example:**

```bash
$ mantle init
Running migrations...
Migrations complete.
```

**Errors:**

If Postgres is not running or the connection URL is wrong, you see:

```
Error: failed to connect to database: ...
```

Fix: verify Postgres is running (`docker-compose up -d`) and check your `--database-url` or `MANTLE_DATABASE_URL`.

---

## mantle migrate

Run pending database migrations. Functionally equivalent to `mantle init` but organized as a command group with subcommands for status and rollback.

```
Usage:
  mantle migrate
  mantle migrate status
  mantle migrate down
```

### mantle migrate status

Show which migrations have been applied and which are pending.

```bash
$ mantle migrate status
Applied At                      Migration
============================================================
2026-03-18 10:00:00 +0000       001_initial_schema.sql
```

### mantle migrate down

Roll back the most recently applied migration. Use with caution in production.

```bash
$ mantle migrate down
Rollback complete.
```

---

## Exit Codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Validation error, runtime error, or command failure |
