#!/bin/bash

# Check if reset binary exists, if not build it
if [ ! -f "./reset" ]; then
    echo "⚠️  Reset tool not found. Building..."
    (cd server_go && go build -o ../reset ./cmd/reset)
    if [ $? -ne 0 ]; then
        echo "❌ Failed to build reset tool."
        exit 1
    fi
    echo "✅ Reset tool built."
fi

# Check for --prod flag
if [[ "$1" == "--prod" ]]; then
    echo "🚀 Updating Production (App & Proxy)..."
    (cd frontend && npm run build) && \
    docker compose --env-file .env.prod -f docker-compose.prod.yml up -d --build go-api-prod python-runner-prod && \
    docker compose --env-file .env.prod -f docker-compose.proxy.prod.yml up -d && \
    docker compose --env-file .env.prod -f docker-compose.proxy.prod.yml restart nginx
    exit 0
fi

# Run reset with args
./reset "$@" && docker compose -f docker-compose.proxy.yml down && docker compose -f docker-compose.proxy.yml up -d