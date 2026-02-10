#!/bin/sh
set -e

# Start the Go API inside the web container when explicitly enabled.
# This is intended for dev unification/testing only.
if [ "${ENABLE_API_IN_WEB:-}" != "1" ]; then
  exit 0
fi

API_INTERNAL_PORT="${API_INTERNAL_PORT:-8081}"
echo "[entrypoint] Starting embedded API on :${API_INTERNAL_PORT}"

# Run API in background on a dedicated port.
PORT="${API_INTERNAL_PORT}" /app/api &
