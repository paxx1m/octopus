# syntax=docker/dockerfile:1

# ---------- 前端构建 ----------
FROM --platform=$BUILDPLATFORM node:22-bookworm AS frontend-builder

ARG BUILD_VERSION=docker

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
