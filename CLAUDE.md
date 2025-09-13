# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

```bash
# Run the server
go run cmd/server/main.go

# Install dependencies
go mod tidy
go mod download

# Generate protobuf/gRPC code
just gen
# Or manually:
docker run --rm -v "$(pwd):/workspace" -w /workspace bufbuild/buf generate --path api/proto/rag/v1

# Clean generated code
just clean

# Run with custom config
go run cmd/server/main.go --config=/path/to/config.yaml

# Build binary
go build -o rag-server cmd/server/main.go

# Docker compose for dependencies
docker-compose up -d  # Start PostgreSQL, Redis, MinIO
```

## Architecture Overview

This is a document-based RAG (Retrieval-Augmented Generation) system built with Go using dependency injection (Uber FX) and Connect RPC framework.

### Core Flow
1. **Document Upload**: PDF → Doc2X parsing → Text extraction → Chunking → Embedding generation → Store in PostgreSQL with pgvector
2. **Query Processing**: User query → LLM keyword extraction → Embedding generation → Vector similarity search → Re-ranking → LLM summary generation

### Key Components

**Server Layer** (`internal/server/`)
- `server.go`: Main RagServer struct implementing Connect RPC service
- `upload_pdf.go`: PDF processing pipeline - parses with Doc2X, chunks text, generates embeddings
- `get_context.go`: Query processing - keyword extraction, vector search, re-ranking, LLM summarization
- `modules.go`: Dependency injection module definitions using Uber FX

**Chunking System** (`internal/chunking/`)
- `OptimizedMarkdownChunker`: AST-based markdown parsing with semantic boundaries
- Key features: sparse parent merging, smart overlap, structure preservation
- Config: `max_chunk_size` (2000), `min_chunk_size` (500), `overlap_size` (200)

**External Service Clients** (`internal/clients/`)
- All clients inherit from `base.HTTPClient` with standardized error handling
- `doc2x/`: PDF to markdown conversion service
- `embedding/`: Vector embedding generation (supports multiple models)
- `openai/`: LLM integration for keyword extraction and summarization
- `rerank/`: Document re-ranking service

**Storage Layer**
- `internal/adapters/postgres.go`: PostgreSQL with pgvector for semantic search
- `internal/storage/minio.go`: MinIO/S3 for document storage
- `internal/redis/`: Caching layer for documents and embeddings

### Key Algorithms

**Semantic Search** (`get_context.go:103-116`)
- Uses cosine similarity with pgvector
- Default similarity threshold: 0.3
- Retrieves top 15 candidates for re-ranking

**Hybrid Re-ranking** (`get_context.go:360-398`)
- Weights: Vector similarity (40%), Keyword match (30%), Phrase match (20%), Content quality (10%)
- Filters results with similarity > 0.25

**Smart Chunking** (`chunking/markdown.go`)
- Builds document AST tree
- Detects semantic boundaries
- Merges sparse parent sections (< 20 tokens) with children
- Maintains relationships between chunks

### Configuration

Main config file: `config.yaml`
- Database: PostgreSQL connection settings
- Redis: Cache configuration
- MinIO: Object storage settings
- Services: API endpoints and keys for Doc2X, Embedding, Reranker, LLM
- Chunking: Size limits and boundary detection settings

### API Endpoints

All endpoints use Connect RPC protocol at base path `/rag.v1.RagService/`

1. `PreUpload`: Generate presigned URL for file upload
2. `UploadPdf`: Process PDF and store in vector database
3. `GetContext`: Query documents with semantic search

### Testing Approach

Since no test files exist yet, create tests following these patterns:
- Unit tests for chunking algorithms
- Integration tests for service clients
- End-to-end tests for the full RAG pipeline
- Mock external services using interfaces

### Common Issues and Solutions

1. **pgvector extension**: Ensure PostgreSQL has pgvector extension installed
2. **Embedding dimensions**: Must match model output (e.g., 4096 for Qwen3-Embedding-8B)
3. **Chinese text**: System supports Chinese with specialized keyword extraction and tokenization
4. **Memory usage**: Large PDFs may require chunking optimization