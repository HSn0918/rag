package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hsn0918/rag/pkg/logger"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"go.uber.org/zap"
)

const (
	createDocumentsTableTemplate = `
	CREATE TABLE IF NOT EXISTS %s (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		title TEXT NOT NULL,
		minio_key TEXT NOT NULL,
		metadata JSONB DEFAULT '{}',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	);`

	createChunksTableTemplate = `
	CREATE TABLE IF NOT EXISTS %s (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		document_id UUID NOT NULL REFERENCES %s(id) ON DELETE CASCADE,
		chunk_index INTEGER NOT NULL,
		content TEXT NOT NULL,
		embedding vector(%d),
		metadata JSONB DEFAULT '{}',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		UNIQUE(document_id, chunk_index)
	);`

	createDocumentsTitleIndexTemplate = `
	CREATE INDEX IF NOT EXISTS idx_gin_documents_title_%dd ON %s USING GIN (to_tsvector('chinese_zh', title));`

	createChunksContentIndexTemplate = `
	CREATE INDEX IF NOT EXISTS idx_gin_chunks_content_%dd ON %s USING GIN (to_tsvector('chinese_zh', content));`

	insertDocumentTemplate = `INSERT INTO %s (id, title, minio_key, metadata) VALUES ($1, $2, $3, $4)`
	insertChunkTemplate    = `INSERT INTO %s (document_id, chunk_index, content, embedding, metadata) VALUES ($1, $2, $3, $4, $5)`
	searchChunksTemplate   = `
		SELECT
			c.id as chunk_id,
			c.document_id,
			c.content,
			1 - (c.embedding <=> $1) as similarity,
			c.metadata
		FROM %s c
		WHERE 1 - (c.embedding <=> $1) > $2
		ORDER BY c.embedding <=> $1
		LIMIT $3`
)

// ChunkSearchResult 表示分块搜索结果
type ChunkSearchResult struct {
	ChunkID    string                 `json:"chunk_id"`
	DocumentID string                 `json:"document_id"`
	Content    string                 `json:"content"`
	Similarity float32                `json:"similarity"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// VectorDB 定义了向量数据库操作的接口。
type VectorDB interface {
	StoreDocument(ctx context.Context, title, minioKey string, metadata map[string]interface{}) (string, error)
	StoreChunk(ctx context.Context, docID string, chunkIndex int, content string, embedding []float32, metadata map[string]interface{}) error
	SearchSimilarChunks(ctx context.Context, queryVector []float32, limit int, threshold float32) ([]ChunkSearchResult, error)
	GetDimensions() int
	GetTableNames() (documents, chunks string)
}

var _ VectorDB = (*PostgresVectorDB)(nil)

// PostgresVectorDB 实现了 VectorDB 接口，使用 PostgreSQL 和 pgvector。
type PostgresVectorDB struct {
	pool           *pgxpool.Pool
	dimensions     int
	documentsTable string
	chunksTable    string
}

// NewPostgresVectorDB 创建并返回一个新的 PostgresVectorDB 实例。
// dimensions: 向量维度，用于生成表名和向量字段
func NewPostgresVectorDB(dsn string, dimensions int) (*PostgresVectorDB, error) {
	ctx := context.Background()

	// 1. 配置连接池
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("解析数据库连接字符串失败: %w", err)
	}

	// 设置连接池参数
	config.MaxConns = 25 // 最大连接数
	config.MinConns = 5  // 最小连接数
	config.MaxConnLifetime = 1 * time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	// 2. 创建连接池
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("无法创建数据库连接池: %w", err)
	}

	// 3. 检查连接池是否成功
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("数据库 ping 失败: %w", err)
	}

	logger.Get().Info("成功创建 PostgreSQL 连接池",
		zap.Int32("max_conns", config.MaxConns),
		zap.Int32("min_conns", config.MinConns),
	)

	// 4. 启用 pgvector 扩展
	_, err = pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector;")
	if err != nil {
		return nil, fmt.Errorf("无法启用 vector 扩展: %w", err)
	}
	logger.Get().Info("pgvector 扩展已启用")

	// 5. 启用中文分词扩展 zhparser
	_, err = pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS zhparser;")
	if err != nil {
		return nil, fmt.Errorf("无法启用 zhparser 扩展: %w", err)
	}
	logger.Get().Info("zhparser 扩展已启用")

	// 6. 创建并配置中文分词
	// 使用 DO block 来确保仅在配置不存在时才创建，避免并发问题或重复执行错误
	createTsConfig := `
	DO $$
	BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_ts_config WHERE cfgname = 'chinese_zh') THEN
			CREATE TEXT SEARCH CONFIGURATION chinese_zh (PARSER = zhparser);
			ALTER TEXT SEARCH CONFIGURATION chinese_zh
				ADD MAPPING FOR n,v,a,i,e,l WITH simple;
		END IF;
	END$$;`
	_, err = pool.Exec(ctx, createTsConfig)
	if err != nil {
		return nil, fmt.Errorf("无法创建中文分词配置: %w", err)
	}
	logger.Get().Info("中文分词配置 'chinese_zh' 已准备就绪")

	// 7. 根据维度生成表名
	documentsTable := fmt.Sprintf("document_%dd", dimensions)
	chunksTable := fmt.Sprintf("document_chunk_%dd", dimensions)

	// 8. 创建文档表和文档块表
	createDocumentsTable := fmt.Sprintf(createDocumentsTableTemplate, documentsTable)
	createChunksTable := fmt.Sprintf(createChunksTableTemplate, chunksTable, documentsTable, dimensions)

	// 执行创建表的操作
	_, err = pool.Exec(ctx, createDocumentsTable)
	if err != nil {
		return nil, fmt.Errorf("无法创建 rag_documents 表: %w", err)
	}

	_, err = pool.Exec(ctx, createChunksTable)
	if err != nil {
		return nil, fmt.Errorf("无法创建 document_chunks 表: %w", err)
	}
	logger.Get().Info(fmt.Sprintf("表 %s 和 %s 已准备就绪", documentsTable, chunksTable))

	// 9. 为 title 和 content 字段创建中文分词 GIN 索引
	createDocumentsTitleIndex := fmt.Sprintf(createDocumentsTitleIndexTemplate, dimensions, documentsTable)
	createChunksContentIndex := fmt.Sprintf(createChunksContentIndexTemplate, dimensions, chunksTable)

	_, err = pool.Exec(ctx, createDocumentsTitleIndex)
	if err != nil {
		return nil, fmt.Errorf("无法为 documents 表的 title 创建 GIN 索引: %w", err)
	}

	_, err = pool.Exec(ctx, createChunksContentIndex)
	if err != nil {
		return nil, fmt.Errorf("无法为 chunks 表的 content 创建 GIN 索引: %w", err)
	}
	logger.Get().Info(fmt.Sprintf("为表 %s 和 %s 的文本内容创建了中文分词 GIN 索引", documentsTable, chunksTable))

	return &PostgresVectorDB{
		pool:           pool,
		dimensions:     dimensions,
		documentsTable: documentsTable,
		chunksTable:    chunksTable,
	}, nil
}

// StoreDocument 存储文档并返回文档ID
func (db *PostgresVectorDB) StoreDocument(ctx context.Context, title, minioKey string, metadata map[string]interface{}) (string, error) {
	docID := uuid.New().String()

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("序列化 metadata 失败: %w", err)
	}

	_, err = db.pool.Exec(ctx,
		fmt.Sprintf(insertDocumentTemplate, db.documentsTable),
		docID, title, minioKey, metadataJSON)
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

	_, err = db.pool.Exec(ctx,
		fmt.Sprintf(insertChunkTemplate, db.chunksTable),
		docID, chunkIndex, content, pgvector.NewVector(embedding), metadataJSON)
	if err != nil {
		return fmt.Errorf("存储文档块失败: %w", err)
	}

	return nil
}

// SearchSimilarChunks 基于向量相似性搜索相关文档块
func (db *PostgresVectorDB) SearchSimilarChunks(ctx context.Context, queryVector []float32, limit int, threshold float32) ([]ChunkSearchResult, error) {
	// 使用余弦相似度搜索相似的文档块
	query := fmt.Sprintf(searchChunksTemplate, db.chunksTable)

	rows, err := db.pool.Query(ctx, query, pgvector.NewVector(queryVector), threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("查询相似文档块失败: %w", err)
	}
	defer rows.Close()

	var results []ChunkSearchResult
	for rows.Next() {
		var result ChunkSearchResult
		var metadataJSON []byte

		err := rows.Scan(
			&result.ChunkID,
			&result.DocumentID,
			&result.Content,
			&result.Similarity,
			&metadataJSON,
		)
		if err != nil {
			logger.Get().Error("扫描搜索结果失败", zap.Error(err))
			continue
		}

		// 解析metadata
		if len(metadataJSON) > 0 {
			err = json.Unmarshal(metadataJSON, &result.Metadata)
			if err != nil {
				logger.Get().Error("解析metadata失败", zap.Error(err))
				result.Metadata = make(map[string]interface{})
			}
		} else {
			result.Metadata = make(map[string]interface{})
		}

		results = append(results, result)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历搜索结果失败: %w", err)
	}

	logger.Get().Info(fmt.Sprintf("向量搜索完成，找到 %d 个相似块", len(results)))
	return results, nil
}

// GetDimensions 返回向量维度
func (db *PostgresVectorDB) GetDimensions() int {
	return db.dimensions
}

// GetTableNames 返回文档表和分块表的名称
func (db *PostgresVectorDB) GetTableNames() (documents, chunks string) {
	return db.documentsTable, db.chunksTable
}
