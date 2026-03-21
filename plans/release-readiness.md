# Mantle V1 Release Readiness Plan

> Compiled 2026-03-21 from 12 specialist reviews + security/compliance validation pass.
> Reviews: Brand Guardian, UX Architect, Product Manager, Reality Checker, Performance Benchmarker, Tool Evaluator, Workflow Optimizer, Infrastructure Maintainer, Compliance Auditor, Developer Advocate, Governance Architect, Technical Writer.
> Validated by: Security Engineer + Compliance Auditor (cross-referencing for conflicts and missing controls).

## Overall Assessment: B- (NEEDS WORK, but close)

The codebase is substantially more complete than most V1 projects. All 6 planned phases plus several V1.1 features are implemented. Documentation is strong. The gaps are concentrated in: legal/community files, compliance hardening, observability, CLI polish, and a few correctness bugs.

---

## Tier 0: Hard Blockers (cannot launch without these)

| # | Finding | Source | Effort |
|---|---------|--------|--------|
| 1 | **No LICENSE file** — code is legally "all rights reserved" | Brand, PM, DevRel, Writer | 30 min |
| 2 | **No CONTRIBUTING.md** | Brand, PM, DevRel, Writer | 2 hr |
| 3 | **No SECURITY.md** | Brand, PM, Compliance, Writer | 1 hr |
| 4 | **No CODE_OF_CONDUCT.md** | Brand, DevRel | 30 min |
| 5 | **No CHANGELOG.md** | PM, DevRel, Writer | 1 hr |
| 6 | **Cron scheduler fires on ALL replicas** — correctness bug in multi-replica deployments | Infra | 1-2 days |
| 7 | **`ClaimAnyStep` has no team_id filter** — cross-tenant step execution | Compliance | 1 day |
| 8 | **`trigger.payload` not declared in CEL environment** — webhook workflows broken | Workflow | 1 hr |
| 9 | **CLI `logs` command queries without team_id** — data isolation bypass | Reality | 30 min |
| 10a | **`handleCancel` cancels in-memory context before team_id DB check** — cross-tenant DoS | Security Validation | 1 hr |
| 10b | **`LookupWebhookTrigger` has no team_id filter** — cross-tenant webhook trigger | Security Validation | 1 hr |
| 10c | **`SyncTriggers` deletes triggers by name only** — cross-tenant trigger deletion | Security Validation | 1 hr |
| 10d | **`ListCronTriggers` loads ALL tenants' cron triggers** — cross-tenant execution | Security Validation | 1 hr |
| 10e | **Cascade API key deletion on user removal** — orphaned keys stay valid | Compliance Validation | 30 min |

## Tier 1: Must Fix Before Launch (high adoption/trust impact)

### Security & Compliance

| # | Finding | Source | Effort |
|---|---------|--------|--------|
| 10 | **API server has no TLS** — only `ListenAndServe`, no `ListenAndServeTLS` | Compliance | 2 days |
| 11 | **DB default is `sslmode=disable`** — unencrypted connections | Compliance, Infra | 1 day |
| 12 | **API keys have no expiry or revocation** — no `expires_at`, no `RevokeAPIKey` | Compliance | 3 days |
| 13 | **No rate limiting on any endpoint** — brute force + DoS risk | Compliance, Governance, Reality | 2 days |
| 14 | **Webhook endpoints have no HMAC signature verification** | Compliance | 2 days |
| 15 | **Missing audit events** for user/key/credential lifecycle operations | Compliance | 3 days |
| 16 | **Sensitive data in logs** — OIDC errors may contain tokens, AI errors may contain API responses | Compliance | 2 days |

### Observability

| # | Finding | Source | Effort |
|---|---------|--------|--------|
| 17 | **No HTTP request metrics** — no request duration, count, or error rate | Infra | 1 day |
| 18 | **No execution lifecycle metrics** — no `mantle_executions_total` by status/duration | Infra | 1 day |
| 19 | **No AI/LLM token metrics** — need `mantle_ai_tokens_total{workflow,step,model,provider,token_type}`, `mantle_ai_requests_total`, `mantle_ai_request_duration_seconds` | User, Governance, Perf | 2 days |

### CLI UX

| # | Finding | Source | Effort |
|---|---------|--------|--------|
| 20 | **`mantle run` doesn't show step outputs** — opaque after completion | UX | 1 day |
| 21 | **`orderedSteps` returns non-deterministic order** (Go map iteration) | UX | 30 min |
| 22 | **`validate` calls `os.Exit(1)` directly** — untestable, bypasses Cobra | UX | 30 min |
| 23 | **No `--output json` flag** on any command | UX | 2 days |
| 24 | **Root help doesn't group commands** — 20+ flat list is overwhelming | UX | 1 day |

### Documentation

| # | Finding | Source | Effort |
|---|---------|--------|--------|
| 25 | **`mantle login/logout/teams/users/roles` not in CLI Reference** | Writer | 3 hr |
| 26 | **Auth/OIDC config fields not in Configuration reference** | Writer | 2 hr |
| 27 | **No Authentication & RBAC Guide** | Writer | 4 hr |
| 28 | **No Deployment Guide** (Docker/Helm/binary consolidated) | Writer | 4 hr |
| 29 | **`provider` field missing from ai/completion docs** | Writer | 30 min |
| 30 | **Go version inconsistency** — README says 1.22+, Getting Started says 1.25+ | Writer, PM, DevRel | 5 min |

### Infrastructure

| # | Finding | Source | Effort |
|---|---------|--------|--------|
| 31 | **No Ingress template** in Helm chart | Infra | 1 day |
| 32 | **No HPA template** in Helm chart | Infra | 1 day |
| 33 | **No ServiceMonitor/pod annotations** for Prometheus discovery | Infra | 1 day |
| 34 | **No `.dockerignore`** — build context includes `.git/`, plans, etc. | Infra, Reality | 30 min |

## Tier 2: Should Fix Soon After Launch

### Governance & Cost Control

| # | Finding | Source | Effort |
|---|---------|--------|--------|
| 35 | Retry x tool-loop multiplicative blast radius — token budget resets per retry | Governance | 1 day |
| 36 | No model allowlist — any workflow can use any model | Governance | 1 day |
| 37 | No `base_url` restriction — workflows could point at rogue proxies | Governance | 1 day |
| 38 | Admin-enforced ceiling on `max_rounds` in config | Governance | 1 day |
| 39 | OpenAI 429 not classified as retryable | Governance | 1 day |
| 40 | No data retention / execution cleanup | Compliance | 3 days |

### Workflow Gaps

| # | Finding | Source | Effort |
|---|---------|--------|--------|
| 41 | **No `for_each`/iteration primitive** — blocks batch processing | Workflow, Tools | Large (design needed) |
| 42 | **No input default values** — all inputs implicitly required | Workflow | 1 day |
| 43 | **No on-failure/finally hooks** in workflows | Workflow | Medium |
| 44 | **No workflow-level timeout** | Workflow | 1 day |
| 45 | **CEL expressions not compiled during validation** | Workflow | 1 day |
| 46 | **No connector param validation at validate time** | Workflow | 2 days |
| 47 | **Step-level line numbers missing in validation errors** | Workflow | 1 day |

### Performance

| # | Finding | Source | Effort |
|---|---------|--------|--------|
| 48 | Cache workflow definitions in-process (eliminate per-step DB reload) | Perf | 1 day |
| 49 | Batch metadata queries in MakeGlobalStepExecutor | Perf | 1 day |
| 50 | Add composite index for loadCompletedSteps | Perf | 30 min |
| 51 | Increase `max_idle_conns` default to match `max_open_conns` | Perf | 30 min |
| 52 | Add basic benchmarks (CEL, claim, step round-trip, concurrent workers) | Perf | 3 days |

### Content & Community

| # | Finding | Source | Effort |
|---|---------|--------|--------|
| 53 | **No logo or visual identity** in repo | Brand | External |
| 54 | **No GitHub social preview image** | Brand | 1 hr |
| 55 | **No asciinema/terminal recording** in README | DevRel | 1 hr |
| 56 | **No AI tool-use example** in examples/ | DevRel | 1 hr |
| 57 | **No comparison docs** (vs Temporal, n8n, LangChain) | Brand, Tools, DevRel | 1 day each |
| 58 | **No hosted docs site** | DevRel | 1 day (setup) |
| 59 | GitHub issue templates, PR template, Discussions | DevRel | 1 hr |

## Tier 3: Future (post-launch roadmap)

| # | Finding | Source |
|---|---------|--------|
| 60 | Sub-workflows / workflow composition | Workflow, Tools |
| 61 | Human-in-the-loop approval gates | Workflow, Governance |
| 62 | Wait/delay step primitive | Workflow |
| 63 | KMS-backed envelope encryption | Compliance |
| 64 | OpenTelemetry distributed tracing | Infra |
| 65 | LISTEN/NOTIFY instead of polling | Perf |
| 66 | Terraform provider for Mantle resources | Tools |
| 67 | TypeScript/Python client SDKs | DevRel |
| 68 | Plugin registry | Tools |

---

## Recommended Execution Order

### Sprint 1 (3 days): Legal + Bugs + Quick Wins

Items: 1-5, 6-9, 10a-10e, 21-22, 30, 34, 50-51

- Legal files (LICENSE, CONTRIBUTING, SECURITY, CODE_OF_CONDUCT, CHANGELOG)
- Fix cron duplicate execution (leader election via pg advisory lock)
- Fix ClaimAnyStep cross-tenant leak
- Fix trigger queries team_id (LookupWebhookTrigger, SyncTriggers, ListCronTriggers)
- Fix handleCancel in-memory cancel ordering (team_id check BEFORE context cancel)
- Cascade API key deletion on user removal
- Fix trigger.payload CEL variable
- Fix CLI logs team_id, orderedSteps sort, validate os.Exit
- Fix Go version in README, add .dockerignore
- Fix JSON injection in error responses (fmt.Sprintf → json.Marshal)
- DB index + connection pool tuning

### Sprint 2 (7 days): Security Hardening + Compliance Foundation

Items: 10-16, 37, 40 + new compliance items

- TLS support (or documented reverse proxy requirement with startup warning)
- DB sslmode enforcement (warn/error on sslmode=disable)
- API key expiry + revocation + notification before expiry
- Rate limiting middleware (auth endpoints + API + webhooks)
- Webhook auth model: HMAC verification when secret configured, Bearer auth otherwise
- `base_url` restriction/allowlist (moved from Sprint 5 — credential exfiltration vector)
- Audit events for ALL admin operations (user/key/credential/team CRUD + auth.failed)
- Audit events scoped by team_id (add column + filter to Query)
- Add auth to `/metrics` endpoint (remove from middleware skip list)
- Log sanitization (errors, OIDC tokens, AI API responses)
- API body size limits on non-webhook endpoints
- Data retention/execution cleanup (configurable `retention_days`)
- Document: key rotation procedure, backup/DR requirements, data residency, change management

### Sprint 3 (5 days): Observability + CLI + Metrics

Items: 17-20, 23-24, 19

- HTTP request metrics middleware
- Execution lifecycle metrics
- AI/LLM token metrics (`mantle_ai_tokens_total`, `mantle_ai_requests_total`, `mantle_ai_request_duration_seconds`) with labels for workflow, step, model, provider, token_type
- `mantle run --verbose` with step outputs
- `--output json` on core commands
- Command grouping in root help

### Sprint 4 (5 days): Documentation + Infrastructure

Items: 25-29, 31-33, 27-28

- Document login/logout/teams/users/roles in CLI Reference
- Auth/OIDC config in Configuration reference
- Authentication & RBAC Guide
- Deployment Guide
- Helm: Ingress, HPA, ServiceMonitor templates
- Fix provider field and status_code inconsistencies

### Sprint 5 (5 days): Governance + Workflow + Content

Items: 35-36, 38-39, 42, 44-45, 53-59

- Cross-retry token budget, model allowlist, max_rounds ceiling (base_url moved to Sprint 2)
- OpenAI 429 retryable classification
- Input default values, workflow-level timeout, CEL validation
- Logo, social preview, terminal recording, tool-use example
- Comparison docs, GitHub templates, docs site setup

---

## Key Metrics for Release Readiness

| Metric | Current | Target |
|--------|---------|--------|
| Tier 0 blockers | 9 | 0 |
| Tier 1 items | 25 | 0 |
| Test files / source files | 37% | 50%+ |
| CLI commands documented | 15/20 | 20/20 |
| Benchmark coverage | 0 | 5+ core benchmarks |
| E2E integration tests | 0 | 1+ |
| Legal files present | 0/5 | 5/5 |

---

## Positioning Summary (from Brand + Tools reviews)

**One-liner:** "Define AI workflows as YAML. Deploy them like infrastructure. Run them anywhere."

**Target persona:** Platform engineers and SREs who already use Terraform and manage K8s clusters.

**Key differentiators:**
1. IaC lifecycle (validate/plan/apply) — unique in the category
2. Single binary + Postgres — simplest ops footprint
3. BYOK with AI tool use — no vendor lock-in, no proxy fees
4. Checkpoint-and-resume — production reliability without Temporal's complexity

**Positioning:** "Terraform for workflow automation, with native AI."
