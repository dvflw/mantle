# Docker-Compose Local Dev Setup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add docker-compose with Postgres, update Makefile with dev targets, update README, fix config default URL.

**Architecture:** Infrastructure files only — docker-compose.yml, Makefile updates, README updates, one config default change.

**Tech Stack:** Docker Compose, PostgreSQL 16, Make

**Spec:** `docs/superpowers/specs/2026-03-18-docker-compose-design.md`

**Linear issue:** [DVFLW-273](https://linear.app/dvflw/issue/DVFLW-273/docker-composeyml-for-local-development)

---

### Task 1: docker-compose.yml

**Files:**
- Create: `docker-compose.yml`

- [ ] **Step 1: Create docker-compose.yml**

Create `docker-compose.yml`:

```yaml
services:
  postgres:
    image: postgres:16-alpine
    ports:
      - "5432:5432"
    environment:
      POSTGRES_USER: mantle
      POSTGRES_PASSWORD: mantle
      POSTGRES_DB: mantle
    volumes:
      - pgdata:/var/lib/postgresql/data

volumes:
  pgdata:
```

- [ ] **Step 2: Commit**

```bash
git add docker-compose.yml
git commit -m "feat: add docker-compose.yml with Postgres for local dev"
```

---

### Task 2: Update config default URL

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

The default database URL must include the docker-compose credentials (`mantle:mantle@`).

- [ ] **Step 1: Update default in config.go**

In `internal/config/config.go`, change the default:

From: `v.SetDefault("database.url", "postgres://localhost:5432/mantle?sslmode=disable")`

To: `v.SetDefault("database.url", "postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable")`

- [ ] **Step 2: Update test expectations**

In `internal/config/config_test.go`, update all occurrences of the old default URL to the new one. There are two tests that check the default:

- `TestLoad_Defaults`: change expected `Database.URL`
- `TestLoad_ImplicitConfigMissing_UsesDefaults`: change expected `Database.URL`

Old: `postgres://localhost:5432/mantle?sslmode=disable`
New: `postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable`

- [ ] **Step 3: Run tests**

Run:
```bash
go test ./... -v
```

Expected: All tests pass with new default.

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "fix: update default database URL to match docker-compose credentials"
```

---

### Task 3: Update Makefile

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add new targets**

Add these targets to the existing Makefile (after the existing `clean` target):

```makefile
migrate:
	@echo "No migrations yet. Run 'mantle init' when available."

run:
	go run ./cmd/mantle $(ARGS)

dev:
	docker-compose up -d
```

Also update the `.PHONY` line to include the new targets:

From: `.PHONY: build test lint clean`
To: `.PHONY: build test lint clean migrate run dev`

- [ ] **Step 2: Verify targets work**

Run:
```bash
make migrate
```

Expected: Prints "No migrations yet. Run 'mantle init' when available."

Run:
```bash
make run -- version
```

Expected: Prints version info.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "feat: add migrate, run, dev targets to Makefile"
```

---

### Task 4: Update README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update Development section**

Replace the Development section in README.md with:

```markdown
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
```

- [ ] **Step 2: Update the Configuration section**

Update the example config in README.md to show the correct default database URL with credentials:

```yaml
database:
  url: postgres://mantle:mantle@localhost:5432/mantle?sslmode=disable
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: update README with local dev setup and Makefile commands"
```

---

### Task 5: Final verification

- [ ] **Step 1: Run all tests**

Run:
```bash
go test ./... -v
```

Expected: All tests pass.

- [ ] **Step 2: Build and verify**

Run:
```bash
make build && ./mantle version
```

Expected: Version output works.

- [ ] **Step 3: Verify Makefile targets**

Run:
```bash
make migrate
```

Expected: Prints placeholder message.

- [ ] **Step 4: Clean up**

Run:
```bash
make clean
```
