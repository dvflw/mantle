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

## Setting Up Email Triggers

An email trigger polls an email mailbox and executes a workflow each time a new message matching the filter arrives. The trigger runs continuously when Mantle is in server mode.

```yaml
name: email-inbox-triage
description: AI-powered email classification and organization

triggers:
  - type: email
    mailbox: company-inbox
    folder: INBOX
    filter: unseen
    poll_interval: 30s

steps:
  - name: classify
    action: ai/completion
    credential: my-openai
    params:
      model: gpt-4o
      system_prompt: "Classify this email as: important, actionable, newsletter, or spam."
      prompt: |
        From: {{ trigger.from }}
        Subject: {{ trigger.subject }}
        Body: {{ trigger.body }}
      output_schema:
        type: object
        properties:
          category:
            type: string
            enum: [important, actionable, newsletter, spam]
        required: [category]

  - name: move-email
    action: email/move
    credential: company-inbox
    params:
      uid: "{{ trigger.uid }}"
      target_folder: "{{ steps.classify.output.json.category }}"
```

Apply the workflow to start polling:

```bash
mantle apply email-inbox-triage.yaml
# Applied email-inbox-triage version 1
```

### Email Trigger Configuration

The `email` trigger type polls a mailbox for messages matching a filter and executes the workflow for each message found.

**Configuration:**

| Field | Type | Required | Description |
|---|---|---|---|
| `type` | string | Yes | Must be `email`. |
| `mailbox` | string | Yes | Credential name for the email account (IMAP-compatible). |
| `folder` | string | No | Folder to monitor (e.g., `INBOX`, `Archive`). Default: `INBOX`. |
| `filter` | string | No | Filter messages: `all`, `unseen`, `recent`, `flagged`. Default: `unseen`. |
| `poll_interval` | string | No | How often to check for new messages (e.g., `30s`, `5m`). Default: `60s`. |

### Trigger Context Variables

When an email trigger fires, the following variables are available in CEL expressions and template strings:

| Variable | Type | Description |
|---|---|---|
| `trigger.message_id` | string | Unique message ID. |
| `trigger.uid` | number | IMAP UID (for use with email actions). |
| `trigger.from` | string | Sender email address. |
| `trigger.to` | string | Primary recipient(s). |
| `trigger.cc` | string | CC recipients. |
| `trigger.subject` | string | Message subject. |
| `trigger.body` | string | Message body (plaintext). |
| `trigger.date` | string | Message date (RFC 3339 timestamp). |
| `trigger.headers` | map | Full message headers. |
| `trigger.flags` | array | IMAP flags (e.g., `["seen", "flagged"]`). |

**Example -- conditional logic based on sender:**

```yaml
steps:
  - name: handle-vip-email
    action: ai/completion
    credential: my-openai
    if: "trigger.from == 'ceo@company.com'"
    params:
      model: gpt-4o
      prompt: "This is from the CEO. Provide an executive summary:\n\n{{ trigger.body }}"

  - name: notify-team
    action: slack/send
    credential: slack-bot
    if: "trigger.from == 'ceo@company.com'"
    params:
      channel: "#executive-inbox"
      text: "New email from {{ trigger.from }}: {{ trigger.subject }}"
```

### At-Least-Once Delivery

The email trigger marks messages as seen **after** firing the workflow for each message. If the Mantle process crashes or is restarted mid-poll, messages that were fetched but not yet marked may be re-triggered on the next poll cycle. This means email-triggered workflows may receive the same message more than once.

Design email-triggered workflows to be idempotent. A reliable approach is to deduplicate on `trigger.message_id`:

```yaml
steps:
  - name: check-duplicate
    action: postgres/query
    credential: my-database
    params:
      query: "INSERT INTO processed_emails (message_id) VALUES ($1) ON CONFLICT DO NOTHING"
      args:
        - "{{ trigger.message_id }}"

  - name: process-email
    action: ai/completion
    if: "steps['check-duplicate'].output.rows_affected > 0"
    # ... rest of workflow
```

### Connection Management

Email triggers maintain persistent IMAP connections to reduce authentication overhead. By default, Mantle pools up to 5 concurrent connections per mailbox credential. This limit is a compile-time default in v0.3.0 and is not runtime-configurable via `mantle.yaml`.

### Provider Limits

Email providers have different concurrency limits. Plan your poll intervals accordingly:

| Provider | Max Concurrent | Recommendation |
|---|---|---|
| Gmail | 15 | Up to 15 concurrent connections; use `poll_interval: 30s` for many workflows |
| Microsoft 365 | 20 | Higher concurrency limit; safe for aggressive polling |
| Generic IMAP | Varies | Check with your email host; conservative: 5 concurrent |

**Example -- multiple workflows, Gmail:**

If you have 3 email workflows on Gmail, each connecting once per poll:

```yaml
# workflow-1.yaml
triggers:
  - type: email
    mailbox: my-gmail
    folder: INBOX
    poll_interval: 60s  # Check every minute

# workflow-2.yaml
triggers:
  - type: email
    mailbox: my-gmail
    folder: Archive
    poll_interval: 60s

# workflow-3.yaml
triggers:
  - type: email
    mailbox: my-gmail
    folder: Drafts
    poll_interval: 60s
```

Each workflow gets its own connection, but they share the pool. If latency is a concern, stagger poll intervals.
