package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

// VectorDB 定义了向量数据库操作的接口。
type VectorDB interface {
	Store(content string, embedding []float32) error
	Search(embedding []float32, k int) ([]string, error)
	StoreDocument(ctx context.Context, title, content string, metadata map[string]interface{}) (string, error)
	StoreChunk(ctx context.Context, docID string, chunkIndex int, content string, embedding []float32, metadata map[string]interface{}) error
}

// PostgresVectorDB 实现了 VectorDB 接口，使用 PostgreSQL 和 pgvector。
type PostgresVectorDB struct {
	conn *pgx.Conn
}

// NewPostgresVectorDB 创建并返回一个新的 PostgresVectorDB 实例。
func NewPostgresVectorDB(dsn string, dimensions int) (*PostgresVectorDB, error) {
	ctx := context.Background()

	// 1. 连接到 PostgreSQL 数据库
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("无法连接到数据库: %w", err)
	}

	// 2. 检查连接是否成功
	if err = conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("数据库 ping 失败: %w", err)
	}

	log.Println("成功连接到 PostgreSQL 数据库。")

	// 3. 启用 pgvector 扩展
	_, err = conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector;")
	if err != nil {
		return nil, fmt.Errorf("无法启用 vector 扩展: %w", err)
	}
	log.Println("pgvector 扩展已启用。")

	// 4. 创建文档表和文档块表
	createDocumentsTable := `
	CREATE TABLE IF NOT EXISTS rag_documents (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		title TEXT NOT NULL,
		content TEXT NOT NULL,
		metadata JSONB DEFAULT '{}',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	);`

	createChunksTable := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS document_chunks (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		document_id UUID NOT NULL REFERENCES rag_documents(id) ON DELETE CASCADE,
		chunk_index INTEGER NOT NULL,
		content TEXT NOT NULL,
		embedding vector(%d),
		metadata JSONB DEFAULT '{}',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		UNIQUE(document_id, chunk_index)
	);`, dimensions)

	// 创建表
	_, err = conn.Exec(ctx, createDocumentsTable)
	if err != nil {
		return nil, fmt.Errorf("无法创建 rag_documents 表: %w", err)
	}

	_, err = conn.Exec(ctx, createChunksTable)
	if err != nil {
		return nil, fmt.Errorf("无法创建 document_chunks 表: %w", err)
	}
	log.Println("rag_documents 和 document_chunks 表已准备就绪。")

	return &PostgresVectorDB{conn: conn}, nil
}

// Store 将内容和其对应的向量嵌入存储到数据库中。(已废弃 - 使用 StoreChunk 代替)
func (db *PostgresVectorDB) Store(content string, embedding []float32) error {
	return fmt.Errorf("Store method is deprecated, use StoreChunk instead")
}

// Search 在数据库中搜索与给定向量最相似的 k 个结果。(已废弃 - 需要重新实现)
func (db *PostgresVectorDB) Search(embedding []float32, k int) ([]string, error) {
	return nil, fmt.Errorf("Search method needs to be reimplemented for new schema")
}

// StoreDocument 存储文档并返回文档ID
func (db *PostgresVectorDB) StoreDocument(ctx context.Context, title, content string, metadata map[string]interface{}) (string, error) {
	docID := uuid.New().String()

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("序列化 metadata 失败: %w", err)
	}

	_, err = db.conn.Exec(ctx,
		"INSERT INTO rag_documents (id, title, content, metadata) VALUES ($1, $2, $3, $4)",
		docID, title, content, metadataJSON)
	if err != nil {
		return "", fmt.Errorf("存储文档失败: %w", err)
	}

	return docID, nil
}

// StoreChunk 存储文档块和对应的向量
func (db *PostgresVectorDB) StoreChunk(ctx context.Context, docID string, chunkIndex int, content string, embedding []float32, metadata map[string]interface{}) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("序列化 metadata 失败: %w", err)
	}

	_, err = db.conn.Exec(ctx,
		"INSERT INTO document_chunks (document_id, chunk_index, content, embedding, metadata) VALUES ($1, $2, $3, $4, $5)",
		docID, chunkIndex, content, pgvector.NewVector(embedding), metadataJSON)
	if err != nil {
		return fmt.Errorf("存储文档块失败: %w", err)
	}

	return nil
}
