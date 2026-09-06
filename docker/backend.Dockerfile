ARG GO_IMAGE=golang:1.25.6-alpine
ARG RUNTIME_IMAGE=alpine:3.22
FROM ${GO_IMAGE} AS mupdf

ARG MUPDF_VERSION=1.28.2
ARG MUPDF_SHA256=44075a84e329db55b9bef5f342a70fd26d69e48ad1d33cb89d9664581c641156

RUN apk add --no-cache build-base curl
COPY docker/build-mupdf-cjk.sh /tmp/build-mupdf-cjk.sh
RUN chmod +x /tmp/build-mupdf-cjk.sh && \
    MUPDF_VERSION="${MUPDF_VERSION}" MUPDF_SHA256="${MUPDF_SHA256}" /tmp/build-mupdf-cjk.sh /usr/local

FROM ${GO_IMAGE} AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN apk add --no-cache build-base
COPY --from=mupdf /usr/local /usr/local

WORKDIR /src/platform
COPY platform/go.mod platform/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY platform/ ./

# extlib + static links the pinned MuPDF. Do not use go-fitz bundled libs (no CJK).
ENV CGO_ENABLED=1
ENV CGO_CFLAGS="-I/usr/local/include"
ENV CGO_LDFLAGS="-L/usr/local/lib -lmupdf -lmupdf-third -lm"

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    AINP_REQUIRE_PDF_CJK=1 go test -tags "extlib,static" ./tests -run 'TestKnowledgeSafeParserHandlesUniGBReceipt|TestKnowledgeSafeParserDecodesPDFToUnicodeMapping|TestKnowledgeSafeParserExtractsEnglishPDF' -count=1

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    flags="-s -w -X github.com/yzxingtu2026/ainp/platform/framework/buildinfo.version=${VERSION} -X github.com/yzxingtu2026/ainp/platform/framework/buildinfo.commit=${COMMIT} -X github.com/yzxingtu2026/ainp/platform/framework/buildinfo.buildTime=${BUILD_TIME}" && \
    go build -tags "extlib,static" -trimpath -ldflags "$flags" -o /out/ainp ./cmd/ainp && \
    go build -tags "extlib,static" -trimpath -ldflags "$flags" -o /out/ainp-worker ./cmd/ainp-worker && \
    go build -tags "extlib,static" -trimpath -ldflags "$flags" -o /out/ainp-migrate ./cmd/ainp-migrate

FROM ${RUNTIME_IMAGE} AS runtime

RUN apk add --no-cache ca-certificates curl tzdata && \
    addgroup --system --gid 10001 ainp && \
    adduser --system --disabled-password --no-create-home --uid 10001 --ingroup ainp ainp

WORKDIR /app
COPY --from=builder --chown=ainp:ainp /out/ainp /out/ainp-worker /out/ainp-migrate /usr/local/bin/

USER ainp
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ainp"]
CMD ["-config", "/app/config/config.yaml"]
