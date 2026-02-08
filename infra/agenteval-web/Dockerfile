# Build stage
FROM node:20-alpine AS builder

WORKDIR /app

# Add build arguments for Firebase (Vite needs these at build time)
ARG VITE_FIREBASE_API_KEY
ARG VITE_FIREBASE_AUTH_DOMAIN
ARG VITE_FIREBASE_PROJECT_ID
ARG VITE_FIREBASE_STORAGE_BUCKET
ARG VITE_FIREBASE_MESSAGING_SENDER_ID
ARG VITE_FIREBASE_APP_ID
ARG VITE_GIT_COMMIT

# Set as environment variables for the build process
ENV VITE_FIREBASE_API_KEY=$VITE_FIREBASE_API_KEY
ENV VITE_FIREBASE_AUTH_DOMAIN=$VITE_FIREBASE_AUTH_DOMAIN
ENV VITE_FIREBASE_PROJECT_ID=$VITE_FIREBASE_PROJECT_ID
ENV VITE_FIREBASE_STORAGE_BUCKET=$VITE_FIREBASE_STORAGE_BUCKET
ENV VITE_FIREBASE_MESSAGING_SENDER_ID=$VITE_FIREBASE_MESSAGING_SENDER_ID
ENV VITE_FIREBASE_APP_ID=$VITE_FIREBASE_APP_ID
ENV VITE_GIT_COMMIT=$VITE_GIT_COMMIT

# Copy package files
COPY frontend/package*.json ./
RUN npm install

# Copy source code
COPY frontend/. .

# Build the application
RUN npm run build

# Production stage
FROM nginx:alpine

# Install envsubst (comes with gettext)
RUN apk add --no-cache gettext

# Copy built files
COPY --from=builder /app/dist /usr/share/nginx/html

# Copy nginx config template
COPY frontend/nginx.conf.template /etc/nginx/templates/default.conf.template

# Generate env-config.js at container start (reads VITE_* from runtime env)
COPY frontend/docker-entrypoint.sh /docker-entrypoint.d/99-env-config.sh
RUN chmod +x /docker-entrypoint.d/99-env-config.sh

# Default environment variables
ENV PORT=8080
ENV API_URL=http://go-api:8080

EXPOSE 8080

# The nginx image supports templates out of the box in newer versions, 
# but let's be explicit or use the built-in feature if available.
# Actually, nginx:alpine supports /etc/nginx/templates/*.template since 1.19
CMD ["nginx", "-g", "daemon off;"]
