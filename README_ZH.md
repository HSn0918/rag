# RAG 系统

基于 Go 的文档检索增强生成系统，提供 Connect/gRPC 接口和 Next.js（Bun）前端示例。

## 功能特性

- **文档管线**：预上传 URL → PDF 文本提取（Doc2X）→ 语义分块 → 向量化 → pgvector 存储
- **检索/重排**：向量搜索 + 关键词加权重排，最终由 LLM 生成总结
- **客户端**：提供 Go/TS 代码生成，Next.js 演示 UI（虚拟滚动列表、删除文档、关键词 D3 可视化）
- **基础设施**：PostgreSQL + pgvector、Redis 缓存、MinIO 对象存储
- **工具链**：`just gen` 进行 proto 代码生成（Docker），`just web-*` 前端快捷命令（Bun）

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

- Go 1.25+（建议）
- PostgreSQL + pgvector
- Redis
- MinIO（或 S3 兼容）
- Docker（用于 `just gen` 代码生成）与 Bun（运行 Next.js 前端）

## 快速开始

1) 拉取代码并安装 Go 依赖
```bash
git clone https://github.com/hsn0918/rag.git
cd rag
go mod download
```

2) 配置：复制 `config.example.yaml` 为 `config.yaml`，填入数据库/Redis/MinIO/API Key。

3) 代码生成（需 Docker）：
```bash
just gen
```

4) 启动后端：
```bash
go run cmd/server/main.go
```

5) 前端（Bun + Next.js）：
```bash
cd web
bun install
bun run dev
```

## 配置说明

使用 `config.yaml`（示例见 `config.example.yaml`）或环境变量：

- `server`：监听地址/端口
- `database`：PostgreSQL + pgvector
- `redis`：主机/端口/认证
- `minio`：endpoint/AK/SK/bucket
- `services`：Doc2X、Embedding、Reranker、LLM 的 endpoint、模型和 API Key
- `chunking`：分块大小、重叠、语义分块等

## API（Connect/gRPC）

服务：`rag.v1.RagService`

- `POST /rag.v1.RagService/PreUpload` — 获取预签名上传 URL
- `POST /rag.v1.RagService/UploadPdf` — 处理并入库 PDF
- `POST /rag.v1.RagService/GetContext` — 完整 RAG（提词 → 向量 → 检索 → 重排 → 总结）
- `POST /rag.v1.RagService/ListDocuments` — 游标分页列出文档
- `POST /rag.v1.RagService/DeleteDocument` — 删除文档及其分块

消息定义见 `api/rag/v1/rag.proto`，生成代码位于 `internal/gen`（Go）和 `web/gen`（TS）。

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

## CLI/脚本

- `just gen` — 通过 Docker 运行 buf 生成 Go/TS 代码
- `just web-install|web-dev|web-build|web-start|web-lint` — 前端快捷命令（Bun）
- `go run cmd/server/main.go` — 启动后端

## 前端说明

- Next.js（Bun）+ Connect TS 客户端，通过 `web/next.config.mjs` 将 `/api` 代理到后端。
- 功能：上传流程、RAG 查询、文档列表（游标分页 + 删除）、关键词 D3 可视化、结构化答案展示。

## 注意事项

- 运行 `just gen` 需要 Docker 权限（bufbuild/buf 镜像）。
- 嵌入/重排/LLM 服务的 API Key 和模型请在 `config.yaml` 中更新。
- 确保 Postgres 已安装 pgvector 扩展，配置的 DSN 与实际一致。
- **向量搜索**: `github.com/pgvector/pgvector-go` 用于嵌入向量
- **缓存**: `github.com/redis/rueidis` 用于 Redis
- **存储**: `github.com/minio/minio-go/v7` 用于对象存储
- **HTTP 客户端**: `github.com/go-resty/resty/v2` 用于外部 API
- **依赖注入**: `go.uber.org/fx` 用于应用结构
- **日志**: 标准库 `log/slog` 用于结构化日志

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
