---
"@mantle/engine": minor
---

Add Bedrock support to the `ai/embed` connector via the Amazon Titan text-embedding models (`amazon.titan-embed-text-v2:0`, `amazon.titan-embed-text-v1`). Set `provider: bedrock` with a `region` and an `aws` credential; the connector uses Bedrock `InvokeModel` (embedding one input per call, reassembled in order) and honors `dimensions` on Titan v2. `serve` wires the AWS region/config into the embeddings connector alongside the chat connector. Cohere Bedrock embedding models are a planned follow-up.
