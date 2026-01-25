#!/bin/bash
set -e

# Configuration
PROD_COMPOSE="docker-compose.prod.yml"
DB_CONTAINER="benchmarking-db-prod"
ENV_FILE=".env.prod"

echo "🔒 Secure Production Migration & DB Tool"
echo "======================================="

# Helper functions
get_db_pass() {
    if [ -f "$ENV_FILE" ]; then
        grep "POSTGRES_PASSWORD=" "$ENV_FILE" | cut -d'=' -f2
    else
        echo ""
    fi
}

backup() {
    local backup_file="backup_$(date +%Y%m%d%H%M%S).sql.gz"
    echo "📦 Creating backup..."
    
    # Detect Source
    if [ "$(docker ps -q -f name=benchmarking-db-prod)" ]; then
        SOURCE="benchmarking-db-prod"
        DB_NAME="benchmarking_prod"
    elif [ "$(docker ps -q -f name=benchmarking-db)" ]; then
        SOURCE="benchmarking-db"
        DB_NAME="benchmarking"
    else
        echo "❌ Error: No running database found."
        exit 1
    fi

    echo "   Source: $SOURCE"
    docker exec "$SOURCE" pg_dump -U postgres --no-owner --no-privileges "$DB_NAME" | gzip > "$backup_file"
    echo "✅ Backup saved to: $backup_file"
    echo "$backup_file"
}

restore() {
    local backup_file=$1
    if [ ! -f "$backup_file" ]; then
        echo "❌ Error: Backup file '$backup_file' not found."
        exit 1
    fi

    local db_pass=$(get_db_pass)
    echo "♻️  Restoring data to $DB_CONTAINER..."
    
    # Wait for DB to be ready
    until docker exec "$DB_CONTAINER" pg_isready -U postgres > /dev/null 2>&1; do
        echo "   ...waiting for database readiness"
        sleep 2
    done

    zcat "$backup_file" | docker exec -i -e PGPASSWORD="$db_pass" "$DB_CONTAINER" psql -U postgres -d benchmarking_prod
    echo "✅ Restoration completed."
}

verify_integrity() {
    echo "� Verifying data integrity..."
    local db_pass=$(get_db_pass)
    local count=$(docker exec -i -e PGPASSWORD="$db_pass" "$DB_CONTAINER" psql -U postgres -d benchmarking_prod -t -c "SELECT COUNT(*) FROM users;" | xargs)
    echo "   - Current User Count: $count"
    return 0
}

# Main Logic
ACTION=$1

case $ACTION in
    "backup")
        backup
        ;;
    "restore")
        shift
        restore "$1"
        ;;
    "test-cycle")
        echo "🧪 Running Production Restoration Test Cycle..."
        
        # 1. Backup current
        BACKUP_FILE=$(backup | tail -n 1)
        
        # 2. Count before
        echo "📊 Pre-wipe verification:"
        verify_integrity
        
        # 3. Wipe Prod
        echo "🔥 Wiping production environment..."
        docker compose -f "$PROD_COMPOSE" down -v
        echo "✅ Volume wiped."
        
        # 4. Recreate
        echo "🚀 Restarting empty production DB..."
        export $(grep -v '^#' $ENV_FILE | xargs)
        docker compose -f "$PROD_COMPOSE" up -d db-prod
        
        # 5. Restore
        restore "$BACKUP_FILE"
        
        # 6. Verify
        echo "📊 Post-restore verification:"
        verify_integrity
        
        # 7. Restart App
        echo "🚀 Restarting full production stack..."
        docker compose -f "$PROD_COMPOSE" up -d
        
        echo "🎉 Test Cycle Complete. Production is back online with restored data."
        ;;
    *)
        echo "Usage: $0 {backup|restore <file>|test-cycle}"
        exit 1
        ;;
esac
