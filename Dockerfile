# ============================================================
# NanoGrip Docker 镜像
# 基于多阶段构建：
#   1. 构建阶段：编译 Go 程序
#   2. 运行阶段：轻量级运行镜像
# ============================================================

# ----------------------------------------------------------------
# 阶段 1: 构建阶段
# ----------------------------------------------------------------
# 使用官方的 Go 构建镜像
FROM --platform=linux/amd64 golang:1.21-alpine AS builder

# 安装构建所需的依赖
RUN apk add --no-cache git ca-certificates tzdata

# 设置工作目录
WORKDIR /build

# 复制 Go 模块文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建 NanoGrip 可执行文件
# -buildvcs=false: 不嵌入版本信息（适用于没有 git 的环境）
# -ldflags: 设置运行时参数
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -buildvcs=false \
    -ldflags="-s -w" \
    -o NanoGrip \
    ./cmd/NanoGrip

# ----------------------------------------------------------------
# 阶段 2: 运行阶段
# ----------------------------------------------------------------
# 使用轻量级的 Alpine Linux
FROM --platform=linux/amd64 alpine:3.19

# 安装运行时所需的依赖
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    bash \
    curl \
    git \
    openssh-client

# 创建非 root 用户（提高安全性）
RUN adduser -D -u 1000 NanoGrip

# 设置工作目录
WORKDIR /home/NanoGrip

# 从构建阶段复制可执行文件
COPY --from=builder /build/NanoGrip .

# 创建必要的目录
RUN mkdir -p /home/NanoGrip/.NanoGrip/workspace \
    /home/NanoGrip/.NanoGrip/skills \
    && chown -R NanoGrip:NanoGrip /home/NanoGrip

# 切换到非 root 用户
USER NanoGrip

# 设置环境变量
ENV HOME=/home/NanoGrip \
    PATH=/home/NanoGrip:$PATH \
    NANOGRIP_CONFIG=/home/NanoGrip/.NanoGrip/config.yaml

# 暴露端口
# 18790: Gateway 端口
EXPOSE 18790

# 默认启动命令
CMD ["sh"]

# ----------------------------------------------------------------
# 标签和元数据（供构建时使用）
# ----------------------------------------------------------------
# 构建示例:
#   docker build -t NanoGrip:latest .
#
# 运行示例:
#   docker run -v ~/.NanoGrip:/home/NanoGrip/.NanoGrip -p 18790:18790 NanoGrip:latest
#
# 交互式运行:
#   docker run -it -v ~/.NanoGrip:/home/NanoGrip/.NanoGrip -p 18790:18790 --entrypoint /bin/sh NanoGrip:latest
