-- Vector knowledge base for the RAG example (rag-ingest.yaml / rag-ask.yaml).
--
-- Semantic retrieval with pgvector, driven by the kb/upsert and kb/query
-- connectors. Requires the pgvector extension
-- (https://github.com/pgvector/pgvector). Point a Mantle postgres credential at
-- this database and reference it as `credential: kb-db`.
--
-- The embedding dimension must match the model: text-embedding-3-small = 1536,
-- text-embedding-3-large = 3072, Bedrock titan-embed-text-v2 = 1024 (default).
-- Adjust vector(1536) if you change models.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS kb_documents (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    content     TEXT NOT NULL,
    embedding   vector(1536) NOT NULL,
    -- Arbitrary per-document metadata (title, source, ...), written by
    -- kb/upsert's `metadata` param and returned by kb/query's `columns`.
    metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Idempotency: re-ingesting identical content is a no-op via kb/upsert's
    -- conflict_target (dedupe_key).
    dedupe_key  TEXT GENERATED ALWAYS AS (md5(content)) STORED,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_documents_dedupe
    ON kb_documents (dedupe_key);

-- Approximate-nearest-neighbour index for cosine distance (pgvector >= 0.5),
-- which is kb/query's default metric. For small corpora you can omit this and
-- rely on an exact scan. If you query with metric: l2 or inner_product, add a
-- matching index (vector_l2_ops / vector_ip_ops) or those queries seq-scan.
CREATE INDEX IF NOT EXISTS idx_kb_documents_embedding
    ON kb_documents USING hnsw (embedding vector_cosine_ops);
