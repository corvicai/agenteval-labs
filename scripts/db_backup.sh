#!/bin/bash

# Configuration (can be overridden by environment variables)
DB_CONTAINER="${DB_CONTAINER:-benchmarking-db-prod}"
DB_NAME="${DB_NAME:-benchmarking_prod}"
DB_USER="${DB_USER:-postgres}"
BACKUP_FILE="backup_$(date +%Y%m%d_%H%M%S).sql"

echo "🐘 Starting database backup for Cloud SQL..."
echo "Container: $DB_CONTAINER"
echo "Database:  $DB_NAME"

# Check if container is running
if ! docker ps | grep -q "$DB_CONTAINER"; then
    echo "⚠️ Warning: Container $DB_CONTAINER is not running."
    echo "Attempting to find any postgres container..."
    DB_CONTAINER=$(docker ps --filter "ancestor=postgres:15-alpine" --format "{{.Names}}" | head -n 1)
    
    if [ -z "$DB_CONTAINER" ]; then
        echo "❌ Error: No postgres container found."
        exit 1
    fi
    echo "Found running container: $DB_CONTAINER"
    # Try to guess DB name if it's the default dev one
    if [ "$DB_CONTAINER" == "benchmarking-db" ]; then
        DB_NAME="benchmarking"
    fi
    echo "Using Database: $DB_NAME"
fi

# Run pg_dump inside the container
# --no-owner: Don't output commands to set ownership of objects to match the original database.
# --no-privileges: Prevent dumping of access privileges (grant/revoke commands).
# --clean: Include commands to drop database objects before creating them.
# --if-exists: Use IF EXISTS when dropping objects.
# These are essential for Cloud SQL imports to avoid permission denied errors.
echo "Creating dump: $BACKUP_FILE"
docker exec -t "$DB_CONTAINER" pg_dump -U "$DB_USER" \
    --no-owner \
    --no-privileges \
    --clean \
    --if-exists \
    "$DB_NAME" > "$BACKUP_FILE"

if [ $? -eq 0 ]; then
    echo "✅ Backup completed successfully: $BACKUP_FILE"
    echo ""
    echo "🚀 MIGRATION TO CLOUD SQL STEPS:"
    echo "1. Upload the backup to a Google Cloud Storage bucket:"
    echo "   gsutil cp $BACKUP_FILE gs://YOUR_BUCKET_NAME/"
    echo ""
    echo "2. Grant the Cloud SQL Service Account 'Storage Object Viewer' permission to the bucket."
    echo ""
    echo "3. Import the database into Cloud SQL:"
    echo "   gcloud sql import sql [INSTANCE_NAME] gs://YOUR_BUCKET_NAME/$BACKUP_FILE --database=$DB_NAME"
    echo ""
    echo "Tip: You can find your instance name with 'gcloud sql instances list'"
else
    echo "❌ Backup failed!"
    exit 1
fi
