# Release Readiness Report

**Repository:** dvflw/mantle
**Date:** 2026-03-25
**Release:** v0.3.0
**Release scope:** v0.2.0..HEAD (25 commits, 39 files changed, 4890 insertions)

## Executive Summary

v0.3.0 adds three features: `continue_on_error` step-level error handling, email connectors with IMAP polling triggers, and browser automation via Playwright-in-Docker. The release is architecturally sound with no data-loss risks, no checkpoint model violations, and no critical security vulnerabilities. All tests pass, build is clean, vet is clean. Multiple documentation accuracy issues were found and fixed during the review. Two code-level fixes were applied (distributed CEL context gap, email fetch limit cap). The release is ready to ship with minor caveats documented below.

## Go/No-Go Recommendation

**Recommendation: CONDITIONAL GO**

**Conditions (addressed during review):**
- Distributed CEL context for `continue_on_error` was missing in `MakeGlobalStepExecutor` — **FIXED**
- Email receive limit cap to prevent OOM — **FIXED**
- Documentation accuracy (wrong output keys, fictional config, wrong defaults) — **FIXED**
- Metric for skipped email triggers — **FIXED**

**Remaining caveats (non-blocking, documented):**
- Email trigger uses at-least-once delivery — documented in trigger guide
- `email/send` STARTTLS not enforced (pre-existing, not new in v0.3.0) — tracked for v0.3.1
- Browser `script` field trust assumption needs explicit documentation — tracked for v0.3.1
- TypeScript support limited to type annotations only (no decorators/enums) — documented

## Findings Summary

| Agent | Verdict | Blockers | Warnings | Info |
|-------|---------|----------|----------|------|
| Security Engineer | PASS WITH WARNINGS | 0 | 2 | 3 |
| Code Reviewer | PASS WITH CONDITIONS | 0 | 4 | 3 |
| Software Architect | SHIP WITH ACTIONS | 0 | 2 | 14 |
| Technical Writer | BLOCK ON HIGH | 0 | 9 | 2 |
| Reality Checker | NEEDS WORK | 0 | 2 | 3 |

## Findings Addressed During Review

### Code Fixes Applied

1. **`MakeGlobalStepExecutor` CEL context gap** (Code Review WARNING)
   - `internal/engine/engine.go`: Both `MakeGlobalStepExecutor` and `MakeStepExecutor` now call `loadFailedContinuedSteps` and populate the CEL context with error/output for continued steps.

2. **`email/receive` limit cap** (Security WARNING)
   - `internal/connector/email_receive.go`: Added `maxEmailFetchLimit = 200` constant. Limits above 200 are silently capped.

3. **Double-close in `email_receive.go`** (Code Review WARNING)
   - Removed the `defer cmd.Close()` that conflicted with the explicit close.

4. **Silent trigger drop metric** (Reality Checker MEDIUM)
   - `internal/metrics/metrics.go`: Added `mantle_email_triggers_skipped_total` counter.
   - `internal/server/email_trigger.go`: Increments metric when a trigger is skipped.

### Documentation Fixes Applied

5. **Wrong output field names** (Tech Writer HIGH x3)
   - `email/move`: `success` -> `moved`
   - `email/delete`: `success` -> `deleted`
   - `email/flag`: `success` -> `updated`, added missing `action` field

6. **Wrong filter values** (Tech Writer HIGH)
   - Removed invalid `seen`, added missing `recent`, fixed default `all` -> `unseen`

7. **Fictional config keys** (Tech Writer LOW)
   - Replaced fake `engine.email.max_connections` YAML with accurate text (compile-time default of 5)

8. **Missing `email` trigger type** (Tech Writer MEDIUM)
   - Added `email` to workflow reference triggers section with all fields

9. **`_credential` in params tables** (Tech Writer MEDIUM)
   - Replaced with step-level `credential` authentication notes

10. **Python browser example** (Tech Writer MEDIUM)
    - Fixed to use pre-created `page` object from wrapper

11. **At-least-once delivery** (Reality Check + Architect)
    - Added "At-Least-Once Delivery" section to trigger docs with idempotency guidance

12. **`steps.error` reachability clarification** (Tech Writer MEDIUM)
    - Clarified only practically useful with `continue_on_error: true`

## Remaining Warnings (Non-blocking, tracked for v0.3.1)

### Security

- **F-01: Browser `script` field trust assumption** — User scripts are concatenated into wrapper templates. If CEL resolves external data into the `script` field, structural injection is possible inside the container sandbox. Recommend documenting the trust boundary or adding `{{ }}` template lint check.

- **F-04: `use_tls: false` accepted without warning** — IMAP plaintext mode should log a structured warning at dial time.

- **F-05: Error messages may leak infrastructure details** — `steps.<name>.error` contains raw connector errors. Document as a trust boundary consideration.

### Architecture

- **F-06: `email/send` STARTTLS not enforced** — Pre-existing: `smtp.SendMail` falls back to plaintext silently. Should enforce TLS to match IMAP's security posture. Track for v0.3.1.

- **F-03: `AdvanceExecution` hot-reload caveat** — `continue_on_error` resolved from in-memory steps, not DB. Workflow re-applied mid-execution may produce inconsistent behavior. Add godoc comment.

### Code Quality

- **Duplicated `imapConfig`/`imapDialConfig` across packages** — Export shared type or add field parity test.
- **Duplicated `buildSearchCriteria`/`buildEmailSearchCriteria`** — Consolidate into shared helper.
- **`log.Printf` in `email_delete.go`** — Should use `slog.Warn` for consistency.
- **TypeScript import limitations** — Document that only type annotations are supported (no decorators, enums, or npm imports).
- **`FindBodySection` Peek mismatch in email trigger** — Lookup key doesn't match fetch options; works via fallback but technically wrong.
- **Docker container hardening** — Missing `ReadOnlyRootFilesystem`, no seccomp profile, no non-root user.

## Test Evidence

```
go test ./... -short -count=1    ALL 19 PACKAGES PASS
go vet ./...                     CLEAN
go build ./cmd/mantle/           SUCCESS
Secrets audit                    CLEAN (only example AWS keys in tests)
TODO/FIXME audit                 CLEAN
```

## Appendix: Review Agents

- **Security Engineer** — Reviewed script injection, IMAP credentials, TLS enforcement, Docker security, secret leakage
- **Code Reviewer** — Reviewed correctness, error handling, resource cleanup, test coverage, code organization
- **Software Architect** — Reviewed checkpoint model, connection lifecycle, architectural patterns, migration safety
- **Technical Writer** — Cross-referenced all documentation against Go source code for accuracy
- **Reality Checker** — Ran actual build/test/vet commands, checked for secrets, audited TODOs
