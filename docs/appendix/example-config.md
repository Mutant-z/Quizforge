# Example Config

```yaml
server:
  host: 0.0.0.0
  port: 8080

database:
  path: ./data/app.db
  wal: true
  busy_timeout_ms: 5000

upload:
  root: ./data
  chunk_size_mb: 5
  max_file_size_mb: 2048

worker:
  import_concurrency: 1
  llm_concurrency: 2
  embedding_concurrency: 2

agent:
  max_steps: 8

rag:
  fts_top_k: 20
  vector_top_k: 20
  final_top_k: 6

review:
  scheduler: simple_v1

security:
  api_key_master_key_env: APP_API_KEY_MASTER_KEY
```
