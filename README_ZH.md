# RAG 系统

基于 Go 构建的文档检索增强生成系统。

## 功能特性

- **文档处理**: 使用 Doc2X 进行 PDF 解析和文本提取
- **向量存储**: PostgreSQL + pgvector 实现语义相似性搜索
- **嵌入生成**: 支持多种嵌入模型
- **大模型集成**: OpenAI 兼容 API 进行文本生成
- **缓存优化**: Redis 缓存提升性能
- **文件存储**: MinIO 对象存储
- **智能重排**: 文档相关性评分

## 系统架构

系统采用模块化架构，职责分离清晰：

```
cmd/server/          # 应用程序入口
internal/
├── adapters/        # 数据库适配器 (PostgreSQL)
├── clients/         # 外部服务客户端
│   ├── base/        # 共享 HTTP 客户端工具
│   ├── doc2x/       # 文档解析服务
│   ├── embedding/   # 嵌入生成
│   ├── openai/      # 大模型集成
│   └── rerank/      # 文档重排
├── config/          # 配置管理
├── logger/          # 结构化日志
├── redis/           # 缓存层
├── server/          # HTTP 服务器和处理器
└── storage/         # 文件存储 (MinIO)
```

## 环境要求

- Go 1.25+
- PostgreSQL（带 pgvector 扩展）
- Redis
- MinIO（或 S3 兼容存储）

## 安装部署

1. 克隆仓库：
```bash
git clone https://github.com/hsn0918/rag.git
cd rag
```

2. 安装依赖：
```bash
go mod download
```

3. 配置系统（详见配置章节）

4. 启动服务：
```bash
go run cmd/server/main.go
```

## 配置说明

通过环境变量或配置文件进行系统配置，主要包括：

- 数据库连接（PostgreSQL, Redis）
- 外部服务端点和 API 密钥
- 存储配置（MinIO）
- 服务器端口和设置

## API 接口

系统提供 gRPC/Connect API，包含以下接口：

### 1. 预上传文件
生成用于直接文件上传到存储的预签名 URL。

**接口**: `POST /rag.v1.RagService/PreUpload`

**请求参数**:
```json
{
  "filename": "document.pdf"
}
```

**响应结果**:
```json
{
  "upload_url": "https://minio.example.com/...",
  "file_key": "unique-file-key",
  "expires_in": 900
}
```

**参数说明**:
- `filename` (必需): 文档的原始文件名

### 2. 上传文档
处理并索引已上传到存储的 PDF 文档。

**接口**: `POST /rag.v1.RagService/UploadPdf`

**请求参数**:
```json
{
  "file_key": "unique-file-key",
  "filename": "document.pdf"
}
```

**响应结果**:
```json
{
  "success": true,
  "message": "PDF 处理成功。文档 ID: doc123，分块数: 25",
  "document_id": "doc123"
}
```

**参数说明**:
- `file_key` (必需): 预上传返回的文件密钥
- `filename` (必需): 原始文件名

**处理流程**:
1. 使用文件密钥从存储下载文件
2. 使用 Doc2X 服务提取文本内容
3. 将内容分割成语义块
4. 为每个块生成嵌入向量
5. 将文档和块存储到 PostgreSQL + pgvector

### 3. 查询上下文
基于自然语言查询检索相关信息。

**接口**: `POST /rag.v1.RagService/GetContext`

**请求参数**:
```json
{
  "query": "如何实现机器学习算法？"
}
```

**响应结果**:
```json
{
  "context": "基于您关于实现机器学习算法的查询...\n\n## 相关信息\n\n1. 算法选择...\n2. 实现步骤..."
}
```

**参数说明**:
- `query` (必需): 中文或英文自然语言查询

**处理流程**:
1. 使用大模型从查询中提取关键词
2. 生成嵌入向量进行语义搜索
3. 使用 pgvector 搜索相似文档块
4. 使用混合评分（相似度 + 关键词匹配）重新排序
5. 使用大模型从检索的块生成智能摘要

## 配置参数

### 分块配置
- `max_chunk_size`: 文本块最大大小（默认: 1000）
- `overlap_size`: 块之间重叠大小（默认: 100）
- `adaptive_size`: 启用自适应块大小
- `paragraph_boundary`: 尊重段落边界
- `sentence_boundary`: 尊重句子边界

### 嵌入配置
- 支持模型: BGE-Large、BGE-M3、Qwen3-Embedding 系列
- 维度: 特定于模型（768-4096）
- Token 限制: 根据模型为 512-32768

### 搜索配置
- `similarity_threshold`: 最小相似度分数（0.25-0.4）
- `max_results`: 最大结果数量（5-15）
- `rerank_enabled`: 启用智能重排

## 响应格式

### 错误响应
所有接口返回标准 Connect/gRPC 错误：

```json
{
  "code": "INVALID_ARGUMENT",
  "message": "filename is required"
}
```

### 成功指示
- 上传操作返回 `success: true` 和处理详情
- 查询操作返回结构化上下文和相似度分数
- 所有响应都包含相关元数据和使用提示

## 技术依赖

- **数据库**: `github.com/jackc/pgx/v5` 用于 PostgreSQL
- **向量搜索**: `github.com/pgvector/pgvector-go` 用于嵌入向量
- **缓存**: `github.com/redis/rueidis` 用于 Redis
- **存储**: `github.com/minio/minio-go/v7` 用于对象存储
- **HTTP 客户端**: `github.com/go-resty/resty/v2` 用于外部 API
- **依赖注入**: `go.uber.org/fx` 用于应用结构
- **日志**: `go.uber.org/zap` 用于结构化日志

## 使用示例

### 完整使用流程

1. **获取上传 URL**:
```bash
curl -X POST http://localhost:8080/rag.v1.RagService/PreUpload \
  -H "Content-Type: application/json" \
  -d '{"filename": "技术文档.pdf"}'
```

2. **上传文件到预签名 URL**:
```bash
curl -X PUT "预签名URL" \
  -T "技术文档.pdf"
```

3. **处理文档**:
```bash
curl -X POST http://localhost:8080/rag.v1.RagService/UploadPdf \
  -H "Content-Type: application/json" \
  -d '{"file_key": "返回的文件密钥", "filename": "技术文档.pdf"}'
```

4. **查询信息**:
```bash
curl -X POST http://localhost:8080/rag.v1.RagService/GetContext \
  -H "Content-Type: application/json" \
  -d '{"query": "这个系统如何处理中文文档？"}'
```

## 贡献指南

1. Fork 仓库
2. 创建功能分支
3. 提交更改
4. 添加测试（如适用）
5. 提交 Pull Request

## 开源协议

本项目按照仓库中指定的条款进行许可。