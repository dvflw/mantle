# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-03-22

### Added
- Core workflow engine with validate/plan/apply/run lifecycle
- HTTP, AI (OpenAI + Bedrock), Slack, Email, Postgres, S3 connectors
- AI tool-use loop with multi-turn function calling and crash recovery
- LLMProvider interface with OpenAI and Bedrock providers
- CEL expression engine for data passing and conditionals
- Checkpoint-and-resume execution with Postgres-backed state
- DAG-based parallel step execution with dependency resolution
- AES-256-GCM encrypted credential storage with key rotation
- Cloud secret backends (AWS Secrets Manager, GCP Secret Manager, Azure Key Vault)
- Multi-tenancy with teams, users, RBAC (admin/team_owner/operator)
- API key and OIDC/SSO authentication with token-sniffing middleware
- Cron and webhook triggers with server mode
- Distributed step execution with SKIP LOCKED work distribution
- Worker/reaper liveness tracking with health check integration
- Prometheus metrics and structured logging
- Helm chart with PDB, migration job, startup probe, security contexts
- CI pipeline with govulncheck, gosec, and Trivy scanning
- Docker multi-arch image build and push to GHCR
- Plugin system for custom connectors (JSON stdin/stdout protocol)
- Shared workflow library
- Cloud provider config (AWS/GCP/Azure regions)
- `mantle login` with auth code PKCE and device flow
