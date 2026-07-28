FROM node:22-alpine AS web-build

ARG PNPM_VERSION=10.14.0
WORKDIR /src
RUN corepack enable && corepack prepare pnpm@${PNPM_VERSION} --activate
COPY web/package.json web/pnpm-lock.yaml ./web/
WORKDIR /src/web
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM golang:1.26-alpine AS go-build

ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-build /src/internal/webui/dist ./internal/webui/dist
RUN go test ./...
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/tinyprobe-server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/downloads/tinyprobe-agent-linux-amd64 ./cmd/agent
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
    go build -trimpath -ldflags="-s -w" -o /out/downloads/tinyprobe-agent-linux-arm64 ./cmd/agent

FROM alpine:3.22 AS runtime

RUN addgroup -S tinyprobe \
    && adduser -S -D -H -G tinyprobe tinyprobe \
    && mkdir -p /app/downloads /data \
    && chown -R tinyprobe:tinyprobe /app /data
WORKDIR /app
COPY --from=go-build --chown=tinyprobe:tinyprobe /out/tinyprobe-server ./tinyprobe-server
COPY --from=go-build --chown=tinyprobe:tinyprobe /out/downloads/ ./downloads/
USER tinyprobe
ENV TINYPROBE_ADDR=:8080 \
    TINYPROBE_DB_PATH=/data/tinyprobe.db \
    TINYPROBE_SECURE_COOKIES=false
EXPOSE 8080
ENTRYPOINT ["/app/tinyprobe-server"]
