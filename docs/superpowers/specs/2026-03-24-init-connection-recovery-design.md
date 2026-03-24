# `mantle init` Connection Recovery & Quickstart Fix

**Date:** 2026-03-24
**Issue:** [#7 — Get Running in 5 Minutes](https://github.com/dvflw/mantle/issues/7)
**Status:** Draft

## Problem

The landing page quickstart tells users to run `docker compose up -d` after installing via `go install`. There's no `docker-compose.yml` when you install that way — step 2 immediately fails. The `mantle init` command needs to handle the "no database yet" case gracefully.

## Design

### Connection Recovery Flow

`mantle init` already loads config and calls `db.Open`. The change adds a recovery path when the connection fails:

```
mantle init
  ├─ db.Open succeeds → run migrations → done
  └─ db.Open fails
       ├─ host is NOT loopback → print error with details, suggest --database-url / env var, exit 1
       └─ host IS loopback → offer Docker auto-provisioning
            ├─ user accepts
            │    ├─ docker available → start container, wait for ready, run migrations → done
            │    └─ docker unavailable → show message, offer [R]etry or [C]onnection string
            └─ user declines → offer [R]etry or [C]onnection string
```

### Loopback Detection

Parse the host from the configured database URL. Treat as loopback if the host is:
- `localhost`
- `127.0.0.1`
- `::1`

Use `net/url` to parse the connection string and extract the host.

### Docker Auto-Provisioning

When the user accepts Docker provisioning:

1. Check Docker availability: exec `docker info` and check exit code
2. Run the container:
   ```
   docker run -d \
     --name mantle-postgres \
     -p 5432:5432 \
     -e POSTGRES_USER=mantle \
     -e POSTGRES_PASSWORD=mantle \
     -e POSTGRES_DB=mantle \
     -v mantle-pgdata:/var/lib/postgresql/data \
     postgres:16-alpine
   ```
3. Wait for readiness: poll `db.Open` with backoff (up to ~15s)
4. On success: continue to migrations
5. On timeout: error with "Container started but Postgres isn't accepting connections"

Use `os/exec` to run Docker commands. The container config matches the existing defaults in `config.go` so no config persistence is needed.

If the container name `mantle-postgres` already exists (stopped), remove it first and start fresh. If it's already running, skip straight to the readiness check.

### Fallback: No Docker / User Declined

Present two options:
```
Can't auto-provision — Docker isn't installed or isn't running.

  [R] Retry (install or start Docker first)
  [C] Enter a Postgres connection string

Choice [R/c]:
```

- **Retry**: loop back to Docker availability check
- **Connection string**: prompt for URL, validate with `db.Open`, on success continue to migrations, on failure show the error and re-prompt

### Non-Loopback Failure

When the configured URL points to a remote host and the connection fails:
```
Failed to connect to database at db.example.com:5432

  Error: connection refused

Check that the database is running and accessible.
Override with --database-url or MANTLE_DATABASE_URL.
```

Include the underlying error from `db.Open` (timeout, auth failure, TLS, DNS resolution, etc.) so the user can diagnose without guessing.

### Interactive Input

Follow the existing pattern from `login.go`: use `fmt.Fscanln(cmd.InOrStdin(), &input)` for prompts. No new dependencies needed.

When stdin is not a terminal (piped input, CI), skip all interactive prompts and return the connection error directly. Detect with `os.Stdin.Stat()` checking for `ModeCharDevice`.

## Files Changed

### Modified

| File | Change |
|------|--------|
| `internal/cli/init.go` | Add connection recovery flow, Docker provisioning, interactive prompts |
| `site/src/components/GetStarted.astro` | Remove `docker compose up -d` from step 2, simplify to just `mantle init` |
| `site/src/content/docs/getting-started/index.md` | Update quickstart to remove Docker prerequisite, explain `mantle init` handles DB setup |

### New

| File | Purpose |
|------|---------|
| `internal/cli/docker.go` | Docker availability check, container start, readiness polling — keeps init.go focused |
| `internal/cli/init_test.go` | Tests for loopback detection, non-interactive fallback, prompt flow (mocked stdin/exec) |
| `internal/cli/docker_test.go` | Tests for Docker command construction, container name conflict handling |

## Non-Goals

- **Config file generation**: `mantle init` does not create `mantle.yaml`. The defaults work with the Docker container.
- **Docker Compose**: we use `docker run`, not `docker compose`. No dependency on a compose file.
- **Custom port/user/password in Docker flow**: always matches defaults. Users who need custom config can use the connection string prompt.
- **Container lifecycle management**: `mantle init` starts the container; it doesn't stop or remove it. Users manage that themselves.

## Testing Strategy

- **Loopback detection**: unit test `isLoopback` with localhost, 127.0.0.1, ::1, remote hosts, IPv6
- **Non-interactive detection**: unit test that piped stdin skips prompts and returns error
- **Docker command construction**: verify the exact `docker run` args match defaults
- **Integration**: testcontainers already covers the migration path; the new code paths are the interactive/Docker shell-out portions which are unit-tested with mocked exec
- **Site content**: manual verification that quickstart steps are accurate
