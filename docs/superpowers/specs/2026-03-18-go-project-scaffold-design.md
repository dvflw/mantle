# Design: Go Project Scaffold and CLI Framework

> Linear issue: [DVFLW-218](https://linear.app/dvflw/issue/DVFLW-218/go-project-scaffold-and-cli-framework)
> Date: 2026-03-18

## Goal

Initialize the Go project with module, directory structure, CLI framework (Cobra), and a working `mantle version` command. This is the foundation everything else builds on.

## Acceptance Criteria

- `mantle version` prints version info (version, commit, build date)
- Project structure follows Go conventions (`cmd/`, `internal/`)
- CLI framework supports subcommands with help text

## Project Structure

```
cmd/mantle/
  main.go              # Entrypoint: initializes root command, calls Execute()
internal/
  cli/
    root.go            # Root cobra command, global flags (--config)
    version.go         # `mantle version` subcommand
  version/
    version.go         # Version vars (Version, Commit, Date) set via ldflags
```

Only what's needed for `mantle version`. Future issues add packages under `internal/` as needed (engine/, config/, workflow/, connector/, secret/, api/, audit/).

## Module Path

`github.com/dvflw/mantle`

## Version Output

Default (dev build):
```
mantle dev (none, built 2026-03-18T00:00:00Z)
```

With ldflags (release build):
```
mantle v0.1.0 (abc1234, built 2026-03-18T15:30:00Z)
```

Three variables injected via `-ldflags -X`:
- `Version` — git tag or `dev`
- `Commit` — git short SHA or `none`
- `Date` — RFC3339 build timestamp

## Build

Plain dev build:
```bash
go build ./cmd/mantle
```

Release build (Makefile handles this):
```bash
go build -ldflags "-X github.com/dvflw/mantle/internal/version.Version=$(git describe --tags) \
  -X github.com/dvflw/mantle/internal/version.Commit=$(git rev-parse --short HEAD) \
  -X github.com/dvflw/mantle/internal/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  ./cmd/mantle
```

## Makefile

Targets:
- `build` — builds binary with git info injected via ldflags
- `test` — runs `go test ./...`
- `lint` — runs `golangci-lint run`
- `clean` — removes built binary

## Dependencies

- `github.com/spf13/cobra` — CLI framework (per CLAUDE.md)

No other dependencies.

## What's NOT Included

- Config loading (`mantle.yaml`) — DVFLW-272
- docker-compose — DVFLW-273
- Health endpoints — DVFLW-274
- CI pipeline — DVFLW-275
- Any subcommands beyond `version` — added by their respective issues
