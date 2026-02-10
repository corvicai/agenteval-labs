#!/bin/sh
set -e

# Start the Go API inside the web container when explicitly enabled.
# This is intended for dev unification/testing only.
if [ "${ENABLE_API_IN_WEB:-}" != "1" ]; then
  exit 0
fi

API_INTERNAL_PORT="${API_INTERNAL_PORT:-8081}"
API_APP_ENV="${API_APP_ENV:-development}"
echo "[entrypoint] Starting embedded API on :${API_INTERNAL_PORT} (APP_ENV=${API_APP_ENV})"

# Optional Cloud SQL Proxy (for unified API in web container)
CONNECTION_NAME="${CLOUD_SQL_CONNECTION_NAME:-${CSQL_PROXY_INSTANCE_CONNECTION_NAME:-}}"
if [ -n "${CONNECTION_NAME}" ]; then
  echo "[entrypoint] Starting Cloud SQL Proxy for ${CONNECTION_NAME}"
  PROXY_ARGS="--address 127.0.0.1 --port ${DB_PORT:-5432}"
  if [ "${CSQL_PROXY_AUTO_IAM_AUTHN:-}" = "true" ]; then
    PROXY_ARGS="${PROXY_ARGS} --auto-iam-authn"
  fi
  if [ "${CSQL_PROXY_PRIVATE_IP:-}" = "true" ]; then
    PROXY_ARGS="${PROXY_ARGS} --private-ip"
  fi
  /usr/local/bin/cloud-sql-proxy ${PROXY_ARGS} "${CONNECTION_NAME}" &
  export DB_HOST="${DB_HOST:-127.0.0.1}"
  export DB_PORT="${DB_PORT:-5432}"
fi

# Run API in background on a dedicated port.
if [ ! -x /app/api ]; then
  echo "[entrypoint] ERROR: /app/api not found or not executable"
  exit 1
fi

APP_ENV="${API_APP_ENV}" PORT="${API_INTERNAL_PORT}" /app/api &
API_PID=$!
sleep 1
if ! kill -0 "${API_PID}" 2>/dev/null; then
  echo "[entrypoint] ERROR: Embedded API exited early. Check logs above for fatal errors."
  wait "${API_PID}" || true
fi
