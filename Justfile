# Justfile

# 默认命令，显示可用的任务列表
default:
    @just --list

gen:
    @echo "正在通过 Docker 运行 buf 生成 Protobuf 和 ConnectRPC 代码..."
    docker run --rm -v "$(pwd):/workspace" -w /workspace bufbuild/buf generate --path api/proto/rag/v1
    @echo "代码生成完毕。"

clean:
    @echo "正在清理生成的代码..."
    rm -rf internal/gen
    @echo "清理完成。"

deps:
    @echo "正在安装 Go 依赖..."
    go mod tidy
    @echo "依赖安装完成。"

