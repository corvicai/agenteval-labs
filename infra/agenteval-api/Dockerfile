# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Build-time revision info (injected via --build-arg in CI)
ARG BUILD_COMMIT=""
ARG BUILD_BRANCH=""
ARG BUILD_DIRTY=""
ARG BUILD_UPDATED_AT=""

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

# Copy go mod files first
COPY server_go/go.mod ./

# Download dependencies
RUN go mod tidy

# Copy source code AFTER dependencies are cached
COPY server_go/. .

# Build the application — bake revision info into the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags="-X benchmarking-platform/internal/buildinfo.Commit=${BUILD_COMMIT} \
              -X benchmarking-platform/internal/buildinfo.Branch=${BUILD_BRANCH} \
              -X benchmarking-platform/internal/buildinfo.Dirty=${BUILD_DIRTY} \
              -X benchmarking-platform/internal/buildinfo.UpdatedAt=${BUILD_UPDATED_AT}" \
    -o main .

# Final stage
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Copy binary from builder
COPY --from=builder /app/main .

# Copy migrations
COPY --from=builder /app/migrations ./migrations

# Expose port
EXPOSE 8080

# Run the application
CMD ["./main"]
