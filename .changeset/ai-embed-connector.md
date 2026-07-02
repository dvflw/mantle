---
"@mantle/engine": minor
---

Add an `ai/embed` connector for text embeddings, enabling retrieval-augmented generation (RAG). It mirrors `ai/completion`'s provider selection, base-URL allowlist, metrics, and token-budget accounting, and supports OpenAI and OpenAI-compatible endpoints (Azure OpenAI, local servers) via `base_url`. Output includes a pgvector text literal (`output.vector`) so embeddings drop straight into a `postgres/query` arg cast with `::vector`. Ships with a pgvector RAG example (`rag-ingest.yaml`, `rag-ask.yaml`, `rag-kb-schema.sql`) and a guide. Bedrock embeddings and native `kb/*` / chunking are planned follow-ups (#153).
