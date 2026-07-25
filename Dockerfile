# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder

WORKDIR /build

RUN apk add --no-cache build-base

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=1 go build \
    -trimpath \
    -ldflags="-s -w \
      -X github.com/futuretea/postgres-mcp-server/pkg/core/version.Version=${VERSION} \
      -X github.com/futuretea/postgres-mcp-server/pkg/core/version.Commit=${GIT_COMMIT} \
      -X github.com/futuretea/postgres-mcp-server/pkg/core/version.Date=${BUILD_DATE}" \
    -o /usr/local/bin/postgres-mcp-server ./cmd/postgres-mcp-server

FROM builder AS parser-compile-check

RUN CGO_ENABLED=1 go test ./internal/postgres

FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates \
    && addgroup -S mcp \
    && adduser -S -G mcp mcp

USER mcp

ENTRYPOINT ["/usr/local/bin/postgres-mcp-server", "mcp"]

FROM runtime AS release

COPY postgres-mcp-server /usr/local/bin/postgres-mcp-server

FROM runtime AS dev

COPY --from=builder /usr/local/bin/postgres-mcp-server /usr/local/bin/postgres-mcp-server
