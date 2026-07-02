-- Vector knowledge base for the RAG example (rag-ingest.yaml / rag-ask.yaml).
--
-- Semantic retrieval with pgvector: documents are stored alongside an embedding
-- produced by the ai/embed connector, and queried by cosine distance. Requires
-- the pgvector extension (https://github.com/pgvector/pgvector). Point a Mantle
-- credential at this database and reference it as `credential: kb-db`.
--
-- The embedding dimension must match the model: text-embedding-3-small = 1536,
-- text-embedding-3-large = 3072. Adjust vector(1536) if you change models.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS kb_documents (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source      TEXT,
    title       TEXT,
    content     TEXT NOT NULL,
    embedding   vector(1536) NOT NULL,
    -- Idempotency: re-ingesting identical content is a no-op (see the
    -- ON CONFLICT clause in rag-ingest.yaml).
    dedupe_key  TEXT GENERATED ALWAYS AS (md5(content)) STORED,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_documents_dedupe
    ON kb_documents (dedupe_key);

-- Approximate-nearest-neighbour index for cosine distance (pgvector >= 0.5).
-- For small corpora you can omit this and rely on an exact scan.
CREATE INDEX IF NOT EXISTS idx_kb_documents_embedding
    ON kb_documents USING hnsw (embedding vector_cosine_ops);
