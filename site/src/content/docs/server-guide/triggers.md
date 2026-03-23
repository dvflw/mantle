# Triggers

Triggers define how a workflow is started automatically when Mantle runs in server mode. This page covers setting up cron and webhook triggers.

## Setting Up Cron Triggers

A cron trigger executes a workflow on a recurring schedule. Add a `triggers` section to your workflow YAML:

```yaml
name: daily-report
description: Generate and email a daily summary report

triggers:
  - type: cron
    schedule: "0 9 * * 1-5"

steps:
  - name: fetch-metrics
    action: http/request
    params:
      method: GET
      url: https://api.internal.com/metrics/daily

  - name: summarize
    action: ai/completion
    credential: my-openai
    params:
      model: gpt-4o
      prompt: "Summarize these metrics into 5 bullet points: {{ steps.fetch-metrics.output.body }}"
```

Apply the workflow to register the trigger:

```bash
mantle apply daily-report.yaml
# Applied daily-report version 1
```

The cron scheduler picks up the trigger the next time it polls (within 30 seconds). The workflow runs every weekday at 9 AM.

### Cron Expression Syntax

The `schedule` field uses standard 5-field cron syntax:

```
minute (0-59)
hour (0-23)
day of month (1-31)
month (1-12)
day of week (0-6, Sunday=0)
```

Common patterns:

| Schedule | Expression |
|---|---|
| Every minute | `* * * * *` |
| Every 5 minutes | `*/5 * * * *` |
| Every hour on the hour | `0 * * * *` |
| Daily at midnight | `0 0 * * *` |
| Weekdays at 9 AM | `0 9 * * 1-5` |
| First of every month at noon | `0 12 1 * *` |
| Every 15 minutes during business hours | `*/15 9-17 * * 1-5` |

### Updating a Cron Schedule

Edit the `schedule` field in your YAML and re-apply:

```bash
# Change from every 5 minutes to every 15 minutes
mantle apply daily-report.yaml
# Applied daily-report version 2
```

The scheduler picks up the updated schedule on the next poll cycle.

### Removing a Cron Trigger

Delete the `triggers` section from the YAML (or remove the specific trigger entry) and re-apply:

```bash
mantle apply daily-report.yaml
# Applied daily-report version 3
```

The trigger is deregistered. The workflow is still available for manual execution with `mantle run` or the REST API.

## Setting Up Webhook Triggers

A webhook trigger executes a workflow when an HTTP POST request arrives at a configured path. The request body is available as `trigger.payload` in CEL expressions.

```yaml
name: deploy-notifier
description: Post a Slack notification when a deploy completes

triggers:
  - type: webhook
    path: "/hooks/deploy-notifier"

steps:
  - name: notify-slack
    action: http/request
    params:
      method: POST
      url: https://hooks.slack.com/services/T00/B00/xxx
      body:
        text: "Deployed {{ trigger.payload.repo }}@{{ trigger.payload.sha }} to {{ trigger.payload.environment }}"
```

Apply the workflow:

```bash
mantle apply deploy-notifier.yaml
# Applied deploy-notifier version 1
```

Trigger it from your CI pipeline or any HTTP client:

```bash
curl -X POST http://localhost:8080/hooks/deploy-notifier \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "my-app",
    "sha": "abc1234",
    "environment": "production"
  }'
```

The server starts a new execution and the full JSON body is available as `trigger.payload`.

### Accessing Webhook Payload Data

The `trigger.payload` variable contains the parsed JSON body. Access nested fields with dot notation in template strings or bracket notation in `if` expressions:

```yaml
# In template strings (params):
url: "{{ trigger.payload.callback_url }}"
prompt: "Analyze this event: {{ trigger.payload }}"

# In if expressions:
if: "trigger.payload.action == 'opened'"
```
