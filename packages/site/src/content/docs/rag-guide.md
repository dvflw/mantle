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

`ai/embed` returns each embedding as a **pgvector text literal**
(`output.vector`, e.g. `"[0.1,0.2,...]"`). The `kb/upsert` and `kb/query`
connectors take that literal and handle the pgvector SQL for you — the `::vector`
casts, the cosine-distance operator, `ORDER BY`/`LIMIT`, JSONB metadata, and
multi-row inserts:

```yaml
# ingest — store content + embedding + metadata
- action: kb/upsert
  credential: kb-db
  params:
    table: kb_documents
    content: "{{ inputs.content }}"
    vector: "{{ steps['embed'].output.vector }}"
    metadata: { title: "{{ inputs.title }}", source: "{{ inputs.source }}" }
    conflict_target: dedupe_key   # idempotent re-ingest

# ask — nearest-neighbour search
- action: kb/query
  credential: kb-db
  params:
    table: kb_documents
    vector: "{{ steps['embed-question'].output.vector }}"
    columns: [content, metadata]
    top_k: 5
```

`kb/query` returns the requested columns plus a `distance` field (lower = closer;
for cosine, similarity = `1 - distance`). The retrieved rows go to
`ai/completion` as JSON context, and the model answers using only those passages.

`kb/*` are thin sugar — they run against a pgvector database you provide (the
step `credential`) and don't manage schema, so you still create the table
yourself. If you'd rather write the SQL directly, `postgres/query` works too
(that's what these connectors generate).

## Ingesting documents

```bash
mantle run rag-ingest \
  --input title="Client C integration" \
  --input source="confluence/12345" \
  --input content="Team A connects to Client C via the X integration; Team B uses the direct API."
```

`rag-ingest` splits the document into overlapping chunks with `text/chunk`,
embeds them all in one `ai/embed` call, and stores them in one `kb/upsert`. A
single `metadata` object is broadcast to every chunk. Re-ingesting identical
content is a no-op (the schema dedupes on `md5(content)`).

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
  with the Amazon Titan (`amazon.titan-embed-text-v2:0`,
  `amazon.titan-embed-text-v1`) and Cohere Embed v3 (`cohere.embed-english-v3`,
  `cohere.embed-multilingual-v3`) text-embedding models. To use Bedrock, set
  `provider: bedrock`, a `region`, and an `aws` credential; adjust the schema's
  `vector(...)` dimension to match (Titan v2 defaults to 1024; Cohere v3 is
  1024). For Cohere, embed documents at ingest with `input_type:
  search_document` and questions with `input_type: search_query`; embed both
  sides with the same model. (Cohere's Bedrock response carries no token counts,
  so those embeddings report zero usage.)
- **You provide the vector store.** `kb/*` run against a pgvector database you
  create and point a credential at; they don't provision the extension or table.
- **Metadata filtering.** `kb/query` takes an optional `filter` object and
  restricts the search to rows whose metadata contains it (JSONB `@>`), so you
  can scope retrieval to a source, team, or tag alongside the vector search.
- **Chunking.** `text/chunk` offers fixed-size (`chars`/`words`) windows and a
  separator-aware `recursive` mode that prefers to break on paragraph, line,
  sentence, and word boundaries. Token-accurate chunking remains a follow-up on
  [#153](https://github.com/dvflw/mantle/issues/153).
