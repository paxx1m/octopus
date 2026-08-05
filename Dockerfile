# syntax=docker/dockerfile:1

# ---------- 前端构建 ----------
FROM --platform=$BUILDPLATFORM node:22-bookworm AS frontend-builder

# 与后端 ldflags 使用同一 BUILD_VERSION，避免前后端版本不一致误报
ARG BUILD_VERSION=docker
ENV VITE_APP_VERSION=$BUILD_VERSION

WORKDIR /frontend
COPY web/package.json web/pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    corepack enable && pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm run build
# vite 配置 outDir 为 ../static/out，即 /static/out

# ---------- Go 二进制构建 ----------
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS go-builder

ARG TARGETARCH
ARG BUILD_VERSION=docker

ENV GOPROXY=https://proxy.golang.org,direct
ENV CGO_ENABLED=0

WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
COPY --from=frontend-builder /static/out ./static/out
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=linux GOARCH=${TARGETARCH} \
    go build -tags=jsoniter -o /octopus -ldflags="-s -w \
        -X 'github.com/bestruirui/octopus/internal/conf.Version=${BUILD_VERSION}' \
        -X 'github.com/bestruirui/octopus/internal/conf.BuildTime=$(date -u "+%F %T %z")' \
        -X 'github.com/bestruirui/octopus/internal/conf.Author=bestrui' \
        -X 'github.com/bestruirui/octopus/internal/conf.Commit=unknown'" .

# ---------- 运行镜像 ----------
FROM debian:bookworm-slim

ENV TZ=Asia/Shanghai
ENV PUID=0
ENV PGID=0
# 容器内禁止二进制自更新（应重建镜像），并避免与上游 release 误比
ENV OCTOPUS_DISABLE_SELF_UPDATE=1

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        tzdata \
        gosu \
 && ln -fs /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
 && dpkg-reconfigure -f noninteractive tzdata \
 && rm -rf /var/lib/apt/lists/* \
 && mkdir -p /app

COPY --from=go-builder /octopus /app/octopus
COPY scripts/dockerfiles/entrypoint.sh /entrypoint.sh

RUN chmod +x /app/octopus /entrypoint.sh

WORKDIR /app
EXPOSE 8080
ENTRYPOINT ["/entrypoint.sh"]
