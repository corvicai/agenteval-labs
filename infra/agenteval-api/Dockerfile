# syntax=docker/dockerfile:1.7
# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Build-time revision info (injected via --build-arg in CI)
ARG BUILD_COMMIT=""
ARG BUILD_BRANCH=""
ARG BUILD_DIRTY=""
ARG BUILD_UPDATED_AT=""

# Build dependencies (CGO disabled below; kept for tooling)
RUN apk add --no-cache gcc musl-dev

# Copy module manifests first so dependency download is cached.
COPY server_go/go.mod server_go/go.sum ./

# Download deps (read-only). Previously used `go mod tidy` which mutates
# go.mod and re-resolves the world. `download` respects go.sum.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Copy source AFTER dep resolution.
COPY server_go/. .

# Build the application — bake revision info into the binary.
# Dropped `-a -installsuffix cgo` (defeated the cache).
# Added `-trimpath` and `-s -w` (smaller binary).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath \
    -ldflags="-s -w \
              -X benchmarking-platform/internal/buildinfo.Commit=${BUILD_COMMIT} \
              -X benchmarking-platform/internal/buildinfo.Branch=${BUILD_BRANCH} \
              -X benchmarking-platform/internal/buildinfo.Dirty=${BUILD_DIRTY} \
              -X benchmarking-platform/internal/buildinfo.UpdatedAt=${BUILD_UPDATED_AT}" \
    -o main .

# ──────────────────────────────────────────────────────────────────────
# Final stage
# ──────────────────────────────────────────────────────────────────────
FROM alpine:3.19

WORKDIR /app

RUN apk --no-cache add ca-certificates

# Copy binary from builder
COPY --from=builder /app/main .

# Copy migrations
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

CMD ["./main"]
