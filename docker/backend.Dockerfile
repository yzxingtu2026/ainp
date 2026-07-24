ARG GO_IMAGE=golang:1.25.6-alpine
ARG RUNTIME_IMAGE=alpine:3.22
FROM ${GO_IMAGE} AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN apk add --no-cache build-base

WORKDIR /src/platform
COPY platform/go.mod platform/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY platform/ ./

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    flags="-s -w -X github.com/yzxingtu2026/ainp/platform/framework/buildinfo.version=${VERSION} -X github.com/yzxingtu2026/ainp/platform/framework/buildinfo.commit=${COMMIT} -X github.com/yzxingtu2026/ainp/platform/framework/buildinfo.buildTime=${BUILD_TIME}" && \
    CGO_ENABLED=1 go build -tags musl -trimpath -ldflags "$flags" -o /out/ainp ./cmd/ainp && \
    CGO_ENABLED=1 go build -tags musl -trimpath -ldflags "$flags" -o /out/ainp-worker ./cmd/ainp-worker && \
    CGO_ENABLED=1 go build -tags musl -trimpath -ldflags "$flags" -o /out/ainp-migrate ./cmd/ainp-migrate

FROM ${RUNTIME_IMAGE} AS runtime

RUN apk add --no-cache ca-certificates curl tzdata && \
    addgroup --system --gid 10001 ainp && \
    adduser --system --disabled-password --no-create-home --uid 10001 --ingroup ainp ainp

WORKDIR /app
COPY --from=builder --chown=ainp:ainp /out/ainp /out/ainp-worker /out/ainp-migrate /usr/local/bin/
COPY --chown=ainp:ainp platform/i18n ./i18n
COPY --chown=ainp:ainp platform/prompts ./prompts

USER ainp
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ainp"]
CMD ["-config", "/app/config/config.yaml"]
