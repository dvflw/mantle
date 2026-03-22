# Contributing to Mantle

Thank you for your interest in contributing to Mantle! This document explains how to get involved.

## Reporting Bugs

Open a [GitHub Issue](https://github.com/dvflw/mantle/issues) with:
- Mantle version (`mantle version`)
- Steps to reproduce
- Expected vs actual behavior
- Relevant logs or error messages

## Suggesting Features

Open a [GitHub Issue](https://github.com/dvflw/mantle/issues) with the `enhancement` label. Describe the use case, not just the solution.

## Development Setup

### Prerequisites

- Go 1.25+
- Docker (for local Postgres via docker-compose)
- Make

### Getting Started

```bash
git clone https://github.com/dvflw/mantle.git
cd mantle
docker-compose up -d    # Start Postgres
make build              # Build binary
make test               # Run tests
make lint               # Run linter
```

### Running Tests

```bash
make test               # Unit + integration tests
go test ./internal/...  # All internal packages
go test -race ./...     # With race detector
```

Integration tests use [testcontainers](https://testcontainers.com/) and require Docker.

## Code Style

- Run `gofmt` and `golangci-lint` before committing
- Follow existing patterns in the codebase
- Keep functions focused and files small
- Add tests for new functionality

## Pull Request Process

1. Fork the repository
2. Create a feature branch from `main`
3. Write tests first (TDD encouraged)
4. Ensure `make test` and `make lint` pass
5. Use conventional commit messages:
   - `feat:` new feature
   - `fix:` bug fix
   - `docs:` documentation
   - `refactor:` code change that neither fixes nor adds
   - `test:` adding or updating tests
   - `chore:` maintenance
6. Open a PR against `main` with a clear description

## Testing Requirements

- New code should include unit tests
- Database-dependent tests should use testcontainers
- Connector tests should use mock HTTP servers
- Aim for test coverage on all non-trivial logic

## License

By contributing, you agree that your contributions will be licensed under the project's [Business Source License 1.1](LICENSE).
