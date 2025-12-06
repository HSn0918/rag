# RAG System

[English](README.md) | [中文](README_ZH.md)

A document-based Retrieval-Augmented Generation system built with Go.

## Features

- **Document Processing**: PDF parsing and text extraction using Doc2X
- **Vector Storage**: PostgreSQL with pgvector for similarity search
- **Embedding Generation**: Multiple embedding models support
- **LLM Integration**: OpenAI-compatible API for text generation
- **Caching**: Redis for performance optimization
- **File Storage**: MinIO for document storage
- **Reranking**: Document relevance scoring

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

- Go 1.25+
- PostgreSQL with pgvector extension
- Redis
- MinIO (or S3-compatible storage)

## Installation

1. Clone the repository:
```bash
git clone https://github.com/hsn0918/rag.git
cd rag
```

2. Install dependencies:
```bash
go mod download
```

3. Set up your configuration (see Configuration section)

4. Run the server:
```bash
go run cmd/server/main.go
```

## Configuration

Configure the system using environment variables or configuration files. Key settings include:

- Database connections (PostgreSQL, Redis)
- External service endpoints and API keys
- Storage configuration (MinIO)
- Server port and settings

## API Endpoints

The system provides a gRPC/Connect API with the following endpoints:

### 1. Pre-upload File
Generate a presigned URL for direct file upload to storage.

**Endpoint**: `POST /rag.v1.RagService/PreUpload`

**Request**:
```json
{
  "filename": "document.pdf"
}
```

**Response**:
```json
{
  "upload_url": "https://minio.example.com/...",
  "file_key": "unique-file-key",
  "expires_in": 900
}
```

**Parameters**:
- `filename` (required): Original filename of the document

### 2. Upload Document
Process and index a PDF document that was uploaded to storage.

**Endpoint**: `POST /rag.v1.RagService/UploadPdf`

**Request**:
```json
{
  "file_key": "unique-file-key",
  "filename": "document.pdf"
}
```

**Response**:
```json
{
  "success": true,
  "message": "PDF processed successfully. Document ID: doc123, Chunks: 25",
  "document_id": "doc123"
}
```

**Parameters**:
- `file_key` (required): File key returned from PreUpload
- `filename` (required): Original filename

**Process**:
1. Downloads file from storage using the file key
2. Extracts text content using Doc2X service
3. Splits content into semantic chunks
4. Generates embeddings for each chunk
5. Stores document and chunks in PostgreSQL with pgvector

### 3. Query Context
Retrieve relevant information based on natural language queries.

**Endpoint**: `POST /rag.v1.RagService/GetContext`

**Request**:
```json
{
  "query": "How to implement machine learning algorithms?"
}
```

**Response**:
```json
{
  "context": "Based on your query about implementing machine learning algorithms...\n\n## Relevant Information\n\n1. Algorithm Selection...\n2. Implementation Steps..."
}
```

**Parameters**:
- `query` (required): Natural language query in Chinese or English

**Process**:
1. Uses LLM to extract keywords from the query
2. Generates embedding vector for semantic search
3. Searches similar document chunks using pgvector
4. Re-ranks results using hybrid scoring (similarity + keyword matching)
5. Uses LLM to generate intelligent summary from retrieved chunks

## Configuration Parameters

### Chunking Configuration
- `max_chunk_size`: Maximum size of text chunks (default: 1000)
- `overlap_size`: Overlap between chunks (default: 100)
- `adaptive_size`: Enable adaptive chunk sizing
- `paragraph_boundary`: Respect paragraph boundaries
- `sentence_boundary`: Respect sentence boundaries

### Embedding Configuration
- Supported models: BGE-Large, BGE-M3, Qwen3-Embedding series
- Dimensions: Model-specific (768-4096)
- Token limits: 512-32768 depending on model

### Search Configuration
- `similarity_threshold`: Minimum similarity score (0.25-0.4)
- `max_results`: Maximum number of results (5-15)
- `rerank_enabled`: Enable intelligent re-ranking

## Response Formats

### Error Response
All endpoints return standard Connect/gRPC errors:

```json
{
  "code": "INVALID_ARGUMENT",
  "message": "filename is required"
}
```

### Success Indicators
- Upload operations return `success: true` with processing details
- Query operations return structured context with similarity scores
- All responses include relevant metadata and usage tips

## Dependencies

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
