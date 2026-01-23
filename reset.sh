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

# Run reset with args
./reset -soft-reset "$@" && docker compose -f docker-compose.proxy.yml down && docker compose -f docker-compose.proxy.yml up -d