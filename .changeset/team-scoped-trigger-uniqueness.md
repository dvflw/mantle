---
"@mantle/engine": patch
---

Make workflow trigger uniqueness multi-tenant-safe. Both the cron uniqueness index (`team_id, workflow_name, schedule`) and the webhook path index (`team_id, path`) are now scoped by team, so two teams can each register a workflow with the same name/schedule or the same webhook path instead of the second team's apply failing with a unique-constraint violation. Webhook lookup stays scoped to the caller's authenticated team (the `/hooks/` endpoint is authenticated), so a caller can only trigger its own team's webhooks even when another team uses the same path. Email triggers have no uniqueness index and are unaffected. Fixes #164.
