ARG GO_IMAGE=golang:1.25.6-alpine
ARG RUNTIME_IMAGE=debian:bookworm-slim
FROM ${GO_IMAGE} AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

WORKDIR /src/platform
COPY platform/go.mod platform/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY platform/ ./

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    flags="-s -w -X github.com/yzxingtu2026/ainp/platform/framework/buildinfo.version=${VERSION} -X github.com/yzxingtu2026/ainp/platform/framework/buildinfo.commit=${COMMIT} -X github.com/yzxingtu2026/ainp/platform/framework/buildinfo.buildTime=${BUILD_TIME}" && \
    CGO_ENABLED=0 go build -trimpath -ldflags "$flags" -o /out/ainp ./cmd/ainp && \
    CGO_ENABLED=0 go build -trimpath -ldflags "$flags" -o /out/ainp-worker ./cmd/ainp-worker && \
    CGO_ENABLED=0 go build -trimpath -ldflags "$flags" -o /out/ainp-migrate ./cmd/ainp-migrate

FROM ${RUNTIME_IMAGE} AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl tzdata && \
    rm -rf /var/lib/apt/lists/* && \
    useradd --system --uid 10001 --home-dir /app --shell /usr/sbin/nologin ainp

WORKDIR /app
COPY --from=builder --chown=ainp:ainp /out/ainp /out/ainp-worker /out/ainp-migrate /usr/local/bin/
COPY --chown=ainp:ainp platform/i18n ./i18n
COPY --chown=ainp:ainp platform/prompts ./prompts

USER ainp
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ainp"]
CMD ["-config", "/app/config/config.yaml"]
