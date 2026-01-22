#!/bin/bash

# Configuration
PROJECT_ID=$(gcloud config get-value project)
REGION="us-central1"
APP_NAME="agent-benchmark"

echo "Using Project ID: $PROJECT_ID"
echo "Region: $REGION"

# 1. Build and Push Images to Artifact Registry
# (Assuming you have an artifact registry created)
# REPO="docker-repo"
# IMAGE_BASE="gcr.io/$PROJECT_ID" # Or us-central1-docker.pkg.dev/$PROJECT_ID/$REPO

echo "Building images..."

docker build -t gcr.io/$PROJECT_ID/$APP_NAME-python-runner ./server_python
docker build -t gcr.io/$PROJECT_ID/$APP_NAME-go-api ./server_go
docker build -t gcr.io/$PROJECT_ID/$APP_NAME-frontend ./frontend

echo "Pushing images..."
docker push gcr.io/$PROJECT_ID/$APP_NAME-python-runner
docker push gcr.io/$PROJECT_ID/$APP_NAME-go-api
docker push gcr.io/$PROJECT_ID/$APP_NAME-frontend

# 2. Deploy Python Runner
echo "Deploying Python Runner..."
gcloud run deploy $APP_NAME-python-runner \
  --image gcr.io/$PROJECT_ID/$APP_NAME-python-runner \
  --platform managed \
  --region $REGION \
  --allow-unauthenticated \
  --set-env-vars="PYTHONUNBUFFERED=1"

PYTHON_RUNNER_URL=$(gcloud run services describe $APP_NAME-python-runner --region $REGION --format='value(status.url)')

# 3. Deploy Go API
echo "Deploying Go API..."
# Note: You should set DATABASE_URL and JWT_SECRET via Secret Manager in production
gcloud run deploy $APP_NAME-go-api \
  --image gcr.io/$PROJECT_ID/$APP_NAME-go-api \
  --platform managed \
  --region $REGION \
  --allow-unauthenticated \
  --set-env-vars="PYTHON_RUNNER_URL=$PYTHON_RUNNER_URL,DATABASE_URL=your-database-url,JWT_SECRET=your-secret"

GO_API_URL=$(gcloud run services describe $APP_NAME-go-api --region $REGION --format='value(status.url)')

# 4. Deploy Frontend
echo "Deploying Frontend..."
gcloud run deploy $APP_NAME-frontend \
  --image gcr.io/$PROJECT_ID/$APP_NAME-frontend \
  --platform managed \
  --region $REGION \
  --allow-unauthenticated \
  --set-env-vars="API_URL=$GO_API_URL"

echo "Deployment complete!"
echo "Frontend URL: $(gcloud run services describe $APP_NAME-frontend --region $REGION --format='value(status.url)')"
