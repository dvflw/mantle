-- Knowledge base for the office-assistant MVP.
--
-- Used by office-assistant-ingest-meeting.yaml (writes) and
-- office-assistant-ask.yaml (reads). This is the v0 retrieval layer: plain
-- Postgres full-text search — no extensions, no embeddings. Point a Mantle
-- credential at this database and reference it as `credential: kb-db` in both
-- workflows.
--
-- Upgrade path: swap the `search` tsvector column + GIN index for a pgvector
-- embedding column and an ANN index once volume justifies semantic search
-- (tracked by dvflw/mantle#153).

CREATE TABLE IF NOT EXISTS meeting_notes (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title         TEXT NOT NULL,
    meeting_date  DATE,
    attendees     TEXT,
    summary       TEXT NOT NULL,
    action_items  JSONB NOT NULL DEFAULT '[]'::jsonb,
    topics        JSONB NOT NULL DEFAULT '[]'::jsonb,
    transcript    TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Weighted full-text index: title matches rank highest, then the summary,
    -- then the raw transcript. Regenerated automatically on write.
    search tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(summary, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(transcript, '')), 'C')
    ) STORED
);

CREATE INDEX IF NOT EXISTS idx_meeting_notes_search
    ON meeting_notes USING GIN (search);
