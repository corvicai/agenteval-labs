#!/bin/sh

# This script generates env-config.js from environment variables at runtime.
# It looks for all environment variables starting with VITE_ and writes them
# to /usr/share/nginx/html/env-config.js.

OUTPUT_FILE="/usr/share/nginx/html/env-config.js"

echo "window._env_ = {" > $OUTPUT_FILE

# Get all environment variables starting with VITE_
env | grep '^VITE_' | while read -r line; do
  # Extract key and value
  key=$(echo $line | cut -d '=' -f 1)
  value=$(echo $line | cut -d '=' -f 2-)
  
  # Write to file
  echo "  $key: \"$value\"," >> $OUTPUT_FILE
done

echo "};" >> $OUTPUT_FILE

echo "Generated $OUTPUT_FILE with the following variables:"
grep "VITE_" $OUTPUT_FILE

# Execute the main command (nginx)
exec "$@"
