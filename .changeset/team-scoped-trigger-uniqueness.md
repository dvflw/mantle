---
"@mantle/engine": patch
---

Make workflow trigger uniqueness multi-tenant-safe. The cron trigger uniqueness index is now scoped by team (`team_id, workflow_name, schedule`), so two teams can each register a workflow with the same name and schedule instead of the second team's apply failing with a unique-constraint violation. Webhook paths remain a global routing key (an inbound `POST /hooks/<path>` carries no team identity), and the webhook lookup is no longer team-scoped: it resolves by path and runs the workflow under the owning team from the matched trigger row, so webhooks registered by non-default teams now route correctly. Fixes #164.
