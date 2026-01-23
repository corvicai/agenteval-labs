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

# 2. Build Images using Cloud Build (Server-side builds)
echo "🛠️ Building images with Cloud Build..."

gcloud builds submit --tag "$AR_URI/$APP_NAME-python-runner" ./server_python &
PY_BUILD_PID=$!

gcloud builds submit --tag "$AR_URI/$APP_NAME-go-api" ./server_go &
GO_BUILD_PID=$!

gcloud builds submit --tag "$AR_URI/$APP_NAME-frontend" ./frontend &
FE_BUILD_PID=$!

wait $PY_BUILD_PID $GO_BUILD_PID $FE_BUILD_PID

# 3. Deploy Python Runner (Internal Only)
echo "Deploying Python Runner..."
gcloud run deploy "$APP_NAME-python-runner" \
  --image "$AR_URI/$APP_NAME-python-runner" \
  --platform managed \
  --region "$REGION" \
  --no-allow-unauthenticated \
  --ingress internal \
  --set-env-vars="PYTHONUNBUFFERED=1"

PYTHON_RUNNER_URL=$(gcloud run services describe "$APP_NAME-python-runner" --region "$REGION" --format='value(status.url)')

# 4. Deploy Go API
echo "Deploying Go API..."
# We use Secret Manager for DATABASE_URL and JWT_SECRET
# Assumes secrets exist: DB_URL_SECRET, JWT_SECRET_SECRET
gcloud run deploy "$APP_NAME-go-api" \
  --image "$AR_URI/$APP_NAME-go-api" \
  --platform managed \
  --region "$REGION" \
  --allow-unauthenticated \
  --set-env-vars="PYTHON_RUNNER_URL=$PYTHON_RUNNER_URL" \
  --set-secrets="DATABASE_URL=DB_URL_SECRET:latest,JWT_SECRET=JWT_SECRET_SECRET:latest"

GO_API_URL=$(gcloud run services describe "$APP_NAME-go-api" --region "$REGION" --format='value(status.url)')

# 5. Deploy Frontend
echo "Deploying Frontend..."
gcloud run deploy "$APP_NAME-frontend" \
  --image "$AR_URI/$APP_NAME-frontend" \
  --platform managed \
  --region "$REGION" \
  --allow-unauthenticated \
  --set-env-vars="API_URL=$GO_API_URL"

# 6. Admin Bootstrap Logic
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
