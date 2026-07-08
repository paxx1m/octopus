# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:22-bookworm AS frontend-builder

ARG BUILD_VERSION=docker

WORKDIR /frontend
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    corepack enable && pnpm install --frozen-lockfile
COPY web/ ./
RUN NEXT_PUBLIC_APP_VERSION=${BUILD_VERSION} pnpm run build

FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS go-builder

ARG TARGETARCH
ARG BUILD_VERSION=docker

ENV GOPROXY=https://proxy.golang.org,direct
ENV CGO_ENABLED=0

RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY go.mod go.sum ./
RUN git clone --depth 1 https://github.com/looplj/axonhub.git /tmp/axonhub \
 && mkdir -p /axonhub \
 && cp -R /tmp/axonhub/llm /axonhub/llm
RUN mkdir -p /app /app/../axonhub && ln -s /axonhub/llm /app/../axonhub/llm
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN rm -rf static/out && mkdir -p static/out
COPY --from=frontend-builder /frontend/out ./static/out
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=linux GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w -X github.com/bestruirui/octopus/internal/conf.Version=${BUILD_VERSION}" -o /octopus .

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
