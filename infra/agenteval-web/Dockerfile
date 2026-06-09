# syntax=docker/dockerfile:1.7
# Build stage (frontend)
FROM node:24-alpine AS web_builder
ARG VITE_GIT_COMMIT=unknown
ENV VITE_GIT_COMMIT=$VITE_GIT_COMMIT

WORKDIR /app

# Copy lockfile + manifest first so the install layer caches independently
# of source changes.
COPY frontend/package.json frontend/package-lock.json ./

# `npm ci` is faster, deterministic, and respects the lockfile.
# Cache mount keeps ~/.npm warm across builds.
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-audit --no-fund

# Copy source AFTER deps.
COPY frontend/. .

# Build the application
RUN npm run build

# ──────────────────────────────────────────────────────────────────────
# Production stage
# ──────────────────────────────────────────────────────────────────────
FROM nginx:alpine
ARG VITE_GIT_COMMIT=unknown
ENV VITE_GIT_COMMIT=$VITE_GIT_COMMIT

# envsubst (gettext) + ca-certificates for runtime
RUN apk add --no-cache gettext ca-certificates

# Static build output
COPY --from=web_builder /app/dist /usr/share/nginx/html

# Nginx config template
COPY frontend/nginx.conf.template /etc/nginx/templates/default.conf.template

# Generate env-config.js at container start (reads VITE_* from runtime env)
COPY frontend/docker-entrypoint.sh /docker-entrypoint.d/99-env-config.sh
RUN chmod +x /docker-entrypoint.d/99-env-config.sh

# Default environment variables
ENV PORT=8080
# Fallback Firebase auth host; overridden at runtime by 19-firebase-env.envsh
# from VITE_FIREBASE_PROJECT_ID. Must be non-empty so envsubst always substitutes it.
ENV FIREBASE_AUTH_HOST=placeholder.firebaseapp.com

# Sourced before 20-envsubst-on-templates.sh to export FIREBASE_AUTH_HOST
COPY frontend/docker-firebase-env.envsh /docker-entrypoint.d/19-firebase-env.envsh
RUN chmod +x /docker-entrypoint.d/19-firebase-env.envsh

EXPOSE 8080

# nginx:alpine supports /etc/nginx/templates/*.template since 1.19
CMD ["nginx", "-g", "daemon off;"]
