# Justfile

# 默认命令，显示可用的任务列表
default:
    @just --list

gen:
    @echo "正在通过 Docker 运行 buf 生成 Protobuf 和 ConnectRPC 代码..."
    docker run --rm -v "$(pwd):/workspace" -w /workspace bufbuild/buf generate --path api/rag/v1
    @echo "修复 TypeScript 导入扩展名..."
    sed -i '' 's/\.js"/"/g' web/gen/rag/v1/*.ts
    @echo "代码生成完毕。"

clean:
    @echo "正在清理生成的代码..."
    rm -rf internal/gen
    @echo "清理完成。"

deps:
    @echo "正在安装 Go 依赖..."
    go mod tidy
    @echo "依赖安装完成。"

web-install:
    @echo "在 web 目录安装前端依赖..."
    cd web && bun install
    @echo "前端依赖安装完成。"

web-dev:
    @echo "启动前端开发服务器 (Next.js)..."
    cd web && bun run dev

web-build:
    @echo "构建前端生产包..."
    cd web && bun run build

web-start:
    @echo "启动前端生产服务器..."
    cd web && bun run start

web-lint:
    @echo "运行前端 Lint..."
    cd web && bun run lint
