---
"@mantle/engine": patch
---

Register workflow triggers on apply. Cron, webhook, and email triggers declared in a workflow's YAML were parsed and validated but never written to `workflow_triggers`, so they never fired. `workflow.Save` now reconciles the table in the same transaction as the definition insert (covering both `mantle apply` and GitOps sync), pruned/disabled workflows have their triggers disabled so they stop firing, and the email poller periodically reloads so new email triggers activate without a server restart.
