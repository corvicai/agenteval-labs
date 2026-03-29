#!/bin/bash

set -euo pipefail

run_compose_with_build_fallback() {
  local log_file
  log_file="$(mktemp)"

  if docker compose --profile dev up -d --build 2>&1 | tee "$log_file"; then
    rm -f "$log_file"
    return 0
  fi

  if grep -Eq "failed to prepare extraction snapshot|parent snapshot .* does not exist" "$log_file"; then
    echo "⚠ BuildKit export failed after image build. Retrying startup without rebuild..."
    if docker compose --profile dev up -d; then
      rm -f "$log_file"
      return 0
    fi
  fi

  rm -f "$log_file"
  return 1
}

echo "🚀 Starting Agenteval Labs locally..."

# Create the external network if it doesn't exist
docker network create benchmarking-public 2>/dev/null || true

# Start main services with dev profile (includes frontend-dev)
echo "📦 Starting main services..."
run_compose_with_build_fallback

# Wait a bit for services to be ready
echo "⏳ Waiting for services to initialize..."
sleep 5

# Start the nginx proxy
echo "🌐 Starting nginx proxy..."
docker compose -f docker-compose.proxy.yml up -d

echo ""
echo "✅ Services started!"
echo ""
echo "📍 Access the application at:"
echo "   👉 http://localhost:3010"
echo ""
echo "🔍 Check service status:"
echo "   docker compose ps"
echo ""
echo "📋 View logs:"
echo "   docker compose logs -f"
echo ""
echo "🛑 Stop services:"
echo "   docker compose down && docker compose -f docker-compose.proxy.yml down"
