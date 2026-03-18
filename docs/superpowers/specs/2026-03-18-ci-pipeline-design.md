# Design: CI/CD Pipeline — test, vet, lint

> Linear issue: [DVFLW-275](https://linear.app/dvflw/issue/DVFLW-275/cicd-pipeline-test-vet-lint)
> Date: 2026-03-18

## Goal

Add GitHub Actions CI pipeline that runs tests, vet, and lint on every push and PR. Add golangci-lint configuration. Add CI status badge to README.

## Acceptance Criteria

- `go test ./...` runs on every push and PR
- `go vet ./...` and `golangci-lint` run on every push and PR
- Integration tests run against Postgres via testcontainers (when added)
- CI fails on lint errors or test failures

## Files

- Create: `.github/workflows/ci.yml`
- Create: `.golangci.yml`
- Modify: `README.md` — add CI badge

## GitHub Actions Workflow

`.github/workflows/ci.yml` — triggered on push and pull_request to all branches.

Three parallel jobs on `ubuntu-latest` with Go 1.24:

### test
- Checkout, setup Go
- `go test -race ./...`
- Docker available by default for future testcontainers tests

### vet
- Checkout, setup Go
- `go vet ./...`

### lint
- Checkout, setup Go
- Uses `golangci/golangci-lint-action` with pinned version

## golangci-lint Config

`.golangci.yml` — conservative linter set that catches real bugs:

```yaml
run:
  timeout: 5m

linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - unused
    - ineffassign
    - gosimple
```

## README Badge

Add CI status badge at the top of README.md, right after the `# Mantle` heading:

```markdown
[![CI](https://github.com/dvflw/mantle/actions/workflows/ci.yml/badge.svg)](https://github.com/dvflw/mantle/actions/workflows/ci.yml)
```

## What's NOT Included

- Release/deploy workflows (Phase 4)
- Code coverage reporting
- Go module caching (add later if CI is slow)
