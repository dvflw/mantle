---
title: Office-assistant MVP (meeting notes + Q&A)
---

A minimal "office assistant" built from two manually-run workflows and a Postgres
knowledge base:

- **`office-assistant-ingest-meeting`** — paste a meeting transcript; an LLM
  summarizes it and extracts action items, the notes are stored in the KB, a Jira
  issue is opened for the action items, and the summary is posted to Teams.
- **`office-assistant-ask`** — ask a question; the KB is full-text searched for
  the most relevant notes and an LLM synthesizes a grounded, cited answer.

Cross-run state lives in Postgres, so ingesting builds up a corpus that asking
reads back. Both workflows are in
[`packages/site/src/content/examples/`](https://github.com/dvflw/mantle/tree/main/packages/site/src/content/examples).

## Prerequisites

Create these credentials (`mantle secrets create`) and the KB schema:

| Credential name | Type | Used for |
| --------------- | ---- | -------- |
| `openai` | `openai` | `ai/completion` (summarize + answer). Swap the `credential:` and `model:` for `bedrock` to use Claude via Bedrock. |
| `kb-db` | `postgres` | The knowledge-base database |
| `jira` | `basic` | `jira/create_issue` (email + API token) |
| `teams` | `generic` | `teams/send_message` (incoming-webhook URL) |

Apply the KB schema (pure Postgres full-text search, no extensions) from
[`office-assistant-kb-schema.sql`](https://github.com/dvflw/mantle/blob/main/packages/site/src/content/examples/office-assistant-kb-schema.sql):

```bash
psql "$KB_DATABASE_URL" -f office-assistant-kb-schema.sql
```

Then apply both workflows:

```bash
mantle apply office-assistant-ingest-meeting.yaml
mantle apply office-assistant-ask.yaml
```

## Ingesting a meeting

Put the transcript in a values file (a YAML block scalar handles long,
multi-line text cleanly — there is no input size cap on manual runs):

```yaml
# meeting.values.yaml
inputs:
  title: "Q3 strategy sync"
  meeting_date: "2026-07-01"
  attendees: "Michael, CTO, Team A lead"
  transcript: |
    <paste the full transcript here — as many lines as you like>
```

```bash
mantle run office-assistant-ingest-meeting --values meeting.values.yaml
```

The run summarizes the transcript, stores the note, opens a Jira action-item
issue (skipped if the LLM found none), and posts the summary to Teams.

## Asking a question

```bash
mantle run office-assistant-ask \
  --input question="Who else is working with client C?" \
  --output json
```

The `search` step ranks matching notes with `websearch_to_tsquery`, and the
`answer` step's `output.text` is the grounded answer (the CLI prints step
outputs with `--output json` or `-v`). To send the answer somewhere instead of
reading it from the CLI, add a final `teams/send_message` step.

## What you own, and the caveats

This is a deliberately small v0 that fits what the engine does today. Known
trade-offs:

- **Retrieval is full-text search, not semantic.** The KB uses a Postgres
  `tsvector`; there are no embeddings. It's the simplest thing that works
  end-to-end with stock connectors. A native embeddings + vector-store retrieval
  layer is tracked by [#153](https://github.com/dvflw/mantle/issues/153); the
  schema comment shows the upgrade path.
- **One Jira issue per meeting**, not one per action item. The engine has no
  loop / `for_each` construct, so the workflow opens a single checklist issue
  rather than fanning out.
- **Manual transcription.** You paste a transcript; the bot does not join
  meetings or transcribe audio (tracked by
  [#154](https://github.com/dvflw/mantle/issues/154)).
- **Request/response, not a live Teams chat.** You ask via the CLI (or API) and
  optionally post the answer out; there is no conversational in-Teams bot
  (tracked by [#155](https://github.com/dvflw/mantle/issues/155)).
- **Confluence / GitHub actions** aren't included here but drop in as extra
  `http/request` steps (or the native `jira`/`linear` connectors already used).

See [#161](https://github.com/dvflw/mantle/issues/161) for the full MVP write-up
and how these pieces map to the larger office-assistant vision.
