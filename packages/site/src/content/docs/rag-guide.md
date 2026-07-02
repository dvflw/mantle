---
title: Retrieval-augmented generation (RAG)
---

Build a semantic knowledge base and answer questions grounded in it, using the
`ai/embed` connector, a pgvector table, and `ai/completion`. Two example
workflows tie it together:

- **`rag-ingest`** — embed a document and store it in the vector store.
- **`rag-ask`** — embed a question, find the nearest documents by cosine
  distance, and have an LLM answer grounded in them (with citations).

This is semantic retrieval — matches are by meaning, not keywords. For a simpler
keyword-only variant with no extensions, see the
[office-assistant MVP](/docs/office-assistant-mvp), which uses Postgres
full-text search.

## Prerequisites

- Credentials: an `openai` credential (referenced as `openai`) for `ai/embed`
  and `ai/completion`, and a `postgres` credential (referenced as `kb-db`) for
  the vector store.
- A Postgres database with the [pgvector](https://github.com/pgvector/pgvector)
  extension. Apply the schema from
  [`rag-kb-schema.sql`](https://github.com/dvflw/mantle/blob/main/packages/site/src/content/examples/rag-kb-schema.sql):

```bash
psql "$KB_DATABASE_URL" -f packages/site/src/content/examples/rag-kb-schema.sql
```

Apply both workflows:

```bash
mantle apply packages/site/src/content/examples/rag-ingest.yaml
mantle apply packages/site/src/content/examples/rag-ask.yaml
```

## How it works

`ai/embed` returns each embedding both as a float array (`output.embedding`) and
as a **pgvector text literal** (`output.vector`, e.g. `"[0.1,0.2,...]"`). The
literal binds straight into a `::vector` column, so storing and querying vectors
needs no special encoding:

```yaml
# ingest
args:
  - "{{ inputs.content }}"
  - "{{ steps['embed'].output.vector }}"   # → $2::vector
```

```sql
-- ask: nearest neighbours by cosine distance (<=> is pgvector's operator)
SELECT content, 1 - (embedding <=> $1::vector) AS similarity
FROM kb_documents
ORDER BY embedding <=> $1::vector
LIMIT 5
```

The retrieved rows are passed to `ai/completion` as JSON context, and the model
answers using only those passages.

## Ingesting documents

```bash
mantle run rag-ingest \
  --input title="Client C integration" \
  --input source="confluence/12345" \
  --input content="Team A connects to Client C via the X integration; Team B uses the direct API."
```

For a long document, split it into chunks and run `rag-ingest` once per chunk.
Re-ingesting identical content is a no-op (the schema dedupes on `md5(content)`).

## Asking questions

```bash
mantle run rag-ask \
  --input question="Who works with Client C and how?" \
  --output json
```

The `answer` step's `output.text` is the grounded response (the CLI prints step
outputs with `--output json` or `-v`).

## Notes and limitations

- **Embedding dimension must match the model.** The schema uses `vector(1536)`
  for `text-embedding-3-small`; use `vector(3072)` for `text-embedding-3-large`.
  Embed queries and documents with the same model.
- **Provider support:** `ai/embed` supports `openai` (and OpenAI-compatible
  endpoints like Azure OpenAI or local servers via `base_url`) and `bedrock`
  with the Amazon Titan text-embedding models (`amazon.titan-embed-text-v2:0`,
  `amazon.titan-embed-text-v1`). To use Bedrock, set `provider: bedrock`, a
  `region`, and an `aws` credential; adjust the schema's `vector(...)` dimension
  to match (Titan v2 defaults to 1024). Cohere Bedrock models are a follow-up.
- **No native chunking or `kb/*` convenience steps yet.** You compose RAG from
  `ai/embed` + `postgres/query` + `ai/completion` today; native chunking and
  `kb/upsert` / `kb/query` connectors are tracked by
  [#153](https://github.com/dvflw/mantle/issues/153).
