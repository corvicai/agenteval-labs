#!/bin/bash
set -e

# Configuration
PROD_COMPOSE="docker-compose.prod.yml"
DB_CONTAINER="benchmarking-db-prod"
BACKUP_FILE="backup_pre_migration_$(date +%Y%m%d%H%M%S).sql.gz"
ENV_FILE=".env.prod"

echo "🔒 Secure Production Migration Tool"
echo "==================================="

# 1. Check if production is running
if [ -z "$(docker compose -f $PROD_COMPOSE ps -q db-prod)" ]; then
    echo "⚠️  Production DB container ($DB_CONTAINER) is not running."
    echo "    Attempting to start it temporarily for backup..."
    docker compose -f $PROD_COMPOSE up -d db-prod
    sleep 10 # Wait for DB to be ready
fi

# 2. Backup Data
echo "📦 Backing up current database..."
docker exec $DB_CONTAINER pg_dump -U postgres benchmarking_prod | gzip > $BACKUP_FILE
echo "✅ Backup saved to $BACKUP_FILE"

# 3. Generate New Secrets
echo "🔑 Generating new strong secrets..."
NEW_DB_PASS=$(openssl rand -base64 32 | tr -d '/+=' | cut -c 1-24)
NEW_JWT_SECRET=$(openssl rand -base64 64 | tr -d '\n')

# 4. Create/Update .env.prod
echo "📝 Updating $ENV_FILE..."
cat > $ENV_FILE <<EOF
# Production Secrets - Generated on $(date)
POSTGRES_PASSWORD=$NEW_DB_PASS
JWT_SECRET=$NEW_JWT_SECRET
APP_ENV=production
ALLOWED_ORIGINS=http://localhost
EOF
echo "✅ Secrets saved to $ENV_FILE"

# 5. Destroy Old Infrastructure (including volume to reset DB password)
echo "🔥 Tearing down old containers and volumes..."
docker compose -f $PROD_COMPOSE down -v
echo "✅ Old infrastructure removed."

# 6. Start New Infrastructure
echo "🚀 Starting new secure production environment..."
# Load env vars from file for docker-compose
export $(cat $ENV_FILE | xargs)
docker compose -f $PROD_COMPOSE up -d db-prod

echo "⏳ Waiting for new database to initialize..."
sleep 15 # Give Postgres time to init with new password

# 7. Restore Data
echo "♻️  Restoring data..."
# We need to pass the NEW password to psql inside the container? 
# Usually pg_isready is enough, but to restore we use standard input
zcat $BACKUP_FILE | docker exec -i -e PGPASSWORD=$NEW_DB_PASS $DB_CONTAINER psql -U postgres benchmarking_prod

echo "✅ Data restored successfully."

# 8. Start remaining services
echo "🚀 Starting API and Runners..."
docker compose -f $PROD_COMPOSE up -d --build

echo "🎉 Migration Complete!"
echo "   - New Secrets are in: $ENV_FILE"
echo "   - Backup saved to:    $BACKUP_FILE"
echo "   - API should be running securely now."
