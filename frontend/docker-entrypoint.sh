#!/bin/sh

# This script generates env-config.js from environment variables at runtime.
# It looks for all environment variables starting with VITE_ and writes them
# to /usr/share/nginx/html/env-config.js.

OUTPUT_FILE="/usr/share/nginx/html/env-config.js"

echo "window._env_ = {" > $OUTPUT_FILE

# Get environment variables starting with VITE_, FIREBASE_, or exactly API_URL/WS_URL/GIT_COMMIT
env | grep -E '^(VITE_|FIREBASE_|API_URL=|WS_URL=|GIT_COMMIT=)' | while read -r line; do
  # Extract key and value
  key=$(echo $line | cut -d '=' -f 1)
  value=$(echo $line | cut -d '=' -f 2-)
  
  # Map legacy or infrastructure names to VITE_ names if needed
  mapped_key=$key
  if [ "$key" = "API_URL" ]; then
    mapped_key="VITE_API_URL"
  elif [ "$key" = "WS_URL" ]; then
    mapped_key="VITE_WS_URL"
  elif [ "$key" = "GIT_COMMIT" ]; then
    mapped_key="VITE_GIT_COMMIT"
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
