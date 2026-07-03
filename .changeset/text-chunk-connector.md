---
"@mantle/engine": minor
---

Add a `text/chunk` connector that splits a long document into overlapping chunks for embedding, completing the RAG ingest pipeline: `text/chunk` → `ai/embed` (batch) → `kb/upsert` (batch). Chunk by characters (Unicode-aware) or words with configurable size and overlap; `output.chunks` feeds straight into `ai/embed`'s `input` and `kb/upsert`'s `contents`. Also enhances `kb/upsert` so a single `metadata` object is broadcast to every row of a batch (so chunked ingest can share one title/source). The RAG example now chunks documents; smarter (separator-aware/token) chunking and `kb/query` metadata filtering remain follow-ups on #153.
