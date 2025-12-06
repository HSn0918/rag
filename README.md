# RAG System

[中文](README_ZH.md) | English

Retrieval-Augmented Generation system for documents, built in Go with a Connect/gRPC API and a Next.js (Bun) frontend.

## Features

- **Document pipeline**: Presigned upload → PDF text extraction (Doc2X) → semantic chunking → embeddings → pgvector storage
- **Hybrid retrieval**: Vector search + keyword-aware rerank, then LLM summarization
- **Clients**: Connect/gRPC; generated Go/TypeScript stubs; Bun/Next.js demo UI with virtual scrolling
- **Infra**: PostgreSQL + pgvector, Redis cache, MinIO object storage
- **Ops**: `just gen` for proto codegen via Docker, `just web-*` for frontend tasks

## Architecture

The system follows a modular architecture with clean separation of concerns:

```
cmd/server/          # Application entry point
internal/
├── adapters/        # Database adapters (PostgreSQL)
├── clients/         # External service clients
│   ├── base/        # Shared HTTP client utilities
│   ├── doc2x/       # Document parsing service
│   ├── embedding/   # Embedding generation
│   ├── openai/      # LLM integration
│   └── rerank/      # Document reranking
├── config/          # Configuration management
├── logger/          # Structured logging
├── redis/           # Cache layer
├── server/          # HTTP server and handlers
└── storage/         # File storage (MinIO)
```

## Requirements

- Go 1.21+ (recommended)
- PostgreSQL with pgvector
- Redis
- MinIO (or S3-compatible) for object storage
- Docker (for `just gen` codegen) and Bun (for the Next.js frontend)

## Quickstart

1) Clone & Go deps
```bash
git clone https://github.com/hsn0918/rag.git
cd rag
go mod download
```

2) Configure: copy `config.example.yaml` to `config.yaml` and fill in DB/Redis/MinIO/API keys.

3) Generate code (requires Docker):
```bash
just gen
```

4) Run server:
```bash
go run cmd/server/main.go
```

5) Frontend (Bun + Next.js):
```bash
cd web
bun install
bun run dev
```

## Configuration

Use `config.yaml` (see `config.example.yaml`) or env vars:

- `server`: host/port
- `database`: PostgreSQL + pgvector DSN parts
- `redis`: host/port/auth
- `minio`: endpoint/access keys/bucket
- `services`: Doc2X, Embedding, Reranker, LLM endpoints + models/API keys
- `chunking`: chunk sizes/overlap/semantic options

## API (Connect/gRPC)

Service: `rag.v1.RagService`

- `POST /rag.v1.RagService/PreUpload` — presigned upload URL
- `POST /rag.v1.RagService/UploadPdf` — process & index PDF
- `POST /rag.v1.RagService/GetContext` — full RAG pipeline (keywords → embedding → search → rerank → summarize)
- `POST /rag.v1.RagService/ListDocuments` — cursor-paginated doc list
- `POST /rag.v1.RagService/DeleteDocument` — delete doc and chunks

See `api/rag/v1/rag.proto` for message shapes; generated clients in `internal/gen` (Go) and `web/gen` (TS).

## CLI & Scripts

- `just gen` — buf codegen (via Docker), fixes TS import extensions
- `just web-install|web-dev|web-build|web-start|web-lint` — frontend tasks (Bun)
- `go run cmd/server/main.go` — start backend

## Frontend (Next.js + Bun)

- Uses generated Connect TS client; proxy to backend via `web/next.config.mjs` (`/api -> BACKEND_URL`).
- Includes upload flow, RAG query, document list (cursor pagination, delete), keyword D3 visualization, and structured answer rendering.

## Notes

- For codegen, Docker access is required (bufbuild/buf container).
- For embedding/rerank/LLM services, update API keys and models in `config.yaml`.
- Ensure pgvector extension exists in Postgres and the DSN matches your config.

- **Database**: `github.com/jackc/pgx/v5` for PostgreSQL
- **Vector Search**: `github.com/pgvector/pgvector-go` for embeddings
- **Cache**: `github.com/redis/rueidis` for Redis
- **Storage**: `github.com/minio/minio-go/v7` for object storage
- **HTTP Client**: `github.com/go-resty/resty/v2` for external APIs
- **Dependency Injection**: `go.uber.org/fx` for application structure
- **Logging**: `log/slog` for structured logging

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

This project is licensed under the terms specified in the repository.
