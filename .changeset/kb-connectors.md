---
"@mantle/engine": minor
---

Add `kb/upsert` and `kb/query` connectors for retrieval-augmented generation. They are thin pgvector helpers over a Postgres database (the step's `credential`) that compose with `ai/embed`: `kb/upsert` stores document text + embedding (+ optional JSONB metadata), taking `ai/embed`'s pgvector literal directly and supporting single or batch (chunk) inserts with idempotent `ON CONFLICT`; `kb/query` runs distance-ordered nearest-neighbour search (cosine/l2/inner_product) and returns the closest rows. Table and column names are validated as SQL identifiers to prevent injection. The RAG example (`rag-ingest.yaml`, `rag-ask.yaml`, `rag-kb-schema.sql`) and guide now use these connectors. They do not manage schema (you create the pgvector table); native chunking and metadata filtering remain follow-ups on #153.
