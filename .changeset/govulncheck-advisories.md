---
"@mantle/engine": patch
---

Resolve new govulncheck advisories. Upgrade `github.com/jackc/pgx/v5` to v5.9.2, which fixes GO-2026-5004 (SQL injection via the non-default simple protocol with dollar-quoted literals). Add the three unfixable docker/docker `docker cp`/archive advisories (GO-2026-5617, GO-2026-5668, GO-2026-5746) to the CI govulncheck exclusion list, alongside the existing docker exclusions — docker/docker has no patched release (only moby/moby/v2), tracked by #151.
