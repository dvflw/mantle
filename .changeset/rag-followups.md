---
"@mantle/engine": minor
---

RAG follow-ups on #153:

- **`kb/query` metadata filtering** — an optional `filter` object restricts the search to rows whose metadata contains it, via the JSONB containment operator (`metadata @> $2::jsonb`). The whole filter binds as a single parameter (no injection surface); a custom `metadata_column` is supported. Scope retrieval to a source, team, or tag alongside the vector search.
- **Separator-aware `recursive` chunking** — `text/chunk` gains `unit: recursive`, which walks a separator hierarchy (paragraph → line → sentence → word → character) so chunks break on natural boundaries instead of mid-sentence, then merges pieces up to `chunk_size` with `chunk_overlap` characters carried between chunks. The `rag-ingest` example now uses it.
- **Cohere Bedrock embeddings** — `ai/embed` (provider `bedrock`) now supports `cohere.embed-english-v3` and `cohere.embed-multilingual-v3` in addition to Amazon Titan. Cohere batches natively (up to 96 texts per call) and takes an `input_type` (`search_document` default / `search_query` / `classification` / `clustering`).

Token-accurate chunking remains a follow-up on #153.
