# syntax=docker/dockerfile:1.7
# Build stage
FROM node:24-alpine AS builder
ARG VITE_GIT_COMMIT=unknown
ENV VITE_GIT_COMMIT=$VITE_GIT_COMMIT

WORKDIR /app

# Copy lockfile + manifest first so the install layer caches independently
# of source changes.
COPY package.json package-lock.json ./

# `npm ci` is faster, deterministic, and respects the lockfile (vs `install`
# which can mutate it). The cache mount keeps ~/.npm warm across builds.
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-audit --no-fund

# Copy source AFTER deps so editing source doesn't bust the install layer.
COPY . .

# Build the application
RUN npm run build

# ──────────────────────────────────────────────────────────────────────
# Production stage
# ──────────────────────────────────────────────────────────────────────
FROM nginx:alpine
ARG VITE_GIT_COMMIT=unknown
ENV VITE_GIT_COMMIT=$VITE_GIT_COMMIT

# envsubst (gettext)
RUN apk add --no-cache gettext

# Static build output
COPY --from=builder /app/dist /usr/share/nginx/html

# Nginx config template
COPY nginx.conf.template /etc/nginx/templates/default.conf.template

# Entrypoint script
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

EXPOSE 8080

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"]
