#!/bin/bash

# Configuration
PROJECT_ID=$(gcloud config get-value project)
REGION="us-central1"
REPO_NAME="docker-repo"
APP_NAME="agent-benchmark"
AR_URI="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPO_NAME}"

echo "🚀 Starting Production Deployment for $APP_NAME"
echo "Project ID: $PROJECT_ID"
echo "Region: $REGION"
echo "Registry: $AR_URI"

# 1. Ensure Artifact Registry repository exists
if ! gcloud artifacts repositories describe "$REPO_NAME" --location="$REGION" &>/dev/null; then
    echo "Creating Artifact Registry repository..."
    gcloud artifacts repositories create "$REPO_NAME" \
        --repository-format=docker \
        --location="$REGION" \
        --description="Docker repository for $APP_NAME"
fi

# 1.1 Ensure Secrets exist in Secret Manager
echo "🔐 Checking secrets..."
for SECRET in DB_URL_SECRET JWT_SECRET_SECRET ENCRYPTION_KEY; do
    if ! gcloud secrets describe "$SECRET" --project="$PROJECT_ID" &>/dev/null; then
        echo "Creating secret $SECRET..."
        if [ "$SECRET" == "ENCRYPTION_KEY" ]; then
            # Generate a random 32-char key for AES-256
            RAND_KEY=$(openssl rand -base64 32 | head -c 32)
            echo -n "$RAND_KEY" | gcloud secrets create "$SECRET" --data-file=- --project="$PROJECT_ID"
        elif [ "$SECRET" == "JWT_SECRET_SECRET" ]; then
            RAND_JWT=$(openssl rand -base64 32)
            echo -n "$RAND_JWT" | gcloud secrets create "$SECRET" --data-file=- --project="$PROJECT_ID"
        else
            echo "⚠️ Secret $SECRET not found. Please create it manually if it is for the database URL."
            # We don't exit here because the user might have provided it already or it might be set differently.
        fi
    fi
done

# 1.2 Load local .env if exists for build arguments (Firebase)
if [ -f .env ]; then
    echo "📄 Loading build arguments from .env..."
    # Export variables starting with VITE_ to be used by gcloud command
    export $(grep '^VITE_' .env | xargs)
fi

# 2. Build Images using Cloud Build (Using infra configs)
echo "🛠️ Building images with Cloud Build..."

# Common substitutions
IMAGE_REPO_BASE="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPO_NAME}"
RELEASE_TAG="latest"

gcloud builds submit --config infra/agenteval-api/cloudbuild.yaml \
    --substitutions="_IMAGE_REPO_BASE=$IMAGE_REPO_BASE,_SERVICE_NAME=agenteval-api,_RELEASE_TAG=$RELEASE_TAG" . &
GO_BUILD_PID=$!

# Frontend build (no more build-args)
gcloud builds submit --config infra/agenteval-web/cloudbuild.yaml \
    --substitutions="_IMAGE_REPO_BASE=$IMAGE_REPO_BASE,_SERVICE_NAME=agenteval-web,_RELEASE_TAG=$RELEASE_TAG" . &
FE_BUILD_PID=$!

wait $GO_BUILD_PID $FE_BUILD_PID

# 3. Deploy Go API
echo "Deploying Go API..."
# We use Secret Manager for DATABASE_URL and JWT_SECRET
# Assumes secrets exist: DB_URL_SECRET, JWT_SECRET_SECRET
gcloud run deploy "$APP_NAME-go-api" \
  --image "$IMAGE_REPO_BASE/agenteval-api:$RELEASE_TAG" \
  --platform managed \
  --region "$REGION" \
  --allow-unauthenticated \
  --set-env-vars="APP_ENV=production" \
  --set-secrets="DATABASE_URL=DB_URL_SECRET:latest,JWT_SECRET=JWT_SECRET_SECRET:latest,ENCRYPTION_KEY=ENCRYPTION_KEY:latest"

GO_API_URL=$(gcloud run services describe "$APP_NAME-go-api" --region "$REGION" --format='value(status.url)')

# 4. Deploy Frontend
echo "Deploying Frontend..."
gcloud run deploy "$APP_NAME-frontend" \
  --image "$IMAGE_REPO_BASE/agenteval-web:$RELEASE_TAG" \
  --platform managed \
  --region "$REGION" \
  --allow-unauthenticated \
  --set-env-vars="API_URL=$GO_API_URL,\
VITE_FIREBASE_API_KEY=$VITE_FIREBASE_API_KEY,\
VITE_FIREBASE_AUTH_DOMAIN=$VITE_FIREBASE_AUTH_DOMAIN,\
VITE_FIREBASE_PROJECT_ID=$VITE_FIREBASE_PROJECT_ID,\
VITE_FIREBASE_STORAGE_BUCKET=$VITE_FIREBASE_STORAGE_BUCKET,\
VITE_FIREBASE_MESSAGING_SENDER_ID=$VITE_FIREBASE_MESSAGING_SENDER_ID,\
VITE_FIREBASE_APP_ID=$VITE_FIREBASE_APP_ID"

# 5. Admin Bootstrap Logic
echo "Checking Admin status at $GO_API_URL..."
# Wait a few seconds for the service to be fully up
sleep 5
ADMIN_EXISTS=$(curl -s "$GO_API_URL/auth/check-admin" | python3 -c "import sys, json; print(json.load(sys.stdin).get('exists', False))")

if [ "$ADMIN_EXISTS" = "False" ]; then
    echo "Creating initial admin user..."
    ADMIN_PASS=$(openssl rand -base64 12)
    ADMIN_EMAIL="admin@agenteval.com"
    
    # Use python to generate JSON to avoid dependency on jq
    BOOTSTRAP_PAYLOAD=$(python3 -c "import json, sys; print(json.dumps({'email': '$ADMIN_EMAIL', 'password': '$ADMIN_PASS', 'organization_name': 'Default Admin Org', 'name': 'System Admin'}))")
    
    curl -s -X POST "$GO_API_URL/auth/bootstrap-admin" \
        -H "Content-Type: application/json" \
        -d "$BOOTSTRAP_PAYLOAD" > /dev/null
    
    BOOTSTRAP_MSG="
****************************************************
🚀 INITIAL ADMIN CREATED
Email: $ADMIN_EMAIL
Password: $ADMIN_PASS
****************************************************"
else
    BOOTSTRAP_MSG="Admin user already exists. Use existing credentials."
fi

echo "Deployment complete!"
echo "Frontend URL: $(gcloud run services describe "$APP_NAME-frontend" --region "$REGION" --format='value(status.url)')"
echo "$BOOTSTRAP_MSG"
