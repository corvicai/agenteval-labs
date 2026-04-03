#!/bin/sh

# This script generates env-config.js from environment variables at runtime.
# It looks for all environment variables starting with VITE_ and writes them
# to /usr/share/nginx/html/env-config.js.

OUTPUT_FILE="/usr/share/nginx/html/env-config.js"

echo "window._env_ = {" > $OUTPUT_FILE

# Only expose a safe, explicit whitelist to the browser.
# Keep API_URL/WS_URL server-side to avoid cross-origin misrouting.
PUBLIC_ENV_REGEX='^(VITE_FIREBASE_|VITE_GIT_COMMIT=|VITE_APP_REVISION=|VITE_APP_REVISION_BRANCH=|VITE_APP_REVISION_DIRTY=|VITE_APP_REVISION_UPDATED_AT=|VITE_ENABLE_LEGACY_AUTH=|VITE_AFK_TIMEOUT_MS=|VITE_POSTHOG_KEY=|VITE_POSTHOG_HOST=|VITE_POSTHOG_ENABLED=|VITE_POSTHOG_ENVIRONMENT=|FIREBASE_|GIT_COMMIT=|APP_REVISION=|APP_REVISION_BRANCH=|APP_REVISION_DIRTY=|APP_REVISION_UPDATED_AT=)'
env | grep -E "${PUBLIC_ENV_REGEX}" | while read -r line; do
  # Extract key and value
  key=$(echo $line | cut -d '=' -f 1)
  value=$(echo $line | cut -d '=' -f 2-)
  
  # Map legacy or infrastructure names to VITE_ names if needed
  mapped_key=$key
  if [ "$key" = "GIT_COMMIT" ]; then
    mapped_key="VITE_GIT_COMMIT"
  elif [ "$key" = "APP_REVISION" ]; then
    mapped_key="VITE_APP_REVISION"
  elif [ "$key" = "APP_REVISION_BRANCH" ]; then
    mapped_key="VITE_APP_REVISION_BRANCH"
  elif [ "$key" = "APP_REVISION_DIRTY" ]; then
    mapped_key="VITE_APP_REVISION_DIRTY"
  elif [ "$key" = "APP_REVISION_UPDATED_AT" ]; then
    mapped_key="VITE_APP_REVISION_UPDATED_AT"
  elif echo "$key" | grep -v '^VITE_' | grep -q '^FIREBASE_'; then
    mapped_key="VITE_$key"
  fi

  # Write to file
  echo "  $mapped_key: \"$value\"," >> $OUTPUT_FILE
done

echo "};" >> $OUTPUT_FILE

echo "Generated $OUTPUT_FILE with the following variables:"
grep "VITE_" $OUTPUT_FILE

# Execute the main command (nginx)
exec "$@"
