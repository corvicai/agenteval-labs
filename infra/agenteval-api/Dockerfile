# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache gcc musl-dev


# Copy go mod files first
COPY server_go/go.mod ./

# Download dependencies
RUN go mod tidy

# Copy source code AFTER dependencies are cached
COPY server_go/. .


# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Final stage
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Copy binary from builder
COPY --from=builder /app/main .

# Copy migrations
COPY --from=builder /app/migrations ./migrations

# Copy Firebase service account if it exists
COPY --from=builder /app/firebase-service-account.json* .

# Expose port
EXPOSE 8080

# Run the application
CMD ["./main"]

