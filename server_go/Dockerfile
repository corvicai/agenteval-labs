# syntax=docker/dockerfile:1.7
# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Build dependencies (CGO disabled below, but musl-dev kept for tooling)
RUN apk add --no-cache gcc musl-dev

# Copy module manifests first so dependency download is cached separately
# from source changes.
COPY go.mod go.sum ./

# Download deps (read-only). Previously used `go mod tidy` which mutates
# the manifest at build time and re-resolves the world. `download` is
# faster, deterministic, and respects go.sum.
# BuildKit cache mounts persist the module + build cache across builds —
# without these, every build re-downloads and re-compiles everything.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Copy source AFTER dep resolution so editing source doesn't bust the
# (cached) `go mod download` layer.
COPY . .

# Build. Dropped `-a -installsuffix cgo` (force-rebuild everything,
# defeats the cache). Added `-trimpath` for reproducibility and
# `-ldflags="-s -w"` to strip debug info → ~30-40% smaller binary.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o main .

# ──────────────────────────────────────────────────────────────────────
# Final stage
# ──────────────────────────────────────────────────────────────────────
FROM alpine:3.23

WORKDIR /app

RUN apk --no-cache add ca-certificates

# Copy binary from builder
COPY --from=builder /app/main .

# Copy migrations
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

CMD ["./main"]
